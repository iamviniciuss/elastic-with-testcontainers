package testhelper

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	sharedMongo *SharedMongoDB
	mongoOnce   sync.Once
)

// SharedMongoDB gerencia um container MongoDB compartilhado entre testes
type SharedMongoDB struct {
	mu           sync.RWMutex
	container    testcontainers.Container
	client       *mongo.Client
	database     *mongo.Database
	databaseDW   *mongo.Database
	url          string
	refCount     int32
	startOnce    sync.Once
	started      bool
	dbName       string
	dbNameDW     string
}

// GetSharedMongoDB retorna a instância singleton do MongoDB compartilhado
func GetSharedMongoDB() *SharedMongoDB {
	mongoOnce.Do(func() {
		sharedMongo = &SharedMongoDB{}
	})
	return sharedMongo
}

// Start inicializa o container MongoDB compartilhado
func (s *SharedMongoDB) Start(ctx context.Context) error {
	// Primeiro, tenta reutilizar container existente (sem lock global)
	s.mu.RLock()
	if s.started && s.client != nil {
		s.mu.RUnlock()
		// Testa conexão sem lock para permitir paralelismo
		if err := s.testConnection(ctx); err == nil {
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
	if s.started && s.client != nil {
		if err := s.testConnection(ctx); err == nil {
			atomic.AddInt32(&s.refCount, 1)
			return nil
		}
		// Conexão perdida, reset para tentar novamente
		s.started = false
		s.startOnce = sync.Once{}
	}
	
	var err error
	s.startOnce.Do(func() {
		err = s.startContainer(ctx)
		if err == nil {
			s.started = true
		}
	})
	
	if !s.started {
		return fmt.Errorf("shared mongodb not started: %w", err)
	}
	
	atomic.AddInt32(&s.refCount, 1)
	return nil
}

// Stop decrementa o contador de referências e para o container se necessário
func (s *SharedMongoDB) Stop(ctx context.Context) error {
	if atomic.AddInt32(&s.refCount, -1) <= 0 {
		return s.stopContainer(ctx)
	}
	return nil
}

// GetClient retorna o cliente MongoDB
func (s *SharedMongoDB) GetClient() *mongo.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

// GetDatabase retorna o database principal
func (s *SharedMongoDB) GetDatabase() *mongo.Database {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.database
}

// GetDatabaseDW retorna o database DW
func (s *SharedMongoDB) GetDatabaseDW() *mongo.Database {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.databaseDW
}

// GetURL retorna a URL do MongoDB
func (s *SharedMongoDB) GetURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.url
}

// startContainer inicia o container MongoDB ou usa um externo
func (s *SharedMongoDB) startContainer(ctx context.Context) error {
	// Verifica se deve usar MongoDB externo
	if useExternal, _ := strconv.ParseBool(os.Getenv("USE_EXTERNAL_MONGO")); useExternal {
		return s.setupExternalMongoDB()
	}
	
	return s.setupTestcontainer(ctx)
}

// setupExternalMongoDB configura cliente para MongoDB externo
func (s *SharedMongoDB) setupExternalMongoDB() error {
	mongoURL := os.Getenv("MONGO_URL")
	if mongoURL == "" {
		mongoURL = "mongodb://localhost:27017"
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	
	clientOpts := options.Client().
		ApplyURI(mongoURL).
		SetServerSelectionTimeout(20 * time.Second)
	
	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return fmt.Errorf("failed to create mongodb client: %w", err)
	}
	
	// Testa conectividade
	err = client.Ping(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to external mongodb: %w", err)
	}
	
	// Gera nomes únicos para databases
	s.dbName = fmt.Sprintf("testdb_%d_%d", os.Getpid(), time.Now().UnixNano())
	s.dbNameDW = fmt.Sprintf("testdb_%d_%d_dw", os.Getpid(), time.Now().UnixNano())
	
	s.client = client
	s.database = client.Database(s.dbName)
	s.databaseDW = client.Database(s.dbNameDW)
	s.url = mongoURL
	
	if isDebugEnabled() {
		fmt.Printf("✅ Using external MongoDB at %s\n", mongoURL)
	}
	
	return nil
}

// setupTestcontainer cria e inicia um container MongoDB
func (s *SharedMongoDB) setupTestcontainer(ctx context.Context) error {
	if isDebugEnabled() {
		fmt.Println("🚀 Starting shared MongoDB container...")
	}
	
	const mongoImage = "mongo:5"
	const user = "user"
	const pass = "pass"
	
	req := testcontainers.ContainerRequest{
		Image:        mongoImage,
		ExposedPorts: []string{"27017/tcp"},
		Name:         "shared-mongodb-test",
		Env: map[string]string{
			"MONGO_INITDB_ROOT_USERNAME": user,
			"MONGO_INITDB_ROOT_PASSWORD": pass,
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("Waiting for connections"),
			wait.ForListeningPort("27017/tcp"),
		).WithStartupTimeout(60 * time.Second),
	}
	
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Reuse:            shouldReuseContainer(),
	})
	if err != nil {
		return fmt.Errorf("failed to start mongodb container: %w", err)
	}
	
	host, err := container.Host(ctx)
	if err != nil {
		return fmt.Errorf("failed to get container host: %w", err)
	}
	
	// Em alguns ambientes, host pode ser "localhost" que resolve para ::1; prefira IPv4:
	if host == "localhost" {
		host = "127.0.0.1"
	}
	
	mappedPort, err := container.MappedPort(ctx, "27017/tcp")
	if err != nil {
		return fmt.Errorf("failed to get mapped port: %w", err)
	}
	
	// Gera nomes únicos para databases
	s.dbName = fmt.Sprintf("testdb_%d_%d", os.Getpid(), time.Now().UnixNano())
	s.dbNameDW = fmt.Sprintf("testdb_%d_%d_dw", os.Getpid(), time.Now().UnixNano())
	
	// Monte a URI com authSource=admin
	uri := fmt.Sprintf("mongodb://%s:%s@%s:%s/%s?authSource=admin",
		user, pass, host, mappedPort.Port(), s.dbName)
	
	// Opções do client com timeout de seleção de servidor
	clientOpts := options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(20 * time.Second)
	
	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return fmt.Errorf("failed to connect to mongodb: %w", err)
	}
	
	// Testa conectividade
	ctxPing, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	
	err = client.Ping(ctxPing, nil)
	if err != nil {
		return fmt.Errorf("failed to ping mongodb: %w", err)
	}
	
	s.container = container
	s.client = client
	s.database = client.Database(s.dbName)
	s.databaseDW = client.Database(s.dbNameDW)
	s.url = fmt.Sprintf("mongodb://%s:%s@%s:%s", user, pass, host, mappedPort.Port())
	
	if isDebugEnabled() {
		fmt.Printf("✅ Shared MongoDB container started at %s:%s\n", host, mappedPort.Port())
	}
	
	log.Printf("✅ Shared MongoDB container started at %s:%s", host, mappedPort.Port())
	
	return nil
}

