package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/viniciussantos/claude-testcontainers/test/testhelper"
)

// NOVO MODELO - USANDO CONTAINER COMPARTILHADO
func TestProductRepository(t *testing.T) {
	// ✅ SOLUÇÃO: Container compartilhado entre todos os testes
	suite := testhelper.NewIntegrationTestSuite(t)
	suite.Setup() // Limpa estado para isolamento
	defer suite.Teardown()
	
	// Usa cliente compartilhado
	repo := NewProductRepository(suite.ES())
	ctx := context.Background()
	
	t.Run("Create and Get Product", func(t *testing.T) {
		tenantID := suite.NewTenantID() // Tenant único para este subteste
		product := &Product{
			ID:          "1", 
			Name:        "Test Product",
			Description: "A test product",
			Price:       99.99,
			Category:    "electronics",
			TenantID:    tenantID,
		}
		
		err := repo.Create(ctx, product)
		require.NoError(t, err)
		
		retrieved, err := repo.GetByID(ctx, "1", tenantID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		
		assert.Equal(t, product.ID, retrieved.ID)
		assert.Equal(t, product.Name, retrieved.Name)
		assert.Equal(t, product.Price, retrieved.Price)
		assert.Equal(t, product.TenantID, retrieved.TenantID)
	})
	
	t.Run("Search by Category", func(t *testing.T) {
		tenantID := suite.NewTenantID() // Tenant único para este subteste
		product1 := &Product{
			ID:       "2",
			Name:     "Electronics Product",
			Category: "electronics", 
			Price:    199.99,
			TenantID: tenantID,
		}
		
		product2 := &Product{
			ID:       "3",
			Name:     "Books Product",
			Category: "books",
			Price:    29.99,
			TenantID: tenantID,
		}
		
		err := repo.Create(ctx, product1)
		require.NoError(t, err)
		
		err = repo.Create(ctx, product2)
		require.NoError(t, err)
		
		// Aguarda indexação
		suite.WaitForIndexing()
		
		electronics, err := repo.SearchByCategory(ctx, "electronics", tenantID)
		require.NoError(t, err)
		
		assert.Len(t, electronics, 1)
		assert.Equal(t, "2", electronics[0].ID)
		assert.Equal(t, "Electronics Product", electronics[0].Name)
		assert.Equal(t, tenantID, electronics[0].TenantID)
	})
	
	t.Run("Get Non-Existent Product", func(t *testing.T) {
		tenantID := suite.NewTenantID() // Tenant único para este subteste
		product, err := repo.GetByID(ctx, "non-existent", tenantID)
		require.NoError(t, err)
		assert.Nil(t, product)
	})
}

// EXEMPLO DE MÚLTIPLOS TESTES COM ISOLAMENTO
func TestProductRepository_Multiple(t *testing.T) {
	// ✅ SOLUÇÃO: Mesmo container, dados limpos automaticamente
	suite := testhelper.NewIntegrationTestSuite(t)
	suite.Setup() // Estado limpo garantido
	defer suite.Teardown()
	
	repo := NewProductRepository(suite.ES())
	ctx := context.Background()
	
	t.Run("Bulk Operations", func(t *testing.T) {
		tenantId := testhelper.GenerateTenantID()

		products := []*Product{
			{ID: "bulk1", Name: "Product 1", Category: "test", Price: 10.0, TenantID: tenantId},
			{ID: "bulk2", Name: "Product 2", Category: "test", Price: 20.0, TenantID: tenantId},
			{ID: "bulk3", Name: "Product 3", Category: "test", Price: 30.0, TenantID: tenantId},
		}
		
		for _, p := range products {
			err := repo.Create(ctx, p)
			require.NoError(t, err)
		}
		
		suite.WaitForIndexing()
		
		results, err := repo.SearchByCategory(ctx, "test", tenantId)
		require.NoError(t, err)
		assert.Len(t, results, 3)
	})
	
	t.Run("Category Isolation", func(t *testing.T) {
		tenantId := testhelper.GenerateTenantID()

		// ✅ Estado limpo - não vê dados do teste anterior
		product := &Product{
			ID:       "isolated",
			Name:     "Isolated Product",
			Category: "isolated-category",
			Price:    50.0,
			TenantID: tenantId,
		}
		
		err := repo.Create(ctx, product)
		require.NoError(t, err)
		
		suite.WaitForIndexing()
		
		results, err := repo.SearchByCategory(ctx, "isolated-category", tenantId)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "isolated", results[0].ID)
		
		// Confirma que não vê dados de outros testes
		otherResults, err := repo.SearchByCategory(ctx, "test", tenantId)
		require.NoError(t, err)
		assert.Empty(t, otherResults)
	})
}

