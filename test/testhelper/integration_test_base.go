package testhelper

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/stretchr/testify/require"
)

// IntegrationTestSuite fornece funcionalidades base para testes de integração
type IntegrationTestSuite struct {
	t        *testing.T
	ctx      context.Context
	sharedES *SharedElasticsearch
	tenantID string
}

// NewIntegrationTestSuite cria uma nova suite de testes de integração
func NewIntegrationTestSuite(t *testing.T) *IntegrationTestSuite {
	return &IntegrationTestSuite{
		t:        t,
		ctx:      context.Background(),
		sharedES: GetSharedElasticsearch(),
		tenantID: GenerateTenantID(),
	}
}

// Setup inicializa a suite e limpa o estado do Elasticsearch
func (s *IntegrationTestSuite) Setup() {
	s.t.Helper()
	
	// Inicia o container compartilhado
	err := s.sharedES.Start(context.Background())
	// err := s.sharedES.Start(s.ctx)
	require.NoError(s.t, err, "Failed to start shared Elasticsearch")
	
	// Com tenantID, não precisamos limpar todos os índices
	// Cada teste terá isolamento automático via tenantID
}

// Teardown limpa recursos se necessário
func (s *IntegrationTestSuite) Teardown() {
	s.t.Helper()
	
	// Com container compartilhado, não paramos a cada teste
	// O container será limpo automaticamente pelo testcontainers no final
}

// ES retorna o cliente Elasticsearch
func (s *IntegrationTestSuite) ES() *elasticsearch.Client {
	return s.sharedES.GetClient()
}

// GetElasticsearchURL retorna a URL do Elasticsearch
func (s *IntegrationTestSuite) GetElasticsearchURL() string {
	return s.sharedES.GetURL()
}

// CleanElasticsearch remove todos os índices para isolamento entre testes
func (s *IntegrationTestSuite) CleanElasticsearch() {
	s.t.Helper()
	
	err := s.sharedES.CleanIndices(s.ctx)
	require.NoError(s.t, err, "Failed to clean Elasticsearch indices")
}

// CreateIndex cria um novo índice com mapping opcional
func (s *IntegrationTestSuite) CreateIndex(indexName string, mapping map[string]interface{}) {
	s.t.Helper()
	
	var body strings.Builder
	if mapping != nil {
		mappingJSON, err := json.Marshal(map[string]interface{}{
			"mappings": mapping,
		})
		require.NoError(s.t, err, "Failed to marshal mapping")
		body.WriteString(string(mappingJSON))
	}
	
	req := esapi.IndicesCreateRequest{
		Index: indexName,
		Body:  strings.NewReader(body.String()),
	}
	
	res, err := req.Do(s.ctx, s.ES())
	require.NoError(s.t, err, "Failed to create index")
	defer res.Body.Close()
	
	if res.IsError() {
		require.Fail(s.t, fmt.Sprintf("Failed to create index %s: %s", indexName, res.Status()))
	}
}

// IndexDocument indexa um documento no Elasticsearch
func (s *IntegrationTestSuite) IndexDocument(indexName, docID string, document interface{}) {
	s.t.Helper()
	
	docJSON, err := json.Marshal(document)
	require.NoError(s.t, err, "Failed to marshal document")
	
	req := esapi.IndexRequest{
		Index:      indexName,
		DocumentID: docID,
		Body:       strings.NewReader(string(docJSON)),
		Refresh:    "wait_for",
	}
	
	res, err := req.Do(s.ctx, s.ES())
	require.NoError(s.t, err, "Failed to index document")
	defer res.Body.Close()
	
	if res.IsError() {
		require.Fail(s.t, fmt.Sprintf("Failed to index document: %s", res.Status()))
	}
}

// GetDocument recupera um documento do Elasticsearch
func (s *IntegrationTestSuite) GetDocument(indexName, docID string, target interface{}) bool {
	s.t.Helper()
	
	req := esapi.GetRequest{
		Index:      indexName,
		DocumentID: docID,
	}
	
	res, err := req.Do(s.ctx, s.ES())
	require.NoError(s.t, err, "Failed to get document")
	defer res.Body.Close()
	
	if res.StatusCode == 404 {
		return false
	}
	
	if res.IsError() {
		require.Fail(s.t, fmt.Sprintf("Failed to get document: %s", res.Status()))
	}
	
	var response map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&response)
	require.NoError(s.t, err, "Failed to decode response")
	
	if source, found := response["_source"]; found {
		sourceJSON, err := json.Marshal(source)
		require.NoError(s.t, err, "Failed to marshal source")
		
		err = json.Unmarshal(sourceJSON, target)
		require.NoError(s.t, err, "Failed to unmarshal into target")
	}
	
	return true
}

