# Container Compartilhado para Testes - Implementação

Este projeto demonstra a implementação de um sistema de container compartilhado para testes de integração em Go, conforme especificado no [CLAUDE.md](./CLAUDE.md).

## 🚀 Funcionalidades Implementadas

- ✅ **Container Elasticsearch Compartilhado**: Singleton pattern com reference counting
- ✅ **Suite de Testes Base**: Helpers para operações comuns e isolamento de dados  
- ✅ **Makefile Completo**: Comandos para diferentes cenários de teste
- ✅ **Exemplos de Migração**: Comparação entre modelo antigo e novo
- ✅ **Suporte a ES Externo**: Para ambientes de CI/CD
- ✅ **Isolamento de Dados**: Limpeza automática entre testes
- ✅ **Testes Paralelos**: Suporte seguro para execução paralela

## 📦 Estrutura do Projeto

```
├── test/testhelper/                 # Helpers compartilhados
│   ├── shared_container.go          # Gerenciador do container singleton  
│   └── integration_test_base.go     # Suite base para testes
├── internal/
│   ├── repository/                  # Exemplos de repository
│   │   ├── product_repository.go
│   │   ├── product_repository_test.go      # ✅ Novo modelo
│   │   └── product_repository_old_test.go  # ❌ Modelo antigo (comparação)
│   └── service/                     # Exemplos de service
│       ├── product_service.go
│       └── product_service_test.go         # ✅ Novo modelo
├── Makefile                         # Comandos de teste e utilitários
└── go.mod                           # Dependências
```

## 🏃‍♂️ Como Executar

### Instalação das Dependências

```bash
make deps
```

### Executar Testes de Integração

```bash
# Container compartilhado (recomendado)
make test-integration

# Sem reutilização de container
make test-integration-clean

# Com Elasticsearch externo
make test-integration-external

# Todos os testes
make test-all
```

### Outros Comandos Úteis

```bash
# Testes com debug
make test-debug

# Relatório de cobertura
make test-coverage

# Demonstração de performance
make demo-before-after

# Verificar containers ativos
make check-containers

# Ajuda completa
make help
```

## 🔧 Configuração

### Variáveis de Ambiente

| Variável | Descrição | Padrão |
|----------|-----------|---------|
| `USE_EXTERNAL_ES` | Usa Elasticsearch externo | `false` |
| `ES_URL` | URL do ES externo | `http://localhost:9200` |
| `DEBUG_TEST_CONTAINERS` | Ativa logs de debug | `false` |
| `TEST_CONTAINER_REUSE` | Reutiliza containers | `true` |

### Exemplo de Uso com ES Externo

```bash
# Para CI/CD ou desenvolvimento local
USE_EXTERNAL_ES=true ES_URL=http://localhost:9200 go test ./...
```

## 📊 Comparação de Performance

### Modelo Antigo (❌)
```go
func TestOldWay(t *testing.T) {
    // Cada teste cria seu próprio container
    container, err := elasticsearch.RunContainer(ctx, ...)
    defer container.Terminate(ctx)
    // ...
}
```

**Problemas:**
- 10 packages = 10 containers
- ~5 minutos de tempo total  
- ~5GB RAM utilizada
- Sem isolamento adequado

### Modelo Novo (✅)
```go 
func TestNewWay(t *testing.T) {
    // Usa container compartilhado
    suite := testhelper.NewIntegrationTestSuite(t)
    suite.Setup() // Isolamento automático
    defer suite.Teardown()
    // ...
}
```

**Benefícios:**
- 10 packages = 1 container compartilhado
- ~1.5 minutos de tempo total
- ~1GB RAM utilizada  
- Isolamento garantido

## 🧪 Exemplos de Uso

### 1. Teste Básico de Repository

```go
func TestProductRepository(t *testing.T) {
    suite := testhelper.NewIntegrationTestSuite(t)
    suite.Setup()
    defer suite.Teardown()
    
    repo := NewProductRepository(suite.ES())
    
    // Seus testes aqui...
}
```

### 2. Usando Helpers do TestHelper

```go
func TestWithHelpers(t *testing.T) {
    suite := testhelper.NewIntegrationTestSuite(t)
    suite.Setup()
    defer suite.Teardown()
    
    // Indexa documento
    suite.IndexDocument("products", "1", product)
    suite.WaitForIndexing()
    
    // Busca documentos
    results := suite.SearchDocuments("products", query)
    assert.Equal(t, 1, results.TotalHits())
}
```

### 3. Testes Paralelos

```go
func TestParallel(t *testing.T) {
    t.Parallel() // Seguro com container compartilhado
    
    suite := testhelper.NewIntegrationTestSuite(t)
    suite.Setup() // Dados isolados automaticamente
    defer suite.Teardown()
    
    // Seus testes aqui...
}
```

## 🔍 Debugging e Troubleshooting

### Ativar Logs de Debug

```bash
DEBUG_TEST_CONTAINERS=true make test-integration
```

### Verificar Containers Ativos

```bash
make check-containers
```

### Limpar Containers Órfãos

```bash
make clean
```

### Problemas Comuns

1. **Container não compartilhado**: Verificar flag `Reuse: true` 
2. **Testes interferindo**: Garantir que `Setup()` está sendo chamado
3. **Container órfão**: Usar `make stop-test-containers`

## 📈 Próximos Passos

1. **Implementar no seu projeto**: Adaptar os exemplos para sua arquitetura
2. **Adicionar outros serviços**: Aplicar padrão para Redis, PostgreSQL, etc.
3. **CI/CD**: Configurar pipelines com ES externo
4. **Monitoramento**: Adicionar métricas de performance dos testes

## 🤝 Contribuindo

1. Faça fork do projeto
2. Crie uma branch para sua feature
3. Commit suas mudanças
4. Abra um Pull Request

## 📝 Licença

Este projeto é um exemplo educacional baseado no guia do CLAUDE.md.# elastic-with-testcontainers