// EXEMPLO DE SUITE DE TESTES ESTRUTURADA
func TestProductRepository_Suite(t *testing.T) {
	suite := testhelper.NewIntegrationTestSuite(t)
	suite.Setup()
	defer suite.Teardown()
	
	repo := NewProductRepository(suite.ES())
	ctx := context.Background()
	
	// Setup de fixtures para toda a suite
	setupTestProducts := func(tenantID string) []*Product {
		products := []*Product{
			{ID: "p1", Name: "Laptop", Category: "electronics", Price: 999.99, TenantID: tenantID},
			{ID: "p2", Name: "Book", Category: "books", Price: 19.99, TenantID: tenantID},
			{ID: "p3", Name: "Phone", Category: "electronics", Price: 599.99, TenantID: tenantID},
		}
		
		for _, p := range products {
			err := repo.Create(ctx, p)
			require.NoError(t, err)
		}
		
		suite.WaitForIndexing()
		return products
	}
	
	t.Run("Search Electronics", func(t *testing.T) {
		tenantId := testhelper.GenerateTenantID()

		setupTestProducts(tenantId)
		
		results, err := repo.SearchByCategory(ctx, "electronics", tenantId)
		require.NoError(t, err)
		
		assert.Len(t, results, 2)
		
		// Verifica se ambos produtos de eletrônicos foram encontrados
		ids := make([]string, len(results))
		for i, p := range results {
			ids[i] = p.ID
		}
		assert.Contains(t, ids, "p1")
		assert.Contains(t, ids, "p3")
	})
	
	t.Run("Search Books", func(t *testing.T) {
		tenantId := testhelper.GenerateTenantID()

		setupTestProducts(tenantId)
		
		results, err := repo.SearchByCategory(ctx, "books", tenantId)
		require.NoError(t, err)
		
		assert.Len(t, results, 1)
		assert.Equal(t, "p2", results[0].ID)
		assert.Equal(t, "Book", results[0].Name)
	})
	
	t.Run("Individual Product Retrieval", func(t *testing.T) {
		tenantId := testhelper.GenerateTenantID()

		setupTestProducts(tenantId)
		
		product, err := repo.GetByID(ctx, "p1", tenantId)
		require.NoError(t, err)
		require.NotNil(t, product)
		
		assert.Equal(t, "Laptop", product.Name)
		assert.Equal(t, 999.99, product.Price)
	})
}

// EXEMPLO DE TESTE COM HELPERS DO TESTHELPER
func TestProductRepository_WithHelpers(t *testing.T) {
	suite := testhelper.NewIntegrationTestSuite(t)
	suite.Setup()
	defer suite.Teardown()
	
	t.Run("Using Suite Helpers", func(t *testing.T) {
		// ✅ Usa helpers do testhelper para operações comuns
		product := &Product{
			ID:       "helper-test",
			Name:     "Helper Product",
			Category: "helpers",
			Price:    25.50,
		}
		
		// Indexa usando helper
		suite.IndexDocument("products", product.ID, product)
		suite.WaitForIndexing()
		
		// Verifica usando helper
		var retrieved Product
		found := suite.GetDocument("products", product.ID, &retrieved)
		require.True(t, found)
		assert.Equal(t, product.Name, retrieved.Name)
		
		// Busca usando helper
		query := map[string]interface{}{
			"query": map[string]interface{}{
				"match": map[string]interface{}{
					"category": "helpers",
				},
			},
		}
		
		results := suite.SearchDocuments("products", query)
		assert.Equal(t, 1, results.TotalHits())
		
		var products []Product
		err := results.UnmarshalDocuments(&products)
		require.NoError(t, err)
		assert.Len(t, products, 1)
		assert.Equal(t, "Helper Product", products[0].Name)
	})
}

// EXEMPLO DE TESTES PARALELOS (ISOLADOS)
func TestProductRepository_Parallel(t *testing.T) {
	tenantId := testhelper.GenerateTenantID()

	t.Parallel() // ✅ Marca como paralelizável
	
	suite := testhelper.NewIntegrationTestSuite(t)
	suite.Setup()
	defer suite.Teardown()
	
	repo := NewProductRepository(suite.ES())
	ctx := context.Background()
	
	// Cada teste paralelo usa namespace único para evitar conflitos
	testID := "parallel_" + t.Name()
	
	product := &Product{
		ID:       testID,
		Name:     "Parallel Product",
		Category: "parallel",
		Price:    123.45,
		TenantID: tenantId,
	}
	
	err := repo.Create(ctx, product)
	require.NoError(t, err)
	
	retrieved, err := repo.GetByID(ctx, testID, tenantId)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	
	assert.Equal(t, product.Name, retrieved.Name)
}