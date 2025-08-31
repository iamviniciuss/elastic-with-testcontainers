package testhelper

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestDependenciesBuilder implementa o padrão Builder para dependências de teste
// Integrado ao sistema de containers compartilhados do testhelper
type TestDependenciesBuilder struct {
	// Conexões finais
	PostgresConn *sql.DB
	MongoConn    *mongo.Database
	MongoConnDW  *mongo.Database
	ESConn       *elasticsearch.Client
	
	// Funções de limpeza individuais
	ESClearFunc    func()
	MongoClearFunc func(ctx context.Context) error
	PostgresClearFunc func(ctx context.Context) error
	
	// Referências para os shared containers
	sharedES    *SharedElasticsearch
	sharedMongo *SharedMongoDB
	sharedPG    *SharedPostgreSQL
	
	// Configuração
	needsPostgres     bool
	needsMongo        bool
	needsElasticsearch bool
	sqlFilePaths      []string
	
	// Controle interno
	cleanupFuncs []func()
	built        bool
	mu           sync.RWMutex
}

// NewTestDependenciesBuilder cria uma nova instância do builder
func NewTestDependenciesBuilder() *TestDependenciesBuilder {
	return &TestDependenciesBuilder{
		cleanupFuncs: make([]func(), 0),
	}
}

// WithPostgres configura o builder para usar PostgreSQL com arquivos SQL opcionais
func (b *TestDependenciesBuilder) WithPostgres(sqlFilePaths ...string) *TestDependenciesBuilder {
	b.needsPostgres = true
	b.sqlFilePaths = sqlFilePaths
	return b
}

// WithMongo configura o builder para usar MongoDB
func (b *TestDependenciesBuilder) WithMongo() *TestDependenciesBuilder {
	b.needsMongo = true
	return b
}

// WithElasticsearch configura o builder para usar Elasticsearch
func (b *TestDependenciesBuilder) WithElasticsearch() *TestDependenciesBuilder {
	b.needsElasticsearch = true
	return b
}

// Build cria e inicializa as dependências configuradas em paralelo
func (b *TestDependenciesBuilder) Build() (*TestDependenciesBuilder, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if b.built {
		return b, nil // Já foi construído
	}
	
	if isDebugEnabled() {
		log.Println("🚀 Building test dependencies...")
	}
	start := time.Now()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []error
	
	ctx := context.Background()
	
	// Setup PostgreSQL se necessário
	if b.needsPostgres {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if isDebugEnabled() {
				log.Println("📦 Initializing PostgreSQL...")
			}
			
			b.sharedPG = GetSharedPostgreSQL()
			err := b.sharedPG.Start(ctx, b.sqlFilePaths...)
			
			mu.Lock()
			if err != nil {
				errors = append(errors, fmt.Errorf("postgres setup failed: %w", err))
			} else {
				b.PostgresConn = b.sharedPG.GetConnection()
				b.PostgresClearFunc = b.sharedPG.CleanDatabase
				b.cleanupFuncs = append(b.cleanupFuncs, func() {
					b.sharedPG.Stop(ctx)
				})
				if isDebugEnabled() {
					log.Println("✅ PostgreSQL initialized successfully")
				}
			}
			mu.Unlock()
		}()
	}
	
	// Setup MongoDB se necessário
	if b.needsMongo {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if isDebugEnabled() {
				log.Println("📦 Initializing MongoDB...")
			}
			
			b.sharedMongo = GetSharedMongoDB()
			err := b.sharedMongo.Start(ctx)
			
			mu.Lock()
			if err != nil {
				errors = append(errors, fmt.Errorf("mongo setup failed: %w", err))
			} else {
				b.MongoConn = b.sharedMongo.GetDatabase()
				b.MongoConnDW = b.sharedMongo.GetDatabaseDW()
				b.MongoClearFunc = b.sharedMongo.CleanDatabase
				b.cleanupFuncs = append(b.cleanupFuncs, func() {
					b.sharedMongo.Stop(ctx)
				})
				if isDebugEnabled() {
					log.Println("✅ MongoDB initialized successfully")
				}
			}
			mu.Unlock()
		}()
	}
	
	// Setup Elasticsearch se necessário
	if b.needsElasticsearch {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if isDebugEnabled() {
				log.Println("📦 Initializing Elasticsearch...")
			}
			
			b.sharedES = GetSharedElasticsearch()
			err := b.sharedES.Start(ctx)
			
			mu.Lock()
			if err != nil {
				errors = append(errors, fmt.Errorf("elasticsearch setup failed: %w", err))
			} else {
				b.ESConn = b.sharedES.GetClient()
				b.ESClearFunc = func() {
					b.sharedES.CleanIndices(ctx)
				}
				b.cleanupFuncs = append(b.cleanupFuncs, func() {
					b.sharedES.Stop(ctx)
				})
				if isDebugEnabled() {
					log.Println("✅ Elasticsearch initialized successfully")
				}
			}
			mu.Unlock()
		}()
	}
	
	// Aguarda todos os goroutines terminarem
	wg.Wait()
	
	if len(errors) > 0 {
		b.cleanup()
		return nil, fmt.Errorf("initialization errors: %v", errors)
	}

	elapsed := time.Since(start)
	if isDebugEnabled() {
		log.Printf("🎉 Test dependencies built successfully in %v", elapsed)
	}
	
	b.built = true
	
	// Retorna uma nova instância com as conexões populadas
	return &TestDependenciesBuilder{
		PostgresConn:      b.PostgresConn,
		MongoConn:         b.MongoConn,
		MongoConnDW:       b.MongoConnDW,
		ESConn:            b.ESConn,
		ESClearFunc:       b.ESClearFunc,
		MongoClearFunc:    b.MongoClearFunc,
		PostgresClearFunc: b.PostgresClearFunc,
		
		// Mantém referências para limpeza
		sharedES:     b.sharedES,
		sharedMongo:  b.sharedMongo,
		sharedPG:     b.sharedPG,
		cleanupFuncs: b.cleanupFuncs,
		built:        true,
	}, nil
}