// stopContainer para o container se não estiver sendo reutilizado
func (s *SharedMongoDB) stopContainer(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.client != nil {
		if isDebugEnabled() {
			fmt.Println("🔌 Disconnecting MongoDB client...")
		}
		// Desconecta o client
		if err := s.client.Disconnect(ctx); err != nil {
			log.Printf("Warning: failed to disconnect MongoDB client: %v", err)
		}
	}
	
	if s.container != nil && !shouldReuseContainer() {
		if isDebugEnabled() {
			fmt.Println("🛑 Stopping shared MongoDB container...")
		}
		return s.container.Terminate(ctx)
	}
	
	return nil
}

// CleanDatabase limpa todas as coleções dos databases
func (s *SharedMongoDB) CleanDatabase(ctx context.Context) error {
	s.mu.RLock()
	client := s.client
	database := s.database
	databaseDW := s.databaseDW
	s.mu.RUnlock()
	
	if client == nil {
		return fmt.Errorf("mongodb client not available")
	}
	
	// Limpa database principal
	if database != nil {
		collections, err := database.ListCollectionNames(ctx, map[string]interface{}{})
		if err != nil {
			return fmt.Errorf("failed to list collections: %w", err)
		}
		
		for _, collection := range collections {
			err = database.Collection(collection).Drop(ctx)
			if err != nil && isDebugEnabled() {
				fmt.Printf("⚠️  Failed to drop collection %s: %v\n", collection, err)
			}
		}
	}
	
	// Limpa database DW
	if databaseDW != nil {
		collections, err := databaseDW.ListCollectionNames(ctx, map[string]interface{}{})
		if err != nil {
			return fmt.Errorf("failed to list DW collections: %w", err)
		}
		
		for _, collection := range collections {
			err = databaseDW.Collection(collection).Drop(ctx)
			if err != nil && isDebugEnabled() {
				fmt.Printf("⚠️  Failed to drop DW collection %s: %v\n", collection, err)
			}
		}
	}
	
	return nil
}

// ResetSpecificCollections remove coleções específicas (como no builder original)
func (s *SharedMongoDB) ResetSpecificCollections(ctx context.Context) error {
	database := s.GetDatabase()
	if database == nil {
		return fmt.Errorf("mongo database not initialized")
	}
	
	collections := []string{"dw_surveys", "ext_tickets", "ext_boards"}
	
	for _, collName := range collections {
		err := database.Collection(collName).Drop(ctx)
		if err != nil && isDebugEnabled() {
			fmt.Printf("⚠️  Failed to drop collection %s: %v\n", collName, err)
		}
	}
	
	return nil
}

// testConnection testa se a conexão com MongoDB está funcionando
func (s *SharedMongoDB) testConnection(ctx context.Context) error {
	if s.client == nil {
		return fmt.Errorf("client is nil")
	}
	
	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	
	return s.client.Ping(ctxPing, nil)
}