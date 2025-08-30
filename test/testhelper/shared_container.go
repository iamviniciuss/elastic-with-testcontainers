package testhelper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/testcontainers/testcontainers-go"
	elasticsearchTestContainer "github.com/testcontainers/testcontainers-go/modules/elasticsearch"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	sharedES   *SharedElasticsearch
	esOnce     sync.Once
)

// SharedElasticsearch gerencia um container Elasticsearch compartilhado entre testes
type SharedElasticsearch struct {
	mu        sync.RWMutex
	container testcontainers.Container
	client    *elasticsearch.Client
	url       string
	refCount  int32
	startOnce sync.Once
	started   bool
}

// GetSharedElasticsearch retorna a instância singleton do Elasticsearch compartilhado
func GetSharedElasticsearch() *SharedElasticsearch {
	esOnce.Do(func() {
		sharedES = &SharedElasticsearch{}
	})
	return sharedES
}

// Start inicializa o container Elasticsearch compartilhado
func (s *SharedElasticsearch) Start(ctx context.Context) error {
	// Primeiro, tenta reutilizar container existente (sem lock global)
	s.mu.RLock()
	if s.started && s.client != nil {
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
	if s.started && s.client != nil {
		if err := s.testConnection(); err == nil {
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
		return fmt.Errorf("shared elasticsearch not started: %w", err)
	}
	
	atomic.AddInt32(&s.refCount, 1)
	return nil
}

// Stop decrementa o contador de referências e para o container se necessário
func (s *SharedElasticsearch) Stop(ctx context.Context) error {
	if atomic.AddInt32(&s.refCount, -1) <= 0 {
		return s.stopContainer(ctx)
	}
	return nil
}

// GetClient retorna o cliente Elasticsearch
func (s *SharedElasticsearch) GetClient() *elasticsearch.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

// GetURL retorna a URL do Elasticsearch
func (s *SharedElasticsearch) GetURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.url
}

// startContainer inicia o container Elasticsearch ou usa um externo
func (s *SharedElasticsearch) startContainer(ctx context.Context) error {
	// Verifica se deve usar Elasticsearch externo
	if useExternal, _ := strconv.ParseBool(os.Getenv("USE_EXTERNAL_ES")); useExternal {
		return s.setupExternalElasticsearch()
	}
	
	return s.setupTestcontainer(ctx)
}

// setupExternalElasticsearch configura cliente para ES externo
func (s *SharedElasticsearch) setupExternalElasticsearch() error {
	esURL := os.Getenv("ES_URL")
	if esURL == "" {
		esURL = "http://localhost:9209"
	}
	
	cfg := elasticsearch.Config{
		Addresses: []string{esURL},
	}
	
	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create elasticsearch client: %w", err)
	}
	
	// Testa conectividade
	res, err := client.Info()
	if err != nil {
		return fmt.Errorf("failed to connect to external elasticsearch: %w", err)
	}
	res.Body.Close()
	
	// Não precisa de lock aqui pois já estamos dentro do contexto de lock da função Start()
	s.client = client
	s.url = esURL
	
	if isDebugEnabled() {
		fmt.Printf("✅ Using external Elasticsearch at %s\n", esURL)
	}
	
	return nil
}

// setupTestcontainer cria e inicia um container Elasticsearch
func (s *SharedElasticsearch) setupTestcontainer(ctx context.Context) error {
	if isDebugEnabled() {
		fmt.Println("🚀 Starting shared Elasticsearch container...")
	}

	// os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "false")

	genericContainerRequest := &testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			WaitingFor: wait.ForLog("started").WithPollInterval(50 * time.Millisecond),
			Name: "shared-elasticsearch-test5",
			Env: map[string]string{
				"ES_JAVA_OPTS":   "-Xms256m -Xmx256m",
				"discovery.type": "single-node",
				// "node.name":      "shared-elasticsearch-test5",
				// "cluster.name":   "shared-elasticsearch-test5",
				"xpack.security.enabled": "false",
				"bootstrap.memory_lock": "false",
			},
			// ExposedPorts: []string{"9200/tcp", "9300/tcp"},
		},
		Started:      false,
		Reuse:        true,
		ProviderType: 0,

	}

	container, err := elasticsearchTestContainer.RunContainer(
		ctx,
		testcontainers.WithImage("docker.elastic.co/elasticsearch/elasticsearch:8.2.0"),
		testcontainers.CustomizeRequest(*genericContainerRequest),
	)
	if err != nil {
		return fmt.Errorf("failed to start elasticsearch container: %w", err)
	}


	cfg := elasticsearch.Config{
		Logger: nil,
		Addresses: []string{
			container.Settings.Address,
		},
	}

	esClient, err := elasticsearch.NewClient(cfg)
	if err != nil {
		panic(err)
	}

	resp, err := esClient.Info()
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()


	// log.Panicf("Elasticsearch container started successfully", address)
	log.Println("Elasticsearch container started successfully", container.Settings.Address)


	

	// s.mu.Lock()
	s.container = container
	s.client = esClient
	s.url = container.Settings.Address
	// s.mu.Unlock()
	
	if isDebugEnabled() {
		fmt.Printf("✅ Shared Elasticsearch container started at %s\n", container.Settings.Address)
	}

	log.Println("✅ Shared Elasticsearch container started at", container.Settings.Address)
	
	return nil
}