// cleanup executa todas as funções de limpeza registradas
func (b *TestDependenciesBuilder) cleanup() {
	for i := len(b.cleanupFuncs) - 1; i >= 0; i-- {
		if b.cleanupFuncs[i] != nil {
			b.cleanupFuncs[i]()
		}
	}
}

// Cleanup limpa todos os recursos
func (b *TestDependenciesBuilder) Cleanup() {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if isDebugEnabled() {
		log.Println("🧹 Cleaning up test dependencies...")
	}
	b.cleanup()
	if isDebugEnabled() {
		log.Println("✅ Cleanup completed")
	}
}

// ResetElasticsearch limpa todos os índices do Elasticsearch
func (b *TestDependenciesBuilder) ResetElasticsearch() {
	if b.ESClearFunc != nil {
		b.ESClearFunc()
	}
}

// ResetMongo limpa todas as coleções do MongoDB
func (b *TestDependenciesBuilder) ResetMongo(ctx context.Context) error {
	if b.MongoClearFunc != nil {
		return b.MongoClearFunc(ctx)
	}
	return fmt.Errorf("mongo connection not initialized")
}

// ResetSpecificMongoCollections limpa coleções específicas do MongoDB (compatível com builder original)
func (b *TestDependenciesBuilder) ResetSpecificMongoCollections(ctx context.Context) error {
	if b.sharedMongo == nil {
		return fmt.Errorf("mongo connection not initialized")
	}
	return b.sharedMongo.ResetSpecificCollections(ctx)
}

// ResetPostgres limpa todas as tabelas do PostgreSQL
func (b *TestDependenciesBuilder) ResetPostgres(ctx context.Context) error {
	if b.PostgresClearFunc != nil {
		return b.PostgresClearFunc(ctx)
	}
	return fmt.Errorf("postgres connection not initialized")
}

// ResetPostgresSequences reseta as sequences do PostgreSQL
func (b *TestDependenciesBuilder) ResetPostgresSequences(ctx context.Context) error {
	if b.sharedPG == nil {
		return fmt.Errorf("postgres connection not initialized")
	}
	return b.sharedPG.ResetSequences(ctx)
}

// GetElasticsearchURL retorna a URL do Elasticsearch
func (b *TestDependenciesBuilder) GetElasticsearchURL() string {
	if b.sharedES != nil {
		return b.sharedES.GetURL()
	}
	return ""
}

// GetMongoURL retorna a URL do MongoDB
func (b *TestDependenciesBuilder) GetMongoURL() string {
	if b.sharedMongo != nil {
		return b.sharedMongo.GetURL()
	}
	return ""
}

// GetPostgresURL retorna a URL do PostgreSQL
func (b *TestDependenciesBuilder) GetPostgresURL() string {
	if b.sharedPG != nil {
		return b.sharedPG.GetURL()
	}
	return ""
}

// IsBuilt verifica se o builder foi construído
func (b *TestDependenciesBuilder) IsBuilt() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.built
}