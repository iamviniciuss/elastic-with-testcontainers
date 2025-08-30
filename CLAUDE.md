# Guia de Implementação: Container Compartilhado para Testes em Go

## 📋 Visão Geral

Este guia detalha como implementar um sistema de container compartilhado para testes de integração em Go, resolvendo o problema de múltiplos containers do Elasticsearch sendo criados desnecessariamente durante a execução dos testes.

## 🎯 Objetivo

- **Problema**: Cada package de teste cria seu próprio container do Elasticsearch via Testcontainers
- **Solução**: Compartilhar um único container entre todos os testes, mantendo o isolamento dos dados
- **Benefícios**: Redução de 70-90% no tempo de execução e uso de recursos

## 🏗️ Arquitetura da Solução

```
┌─────────────────────────────────────────────┐
│            Shared Container Manager          │
│                                              │
│  ┌──────────────┐      ┌──────────────┐    │
│  │   Singleton  │──────│  Testcontainer│    │
│  │   Instance   │      │  Elasticsearch│    │
│  └──────────────┘      └──────────────┘    │
│         ▲                                   │
│         │ Reference Counting                │
└─────────┼───────────────────────────────────┘
          │
    ┌─────┴─────┬──────────┬──────────┐
    │ Package A │ Package B │ Package C │
    │  Tests    │  Tests    │  Tests    │
    └───────────┴───────────┴───────────┘
```

## 📦 Estrutura de Diretórios

```
projeto/
├── test/
│   └── testhelper/
│       ├── shared_container.go      # Gerenciador principal do container
│       ├── integration_test_base.go # Suite base para testes
│       └── test_fixtures.go         # Fixtures e dados de teste (opcional)
├── internal/
│   ├── repository/
│   │   └── *_test.go               # Testes usando o helper
│   └── service/
│       └── *_test.go               # Testes usando o helper
├── Makefile                        # Comandos para facilitar execução
├── go.mod
└── go.sum
```

## 🚀 Passos de Implementação

### 1. Instalar Dependências

```bash
# Testcontainers
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/elasticsearch

# Cliente Elasticsearch
go get github.com/elastic/go-elasticsearch/v8

# Testing helpers (opcional mas recomendado)
go get github.com/stretchr/testify
```

### 2. Criar o Gerenciador de Container Compartilhado

**Arquivo**: `test/testhelper/shared_container.go`

#### Principais componentes:

- **Singleton Pattern**: Garante única instância do container
- **Reference Counting**: Controla quantos testes estão usando
- **Lazy Initialization**: Container criado apenas quando necessário
- **Reuse Flag**: Permite reutilizar container existente entre execuções

#### Funcionalidades essenciais:

```go
type SharedElasticsearch struct {
    mu          sync.RWMutex      // Thread-safety
    container   testcontainers.Container
    client      *elasticsearch.Client
    url         string
    refCount    int32              // Contador de referências
    startOnce   sync.Once          // Garante inicialização única
}
```

### 3. Implementar a Suite de Testes Base

**Arquivo**: `test/testhelper/integration_test_base.go`

#### Responsabilidades:

- Abstrair a complexidade do container compartilhado
- Fornecer métodos helper para operações comuns
- Garantir limpeza de dados entre testes
- Gerenciar ciclo de vida dos recursos

### 4. Configurar o Makefile

**Arquivo**: `Makefile`

Adicionar comandos para diferentes cenários de teste:

```makefile
test-unit:          # Testes rápidos sem dependências
test-integration:   # Testes com container compartilhado
test-all:          # Todos os testes
test-coverage:     # Com relatório de cobertura
```

### 5. Adaptar os Testes Existentes

#### Antes (código atual):
```go
func TestRepository(t *testing.T) {
    ctx := context.Background()
    
    // Cada teste cria seu próprio container
    container, err := elasticsearch.RunContainer(ctx, ...)
    defer container.Terminate(ctx)
    
    // ... resto do teste
}
```

#### Depois (código novo):
```go
func TestRepository(t *testing.T) {
    // Usa o container compartilhado
    suite := testhelper.NewIntegrationTestSuite(t)
    suite.Setup() // Limpa dados para isolamento
    
    // ... resto do teste com suite.ES (cliente)
}
```

## 🔧 Configurações e Variáveis de Ambiente

### Variáveis Suportadas

| Variável | Descrição | Valor Padrão |
|----------|-----------|--------------|
| `USE_EXTERNAL_ES` | Usa ES externo ao invés de Testcontainer | `false` |
| `ES_URL` | URL do ES externo | - |
| `DEBUG_TEST_CONTAINERS` | Ativa logs de debug | `false` |
| `TEST_CONTAINER_REUSE` | Reutiliza containers entre execuções | `true` |

### Exemplo de Uso

```bash
# Usar Elasticsearch externo (CI/CD)
USE_EXTERNAL_ES=true ES_URL=http://localhost:9200 go test ./...

# Debug de containers
DEBUG_TEST_CONTAINERS=true go test -v ./...
```

## 📊 Estratégias de Isolamento

### 1. Limpeza Completa (Recomendado)
```go
func (s *IntegrationTestSuite) Setup() {
    // Deleta todos os índices antes de cada teste
    s.CleanElasticsearch()
}
```

### 2. Namespacing
```go
func (s *IntegrationTestSuite) Setup() {
    // Cada teste usa prefixo único
    s.indexPrefix = fmt.Sprintf("test_%s_", uuid.New())
}
```

### 3. Snapshots (Avançado)
```go
func (s *IntegrationTestSuite) Setup() {
    // Cria snapshot do estado limpo
    s.CreateSnapshot("clean_state")
}

func (s *IntegrationTestSuite) Teardown() {
    // Restaura snapshot
    s.RestoreSnapshot("clean_state")
}
```

