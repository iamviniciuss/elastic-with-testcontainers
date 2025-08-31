package testhelper

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	sharedPG *SharedPostgreSQL
	pgOnce   sync.Once
)

// SharedPostgreSQL gerencia um container PostgreSQL compartilhado entre testes
type SharedPostgreSQL struct {
	mu           sync.RWMutex
	container    testcontainers.Container
	connection   *sql.DB
	url          string
	refCount     int32
	startOnce    sync.Once
	started      bool
	dbName       string
	sqlFilePaths []string
}

// GetSharedPostgreSQL retorna a instância singleton do PostgreSQL compartilhado
func GetSharedPostgreSQL() *SharedPostgreSQL {
	pgOnce.Do(func() {
		sharedPG = &SharedPostgreSQL{}
	})
	return sharedPG
}

// Start inicializa o container PostgreSQL compartilhado
func (s *SharedPostgreSQL) Start(ctx context.Context, sqlFilePaths ...string) error {
	// Primeiro, tenta reutilizar container existente (sem lock global)
	s.mu.RLock()
	if s.started && s.connection != nil {
		s.mu.RUnlock()
		// Testa conexão sem lock para permitir paralelismo
		if err := s.testConnection(); err == nil {
			atomic.AddInt32(&s.refCount, 1)
			return nil
		}
		// Conexão perdida, precisa reinicializar
	} else {
		s.mu.RUnlock()
	}
	
	// Se chegou aqui, precisa criar/recriar o container
	// Agora sim usa lock exclusivo apenas para criação
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Double-check: outro goroutine pode ter criado enquanto aguardava lock
	if s.started && s.connection != nil {
		if err := s.testConnection(); err == nil {
			atomic.AddInt32(&s.refCount, 1)
			return nil
		}
		// Conexão perdida, reset para tentar novamente
		s.started = false
		s.startOnce = sync.Once{}
	}
	
	// Armazena os SQL paths para este container
	s.sqlFilePaths = sqlFilePaths
	
	var err error
	s.startOnce.Do(func() {
		err = s.startContainer(ctx)
		if err == nil {
			s.started = true
		}
	})
	
	if !s.started {
		return fmt.Errorf("shared postgresql not started: %w", err)
	}
	
	atomic.AddInt32(&s.refCount, 1)
	return nil
}

// Stop decrementa o contador de referências e para o container se necessário
func (s *SharedPostgreSQL) Stop(ctx context.Context) error {
	if atomic.AddInt32(&s.refCount, -1) <= 0 {
		return s.stopContainer(ctx)
	}
	return nil
}

// GetConnection retorna a conexão PostgreSQL
func (s *SharedPostgreSQL) GetConnection() *sql.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connection
}

// GetURL retorna a URL de conexão do PostgreSQL
func (s *SharedPostgreSQL) GetURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.url
}

// startContainer inicia o container PostgreSQL ou usa um externo
func (s *SharedPostgreSQL) startContainer(ctx context.Context) error {
	// Verifica se deve usar PostgreSQL externo
	if useExternal, _ := strconv.ParseBool(os.Getenv("USE_EXTERNAL_PG")); useExternal {
		return s.setupExternalPostgreSQL()
	}
	
	return s.setupTestcontainer(ctx)
}

// setupExternalPostgreSQL configura conexão para PostgreSQL externo
func (s *SharedPostgreSQL) setupExternalPostgreSQL() error {
	pgURL := os.Getenv("PG_URL")
	if pgURL == "" {
		pgURL = "host=localhost port=5432 user=test password=test sslmode=disable"
	}
	
	conn, err := sql.Open("postgres", pgURL)
	if err != nil {
		return fmt.Errorf("failed to create postgresql connection: %w", err)
	}
	
	// Testa conectividade
	if err := conn.Ping(); err != nil {
		return fmt.Errorf("failed to connect to external postgresql: %w", err)
	}
	
	s.connection = conn
	s.url = pgURL
	
	// Executa SQL files se fornecidos
	if err := s.executeInitialSQL(); err != nil {
		return fmt.Errorf("failed to execute initial SQL: %w", err)
	}
	
	if isDebugEnabled() {
		fmt.Printf("✅ Using external PostgreSQL\n")
	}
	
	return nil
}