// stopContainer para o container se não estiver sendo reutilizado
func (s *SharedElasticsearch) stopContainer(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.container != nil && !shouldReuseContainer() {
		if isDebugEnabled() {
			fmt.Println("🛑 Stopping shared Elasticsearch container...")
		}
		return s.container.Terminate(ctx)
	}
	
	return nil
}

// CleanIndices remove todos os índices para limpeza entre testes
func (s *SharedElasticsearch) CleanIndices(ctx context.Context) error {
	client := s.GetClient()
	if client == nil {
		return fmt.Errorf("elasticsearch client not available")
	}
	
	// Lista todos os índices
	res, err := client.Cat.Indices(
		client.Cat.Indices.WithContext(ctx),
		client.Cat.Indices.WithH("index"),
		client.Cat.Indices.WithFormat("json"),
	)
	if err != nil {
		return fmt.Errorf("failed to list indices: %w", err)
	}
	defer res.Body.Close()
	
	if res.IsError() {
		return fmt.Errorf("elasticsearch error: %s", res.Status())
	}
	
	// Parse da resposta para obter nomes dos índices
	var indices []map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&indices); err != nil {
		return fmt.Errorf("failed to decode indices response: %w", err)
	}
	
	// Deleta índices (exceto os do sistema)
	for _, index := range indices {
		indexName := index["index"].(string)
		if !strings.HasPrefix(indexName, ".") { // Não deleta índices do sistema
			_, err := client.Indices.Delete([]string{indexName})
			if err != nil && isDebugEnabled() {
				fmt.Printf("⚠️  Failed to delete index %s: %v\n", indexName, err)
			}
		}
	}
	
	// Aguarda processamento
	time.Sleep(100 * time.Millisecond)
	
	return nil
}

// RefreshIndices força refresh de todos os índices
func (s *SharedElasticsearch) RefreshIndices(ctx context.Context) error {
	client := s.GetClient()
	if client == nil {
		return fmt.Errorf("elasticsearch client not available")
	}
	
	res, err := client.Indices.Refresh(
		client.Indices.Refresh.WithContext(ctx),
		client.Indices.Refresh.WithIndex("_all"),
	)
	if err != nil {
		return fmt.Errorf("failed to refresh indices: %w", err)
	}
	defer res.Body.Close()
	
	if res.IsError() {
		return fmt.Errorf("elasticsearch refresh error: %s", res.Status())
	}
	
	return nil
}

// isDebugEnabled verifica se o debug está habilitado
func isDebugEnabled() bool {
	debug, _ := strconv.ParseBool(os.Getenv("DEBUG_TEST_CONTAINERS"))
	return debug
}

// shouldReuseContainer verifica se deve reutilizar containers
func shouldReuseContainer() bool {
	reuse, _ := strconv.ParseBool(os.Getenv("TEST_CONTAINER_REUSE"))
	return reuse || true // Por padrão, sempre reutiliza para testes
}

// CleanupSharedResources limpa recursos compartilhados (chamada no TestMain)
func CleanupSharedResources(ctx context.Context) error {
	if sharedES != nil {
		return sharedES.Stop(ctx)
	}
	return nil
}

// testConnection testa se a conexão com Elasticsearch está funcionando
func (s *SharedElasticsearch) testConnection() error {
	if s.client == nil {
		return fmt.Errorf("client is nil")
	}
	
	res, err := s.client.Info()
	if err != nil {
		return err
	}
	defer res.Body.Close()
	
	if res.IsError() {
		return fmt.Errorf("elasticsearch error: %s", res.Status())
	}
	
	return nil
}