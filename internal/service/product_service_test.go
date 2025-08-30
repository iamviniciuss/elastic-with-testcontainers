package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/viniciussantos/claude-testcontainers/internal/repository"
	"github.com/viniciussantos/claude-testcontainers/test/testhelper"
)

// EXEMPLO DE TESTE DE SERVICE USANDO CONTAINER COMPARTILHADO
func TestProductService(t *testing.T) {
	// ✅ Mesmo container compartilhado, mas em outro package
	suite := testhelper.NewIntegrationTestSuite(t)
	suite.Setup()
	defer suite.Teardown()
	
	// Setup da cadeia de dependências
	repo := repository.NewProductRepository(suite.ES())
	service := NewProductService(repo)
	ctx := context.Background()
	
	t.Run("Create Product with Validation", func(t *testing.T) {
		tenantID := testhelper.GenerateTenantID()

		product := &repository.Product{
			ID:          "service-test-1",
			Name:        "Service Test Product",
			Description: "Created via service",
			Price:       199.99,
			Category:    "electronics",
			TenantID:    tenantID,
		}
		
		err := service.CreateProduct(ctx, product)
		require.NoError(t, err)
		
		// Verifica se foi salvo
		retrieved, err := service.GetProductByID(ctx, "service-test-1", tenantID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		
		assert.Equal(t, product.Name, retrieved.Name)
		assert.Equal(t, product.Price, retrieved.Price)
	})
	
	t.Run("Validation Errors", func(t *testing.T) {
		tenantID := testhelper.GenerateTenantID()

		// Nome vazio
		product := &repository.Product{
			ID:       "invalid-1",
			Name:     "", // ❌ Inválido
			Price:    100.0,
			TenantID: tenantID,
		}
		
		err := service.CreateProduct(ctx, product)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
		
		// Preço negativo
		product.Name = "Valid Name"
		product.Price = -10.0 // ❌ Inválido
		
		err = service.CreateProduct(ctx, product)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "price must be positive")
	})
	
	t.Run("Get Products by Category", func(t *testing.T) {
		tenantID := testhelper.GenerateTenantID()
		// Cria múltiplos produtos
		products := []*repository.Product{
			{ID: "svc-1", Name: "Laptop", Category: "electronics2", Price: 1299.99,  TenantID: tenantID},
			{ID: "svc-2", Name: "Phone", Category: "electronics2", Price: 799.99,  TenantID: tenantID},
			{ID: "svc-3", Name: "Book", Category: "books", Price: 29.99,  TenantID: tenantID},
		}
		
		for _, p := range products {
			err := service.CreateProduct(ctx, p)
			require.NoError(t, err)
		}
		
		suite.WaitForIndexing()
		
		// Busca por categoria
		electronics, err := service.GetProductsByCategory(ctx, "electronics2", tenantID)
		require.NoError(t, err)
		
		assert.Len(t, electronics, 2)
		
		names := make([]string, len(electronics))
		for i, p := range electronics {
			names[i] = p.Name
		}
		assert.Contains(t, names, "Laptop")
		assert.Contains(t, names, "Phone")
	})
	
	t.Run("Get Expensive Products", func(t *testing.T) {
		tenantID := testhelper.GenerateTenantID()

		// Cria produtos com preços variados
		products := []*repository.Product{
			{ID: "exp-1", Name: "Cheap Phone", Category: "electronics", Price: 199.99, TenantID: tenantID},
			{ID: "exp-2", Name: "Premium Laptop", Category: "electronics", Price: 2499.99, TenantID: tenantID},
			{ID: "exp-3", Name: "Gaming PC", Category: "electronics", Price: 1599.99, TenantID: tenantID},
		}
		
		for _, p := range products {
			err := service.CreateProduct(ctx, p)
			require.NoError(t, err)
		}
		
		suite.WaitForIndexing()
		
		// Busca produtos caros (> 1000)
		expensive, err := service.GetExpensiveProducts(ctx, 1000.0, tenantID)
		require.NoError(t, err)
		
		assert.Len(t, expensive, 2)
		
		prices := make([]float64, len(expensive))
		for i, p := range expensive {
			prices[i] = p.Price
		}
		
		for _, price := range prices {
			assert.GreaterOrEqual(t, price, 1000.0)
		}
	})
	
	t.Run("Edge Cases", func(t *testing.T) {
		tenantID := testhelper.GenerateTenantID()

		// ID vazio
		_, err := service.GetProductByID(ctx, "", tenantID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ID is required")
		
		// Categoria vazia
		_, err = service.GetProductsByCategory(ctx, "", tenantID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "category is required")
		
		// Produto inexistente
		product, err := service.GetProductByID(ctx, "non-existent", tenantID)
		require.NoError(t, err)
		assert.Nil(t, product)
	})
}

// EXEMPLO DE TESTE INTEGRADO COMPLETO
func TestProductService_IntegratedWorkflow(t *testing.T) {
	suite := testhelper.NewIntegrationTestSuite(t)
	suite.Setup()
	defer suite.Teardown()
	
	repo := repository.NewProductRepository(suite.ES())
	service := NewProductService(repo)
	ctx := context.Background()
	
	t.Run("Complete Workflow", func(t *testing.T) {
		tenantID := testhelper.GenerateTenantID()

		// 1. Criar vários produtos
		products := []*repository.Product{
			{
				ID: "workflow-1", Name: "MacBook Pro", Category: "electronics",
				Description: "Apple laptop", Price: 2399.99, TenantID: tenantID,
			},
			{
				ID: "workflow-2", Name: "iPhone", Category: "electronics",
				Description: "Apple phone", Price: 999.99, TenantID: tenantID,
			},
			{
				ID: "workflow-3", Name: "AirPods", Category: "electronics",
				Description: "Wireless earbuds", Price: 199.99, TenantID: tenantID,
			},
			{
				ID: "workflow-4", Name: "Go Programming", Category: "books",
				Description: "Programming book", Price: 49.99, TenantID: tenantID,
			},
		}
		
		// Criar todos os produtos
		for _, p := range products {
			err := service.CreateProduct(ctx, p)
			require.NoError(t, err)
		}
		
		suite.WaitForIndexing()
		
		// 2. Verificar produtos por categoria
		electronics, err := service.GetProductsByCategory(ctx, "electronics", tenantID)
		require.NoError(t, err)
		assert.Len(t, electronics, 3)
		
		books, err := service.GetProductsByCategory(ctx, "books", tenantID)
		require.NoError(t, err)
		assert.Len(t, books, 1)
		
		// 3. Verificar produtos caros
		expensive, err := service.GetExpensiveProducts(ctx, 500.0, tenantID)
		require.NoError(t, err)
		assert.Len(t, expensive, 2) // MacBook e iPhone
		
		expensiveIDs := make([]string, len(expensive))
		for i, p := range expensive {
			expensiveIDs[i] = p.ID
		}
		assert.Contains(t, expensiveIDs, "workflow-1") // MacBook
		assert.Contains(t, expensiveIDs, "workflow-2") // iPhone
		
		// 4. Recuperar produto específico
		macbook, err := service.GetProductByID(ctx, "workflow-1", tenantID)
		require.NoError(t, err)
		require.NotNil(t, macbook)
		
		assert.Equal(t, "MacBook Pro", macbook.Name)
		assert.Equal(t, 2399.99, macbook.Price)
		assert.Equal(t, "Apple laptop", macbook.Description)
	})
}

// EXEMPLO DE BENCHMARK COMPARATIVO
func BenchmarkProductService_CreateAndSearch(b *testing.B) {
	suite := testhelper.NewIntegrationTestSuite(&testing.T{})
	suite.Setup()
	defer suite.Teardown()
	
	repo := repository.NewProductRepository(suite.ES())
	service := NewProductService(repo)
	ctx := context.Background()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		product := &repository.Product{
			ID:       fmt.Sprintf("bench-%d", i),
			Name:     fmt.Sprintf("Benchmark Product %d", i),
			Category: "benchmark",
			Price:    float64(i) * 10.0,
			TenantID: "bench_tenant",
		}
		
		err := service.CreateProduct(ctx, product)
		if err != nil {
			b.Fatal(err)
		}
		
		if i%10 == 0 { // A cada 10 produtos, faz uma busca
			_, err := service.GetProductsByCategory(ctx, "benchmark", "bench_tenant")
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}