// DeleteDocument remove um documento do Elasticsearch
func (s *IntegrationTestSuite) DeleteDocument(indexName, docID string) {
	s.t.Helper()
	
	req := esapi.DeleteRequest{
		Index:      indexName,
		DocumentID: docID,
		Refresh:    "wait_for",
	}
	
	res, err := req.Do(s.ctx, s.ES())
	require.NoError(s.t, err, "Failed to delete document")
	defer res.Body.Close()
	
	if res.IsError() && res.StatusCode != 404 {
		require.Fail(s.t, fmt.Sprintf("Failed to delete document: %s", res.Status()))
	}
}

// SearchDocuments executa uma busca no Elasticsearch
func (s *IntegrationTestSuite) SearchDocuments(indexName string, query map[string]interface{}) *SearchResult {
	s.t.Helper()
	
	queryJSON, err := json.Marshal(query)
	require.NoError(s.t, err, "Failed to marshal query")
	
	req := esapi.SearchRequest{
		Index: []string{indexName},
		Body:  strings.NewReader(string(queryJSON)),
	}
	
	res, err := req.Do(s.ctx, s.ES())
	require.NoError(s.t, err, "Failed to execute search")
	defer res.Body.Close()
	
	if res.IsError() {
		require.Fail(s.t, fmt.Sprintf("Failed to search: %s", res.Status()))
	}
	
	var searchResponse map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&searchResponse)
	require.NoError(s.t, err, "Failed to decode search response")
	
	return &SearchResult{response: searchResponse}
}

// WaitForIndexing aguarda a indexação dos documentos
func (s *IntegrationTestSuite) WaitForIndexing() {
	return
	s.t.Helper()
	
	err := s.sharedES.RefreshIndices(s.ctx)
	require.NoError(s.t, err, "Failed to refresh indices")
	
	// Pequeno delay adicional para garantir consistência
	time.Sleep(50 * time.Millisecond)
}

// AssertIndexExists verifica se um índice existe
func (s *IntegrationTestSuite) AssertIndexExists(indexName string) {
	s.t.Helper()
	
	req := esapi.IndicesExistsRequest{
		Index: []string{indexName},
	}
	
	res, err := req.Do(s.ctx, s.ES())
	require.NoError(s.t, err, "Failed to check index existence")
	defer res.Body.Close()
	
	require.Equal(s.t, 200, res.StatusCode, "Index %s should exist", indexName)
}

// AssertIndexNotExists verifica se um índice não existe
func (s *IntegrationTestSuite) AssertIndexNotExists(indexName string) {
	s.t.Helper()
	
	req := esapi.IndicesExistsRequest{
		Index: []string{indexName},
	}
	
	res, err := req.Do(s.ctx, s.ES())
	require.NoError(s.t, err, "Failed to check index existence")
	defer res.Body.Close()
	
	require.Equal(s.t, 404, res.StatusCode, "Index %s should not exist", indexName)
}

// SearchResult representa o resultado de uma busca
type SearchResult struct {
	response map[string]interface{}
}

// TotalHits retorna o número total de documentos encontrados
func (r *SearchResult) TotalHits() int {
	hits, ok := r.response["hits"].(map[string]interface{})
	if !ok {
		return 0
	}
	
	total, ok := hits["total"].(map[string]interface{})
	if !ok {
		// Elasticsearch 6.x format
		if totalValue, ok := hits["total"].(float64); ok {
			return int(totalValue)
		}
		return 0
	}
	
	// Elasticsearch 7.x+ format
	value, ok := total["value"].(float64)
	if !ok {
		return 0
	}
	
	return int(value)
}

// Documents retorna os documentos encontrados
func (r *SearchResult) Documents() []map[string]interface{} {
	hits, ok := r.response["hits"].(map[string]interface{})
	if !ok {
		return nil
	}
	
	hitsArray, ok := hits["hits"].([]interface{})
	if !ok {
		return nil
	}
	
	var documents []map[string]interface{}
	for _, hit := range hitsArray {
		hitMap, ok := hit.(map[string]interface{})
		if !ok {
			continue
		}
		
		source, ok := hitMap["_source"].(map[string]interface{})
		if !ok {
			continue
		}
		
		documents = append(documents, source)
	}
	
	return documents
}

// UnmarshalDocuments deserializa os documentos encontrados
func (r *SearchResult) UnmarshalDocuments(target interface{}) error {
	documents := r.Documents()
	documentsJSON, err := json.Marshal(documents)
	if err != nil {
		return err
	}
	
	return json.Unmarshal(documentsJSON, target)
}

// TenantID retorna o tenant ID único para esta suite de teste
func (s *IntegrationTestSuite) TenantID2() string {
	return s.tenantID
}

// NewTenantID gera um novo tenant ID único para sub-testes
func (s *IntegrationTestSuite) NewTenantID() string {
	return GenerateTenantID()
}

// GenerateTenantID gera um tenant ID único para isolamento de testes
func GenerateTenantID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback para timestamp se crypto/rand falhar
		return fmt.Sprintf("test_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("test_%s", hex.EncodeToString(bytes))
}