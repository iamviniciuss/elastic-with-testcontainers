package main

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/viniciussantos/claude-testcontainers/test/testhelper"
)

// TestMain coordena o ciclo de vida dos recursos compartilhados
func TestMain(m *testing.M) {
	ctx := context.Background()
	
	// Configura hooks de limpeza
	setupCleanupHooks(ctx)
	
	// Executa os testes
	exitCode := m.Run()
	
	// Limpa recursos compartilhados
	cleanup(ctx)
	
	// Finaliza com código de saída apropriado
	os.Exit(exitCode)
}

func setupCleanupHooks(ctx context.Context) {
	// Intercepta sinais para cleanup gracioso
	// Em produção, você pode adicionar signal handling aqui
}

func cleanup(ctx context.Context) {
	fmt.Println("🧹 Cleaning up shared test resources...")
	
	// Limpa recursos do Elasticsearch compartilhado
	if err := testhelper.CleanupSharedResources(ctx); err != nil {
		fmt.Printf("⚠️  Warning: failed to cleanup shared resources: %v\n", err)
	}
	
	fmt.Println("✅ Cleanup completed")
}

// Exemplo de teste no package principal (se necessário)
func TestMain_Integration(t *testing.T) {
	t.Skip("This is just an example - remove if not needed")
	
	suite := testhelper.NewIntegrationTestSuite(t)
	suite.Setup()
	defer suite.Teardown()
	
	// Testa conectividade básica
	client := suite.ES()
	if client == nil {
		t.Fatal("Elasticsearch client should not be nil")
	}
	
	// Testa se consegue fazer uma operação básica
	res, err := client.Info()
	if err != nil {
		t.Fatalf("Failed to get Elasticsearch info: %v", err)
	}
	defer res.Body.Close()
	
	if res.IsError() {
		t.Fatalf("Elasticsearch returned error: %s", res.Status())
	}
	
	t.Log("✅ Main integration test passed")
}