// setupTestcontainer cria e inicia um container PostgreSQL
func (s *SharedPostgreSQL) setupTestcontainer(ctx context.Context) error {
	if isDebugEnabled() {
		fmt.Println("🚀 Starting shared PostgreSQL container...")
	}
	
	// Gera nome único do database
	s.dbName = fmt.Sprintf("testdb_%d_%d", os.Getpid(), time.Now().UnixNano())
	
	req := testcontainers.ContainerRequest{
		Image:        "postgres:15",
		ExposedPorts: []string{"5432/tcp"},
		Name:         "shared-postgres-test",
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       s.dbName,
			"POSTGRES_HOST":     "localhost",
			"POSTGRES_PORT":     "5432",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithPollInterval(1 * time.Second).
			WithStartupTimeout(60 * time.Second),
	}
	
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Reuse:            shouldReuseContainer(),
	})
	if err != nil {
		return fmt.Errorf("failed to start postgresql container: %w", err)
	}
	
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		return fmt.Errorf("failed to get mapped port: %w", err)
	}
	
	host, err := container.Host(ctx)
	if err != nil {
		return fmt.Errorf("failed to get container host: %w", err)
	}
	
	dsn := fmt.Sprintf("host=%s port=%s user=test password=test dbname=%s sslmode=disable", 
		host, port.Port(), s.dbName)
	
	dbConn, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	
	// Aguarda database estar pronto com retry
	for i := 0; i < 50; i++ {
		err = dbConn.Ping()
		if err == nil {
			break
		}
		if isDebugEnabled() {
			log.Printf("Waiting for database to be ready... attempt %d/50", i+1)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("database not ready after 50 attempts: %w", err)
	}
	
	s.container = container
	s.connection = dbConn
	s.url = dsn
	
	// Executa SQL files se fornecidos
	if err := s.executeInitialSQL(); err != nil {
		return fmt.Errorf("failed to execute initial SQL: %w", err)
	}
	
	if isDebugEnabled() {
		fmt.Printf("✅ Shared PostgreSQL container started at %s:%s\n", host, port.Port())
	}
	
	log.Printf("✅ Shared PostgreSQL container started at %s:%s", host, port.Port())
	
	return nil
}

// executeInitialSQL executa os arquivos SQL iniciais
func (s *SharedPostgreSQL) executeInitialSQL() error {
	if len(s.sqlFilePaths) == 0 {
		return nil
	}
	
	for _, path := range s.sqlFilePaths {
		if isDebugEnabled() {
			log.Printf("Executing SQL file: %s", path)
		}
		
		initSQL, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read SQL file %s: %w", path, err)
		}
		
		_, err = s.connection.Exec(string(initSQL))
		if err != nil {
			return fmt.Errorf("failed to execute SQL from %s: %w", path, err)
		}
	}
	
	return nil
}

// stopContainer para o container se não estiver sendo reutilizado
func (s *SharedPostgreSQL) stopContainer(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.connection != nil {
		if isDebugEnabled() {
			fmt.Println("🔌 Closing PostgreSQL connection...")
		}
		if err := s.connection.Close(); err != nil {
			log.Printf("Warning: failed to close PostgreSQL connection: %v", err)
		}
	}
	
	if s.container != nil && !shouldReuseContainer() {
		if isDebugEnabled() {
			fmt.Println("🛑 Stopping shared PostgreSQL container...")
		}
		return s.container.Terminate(ctx)
	}
	
	return nil
}

// CleanDatabase executa TRUNCATE em todas as tabelas para limpeza entre testes
func (s *SharedPostgreSQL) CleanDatabase(ctx context.Context) error {
	s.mu.RLock()
	connection := s.connection
	s.mu.RUnlock()
	
	if connection == nil {
		return fmt.Errorf("postgresql connection not available")
	}
	
	// Obtém lista de todas as tabelas do usuário
	rows, err := connection.QueryContext(ctx, `
		SELECT tablename 
		FROM pg_tables 
		WHERE schemaname = 'public'
	`)
	if err != nil {
		return fmt.Errorf("failed to get table list: %w", err)
	}
	defer rows.Close()
	
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			continue
		}
		tables = append(tables, table)
	}
	
	// Desabilita temporarily foreign key checks
	if len(tables) > 0 {
		_, err = connection.ExecContext(ctx, "SET session_replication_role = replica;")
		if err != nil {
			return fmt.Errorf("failed to disable foreign keys: %w", err)
		}
		
		// Truncate todas as tabelas
		for _, table := range tables {
			_, err = connection.ExecContext(ctx, fmt.Sprintf("TRUNCATE TABLE \"%s\" CASCADE", table))
			if err != nil && isDebugEnabled() {
				fmt.Printf("⚠️  Failed to truncate table %s: %v\n", table, err)
			}
		}
		
		// Reabilita foreign key checks
		_, err = connection.ExecContext(ctx, "SET session_replication_role = DEFAULT;")
		if err != nil && isDebugEnabled() {
			fmt.Printf("⚠️  Failed to re-enable foreign keys: %v\n", err)
		}
	}
	
	return nil
}

// ResetSequences reseta todas as sequences para valor inicial
func (s *SharedPostgreSQL) ResetSequences(ctx context.Context) error {
	s.mu.RLock()
	connection := s.connection
	s.mu.RUnlock()
	
	if connection == nil {
		return fmt.Errorf("postgresql connection not available")
	}
	
	// Obtém todas as sequences
	rows, err := connection.QueryContext(ctx, `
		SELECT sequence_name 
		FROM information_schema.sequences 
		WHERE sequence_schema = 'public'
	`)
	if err != nil {
		return fmt.Errorf("failed to get sequence list: %w", err)
	}
	defer rows.Close()
	
	for rows.Next() {
		var sequence string
		if err := rows.Scan(&sequence); err != nil {
			continue
		}
		
		_, err = connection.ExecContext(ctx, fmt.Sprintf("ALTER SEQUENCE \"%s\" RESTART WITH 1", sequence))
		if err != nil && isDebugEnabled() {
			fmt.Printf("⚠️  Failed to reset sequence %s: %v\n", sequence, err)
		}
	}
	
	return nil
}

// testConnection testa se a conexão com PostgreSQL está funcionando
func (s *SharedPostgreSQL) testConnection() error {
	if s.connection == nil {
		return fmt.Errorf("connection is nil")
	}
	
	return s.connection.Ping()
}