## 🎨 Melhores Práticas

### 1. Organização dos Testes

```go
// Agrupe testes relacionados em suites
type ProductTestSuite struct {
    *testhelper.IntegrationTestSuite
    repo *ProductRepository
}

func (s *ProductTestSuite) SetupSuite() {
    // Setup único para toda a suite
}

func (s *ProductTestSuite) SetupTest() {
    // Setup antes de cada teste
    s.IntegrationTestSuite.Setup()
}
```

### 2. Fixtures e Dados de Teste

```go
// test/testhelper/fixtures.go
func LoadProductFixtures(suite *IntegrationTestSuite) []Product {
    products := []Product{
        {ID: "1", Name: "Product A"},
        {ID: "2", Name: "Product B"},
    }
    
    for _, p := range products {
        suite.IndexDocument("products", p.ID, p)
    }
    
    suite.WaitForIndexing()
    return products
}
```

### 3. Paralelização Segura

```go
func TestParallel(t *testing.T) {
    t.Parallel() // Marca como paralelizável
    
    suite := testhelper.NewIntegrationTestSuite(t)
    // Cada teste tem seu próprio índice
    indexName := fmt.Sprintf("test_%d", time.Now().UnixNano())
    suite.CreateIndex(indexName, mapping)
}
```

## 🚨 Troubleshooting

### Problema: Container não é compartilhado

**Sintomas**: Múltiplos containers sendo criados

**Soluções**:
1. Verificar se o nome do container está fixo
2. Adicionar flag `Reuse: true` no ContainerRequest
3. Verificar logs com `DEBUG_TEST_CONTAINERS=true`

### Problema: Testes interferindo uns com os outros

**Sintomas**: Testes falhando aleatoriamente

**Soluções**:
1. Garantir que `Setup()` está sendo chamado
2. Usar índices com nomes únicos
3. Adicionar `WaitForIndexing()` após inserções

### Problema: Container não é limpo após testes

**Sintomas**: Container continua rodando

**Soluções**:
1. Implementar `TestMain` com cleanup
2. Usar `defer CleanupSharedResources()`
3. Configurar timeout de cleanup automático

## 📈 Métricas de Performance

### Antes da Implementação
- 10 packages de teste
- 10 containers Elasticsearch criados
- ~5 minutos de tempo total
- ~5GB de RAM utilizada

### Depois da Implementação
- 10 packages de teste
- 1 container Elasticsearch compartilhado
- ~1.5 minutos de tempo total
- ~1GB de RAM utilizada

## 🔄 Processo de Migração

### Fase 1: Preparação (1-2 dias)
1. Criar estrutura de diretórios `test/testhelper`
2. Implementar `shared_container.go`
3. Implementar `integration_test_base.go`
4. Configurar Makefile

### Fase 2: Migração Gradual (1 semana)
1. Escolher 1-2 packages piloto
2. Migrar testes para nova estrutura
3. Validar funcionamento e performance
4. Documentar problemas encontrados

### Fase 3: Migração Completa (2-3 semanas)
1. Migrar remaining packages
2. Atualizar CI/CD pipelines
3. Treinar equipe
4. Monitorar métricas

## 🔍 Validação e Testes

### Checklist de Validação

- [ ] Container único é criado para múltiplos packages
- [ ] Testes mantêm isolamento de dados
- [ ] Performance melhorou (tempo e recursos)
- [ ] CI/CD funciona corretamente
- [ ] Cleanup automático funciona
- [ ] Testes paralelos funcionam
- [ ] Modo de ES externo funciona

### Comandos de Teste

```bash
# Testar se container é compartilhado
make test-integration 2>&1 | grep "Starting shared Elasticsearch" | wc -l
# Deve retornar 1

# Testar isolamento
go test -v ./internal/repository/... ./internal/service/...

# Testar cleanup
make test-all && docker ps | grep elasticsearch
# Não deve retornar containers órfãos
```

## 📚 Recursos Adicionais

### Documentação
- [Testcontainers for Go](https://golang.testcontainers.org/)
- [Elasticsearch Go Client](https://github.com/elastic/go-elasticsearch)
- [Go Testing Package](https://pkg.go.dev/testing)

### Exemplos de Projetos
- [testcontainers-go examples](https://github.com/testcontainers/testcontainers-go/tree/main/examples)
- [Integration Testing Best Practices](https://github.com/golang/go/wiki/Integration-Testing)

## 🎯 Próximos Passos

1. **Curto Prazo**
   - Implementar a solução básica
   - Migrar 2-3 packages como prova de conceito
   - Coletar métricas de performance

2. **Médio Prazo**
   - Migrar todos os packages
   - Adicionar suporte para outros serviços (Redis, PostgreSQL)
   - Implementar dashboard de métricas

3. **Longo Prazo**
   - Criar biblioteca interna reutilizável
   - Adicionar suporte para ambientes de teste distribuídos
   - Implementar cache de fixtures

## ❓ FAQ

**Q: E se eu precisar de versões diferentes do Elasticsearch?**
A: Pode-se criar múltiplos singletons com chaves diferentes ou usar tags de build.

**Q: Como funciona em CI/CD?**
A: Use a variável `USE_EXTERNAL_ES` para apontar para um ES gerenciado pelo CI.

**Q: Posso aplicar isso para outros serviços?**
A: Sim! O padrão é genérico e funciona para Redis, PostgreSQL, MongoDB, etc.

**Q: E se um teste travar e não liberar o container?**
A: O reference counting com timeout automático garante limpeza eventual.