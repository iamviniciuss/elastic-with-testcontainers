package testhelper

/*
Este arquivo contém exemplos de como usar o TestDependenciesBuilder integrado ao testhelper.

IMPORTANTE: Este arquivo é apenas para documentação e exemplos.
Para usar nos testes, copie os exemplos para seus arquivos de teste.

========================================================================================
MIGRAÇÃO DO test/builder PARA testhelper
========================================================================================

O novo sistema mantém TOTAL compatibilidade com o código existente que usa apenas
Elasticsearch, enquanto adiciona suporte para MongoDB e PostgreSQL usando o padrão
Builder integrado aos containers compartilhados.

========================================================================================
EXEMPLOS DE USO
========================================================================================

1. PARA CÓDIGO EXISTENTE (SEM MUDANÇAS NECESSÁRIAS):
   
   Os testes que já usam NewIntegrationTestSuite continuam funcionando exatamente igual:

   func TestExistingElasticsearch(t *testing.T) {
       suite := testhelper.NewIntegrationTestSuite(t)
       suite.Setup() // Inicia Elasticsearch
       defer suite.Teardown()
       
       // Use suite.ES() normalmente
       client := suite.ES()
       // ... resto do teste igual
   }

2. PARA TESTES QUE PRECISAM APENAS DE PostgreSQL:

   func TestWithPostgresOnly(t *testing.T) {
       // Opção A: Usando Builder direto (igual ao test/builder)
       builder := testhelper.NewTestDependenciesBuilder()
       deps, err := builder.WithPostgres("path/to/init.sql").Build()
       require.NoError(t, err)
       defer deps.Cleanup()
       
       // Use deps.PostgresConn
       db := deps.PostgresConn
       // ... teste com PostgreSQL
   }

   // OU

   func TestWithPostgresUsingSuite(t *testing.T) {
       // Opção B: Usando IntegrationTestSuite + Builder
       suite, err := testhelper.NewIntegrationTestSuiteBuilder(t).
           WithPostgres("path/to/init.sql").
           Build()
       require.NoError(t, err)
       
       // Use suite.Postgres()
       db := suite.Postgres()
       suite.CleanPostgres() // Limpa entre subtestes
       // ... teste com PostgreSQL
   }

3. PARA TESTES QUE PRECISAM DE MongoDB E Elasticsearch:

   func TestWithMongoAndES(t *testing.T) {
       // Opção A: Builder direto
       builder := testhelper.NewTestDependenciesBuilder()
       deps, err := builder.WithMongo().WithElasticsearch().Build()
       require.NoError(t, err)
       defer deps.Cleanup()
       
       // Use deps.MongoConn, deps.ESConn
       mongoDB := deps.MongoConn
       esClient := deps.ESConn
       // ... teste
   }

   // OU

   func TestWithMongoAndESUsingSuite(t *testing.T) {
       // Opção B: Suite + Builder
       suite, err := testhelper.NewIntegrationTestSuiteBuilder(t).
           WithMongo().
           WithElasticsearch().
           Build()
       require.NoError(t, err)
       
       // Use suite.Mongo(), suite.ES()
       mongoDB := suite.Mongo()
       esClient := suite.ES()
       
       // Limpa dados entre subtestes
       suite.CleanAll() // Limpa tudo
       // OU individualmente:
       // suite.CleanMongo()
       // suite.CleanElasticsearch()
   }

4. PARA TESTES QUE PRECISAM DE TODAS AS DEPENDÊNCIAS:

   func TestWithAllDeps(t *testing.T) {
       suite, err := testhelper.NewIntegrationTestSuiteBuilder(t).
           WithPostgres("tickets_init.sql", "users_init.sql").
           WithMongo().
           WithElasticsearch().
           Build()
       require.NoError(t, err)
       
       // Todas as dependências disponíveis
       db := suite.Postgres()
       mongoDB := suite.Mongo()
       mongoDW := suite.MongoDW()
       esClient := suite.ES()
       
       // Executa teste complexo usando todas as dependências
       
       // Limpa tudo entre subtestes
       suite.CleanAll()
   }

5. COMPATIBILIDADE COM RESET ESPECÍFICO (igual ao builder original):

   func TestSpecificReset(t *testing.T) {
       builder := testhelper.NewTestDependenciesBuilder()
       deps, err := builder.WithMongo().WithPostgres().Build()
       require.NoError(t, err)
       defer deps.Cleanup()
       
       // Reset específico de coleções MongoDB (como no builder original)
       err = deps.ResetSpecificMongoCollections(ctx) // dw_surveys, ext_tickets, ext_boards
       require.NoError(t, err)
       
       // Reset sequences PostgreSQL
       err = deps.ResetPostgresSequences(ctx)
       require.NoError(t, err)
   }

6. USANDO VARIÁVEIS DE AMBIENTE:

   Para usar dependências externas (igual ao shared_container existente):
   
   export USE_EXTERNAL_ES=true ES_URL=http://localhost:9200
   export USE_EXTERNAL_MONGO=true MONGO_URL=mongodb://localhost:27017
   export USE_EXTERNAL_PG=true PG_URL="host=localhost port=5432 user=test password=test sslmode=disable"
   
   O sistema automaticamente usa as instâncias externas quando configurado.

7. EXAMPLE COM SUBTESTS E ISOLAMENTO:

   func TestCompleteFeature(t *testing.T) {
       suite, err := testhelper.NewIntegrationTestSuiteBuilder(t).
           WithPostgres("schema.sql").
           WithMongo().
           WithElasticsearch().
           Build()
       require.NoError(t, err)
       
       t.Run("CreateUser", func(t *testing.T) {
           suite.CleanAll() // Isolamento
           
           // Test user creation
           db := suite.Postgres()
           mongo := suite.Mongo()
           es := suite.ES()
           
           // ... implementação do teste
       })
       
       t.Run("SearchUser", func(t *testing.T) {
           suite.CleanAll() // Isolamento
           
           // Setup data
           // ... test search functionality
       })
       
       t.Run("UpdateUser", func(t *testing.T) {
           suite.CleanAll() // Isolamento
           
           // ... test update functionality
       })
   }

========================================================================================
VANTAGENS DA NOVA IMPLEMENTAÇÃO
========================================================================================

1. COMPATIBILIDADE TOTAL: Código existente não precisa de mudanças
2. CONTAINERS COMPARTILHADOS: Mesmo benefício de performance do shared_container.go
3. INICIALIZAÇÃO PARALELA: MongoDB, PostgreSQL e Elasticsearch iniciam em paralelo
4. BUILDER PATTERN: Configuração fluente e flexível
5. ISOLAMENTO: Cada teste pode limpar apenas o que precisa
6. REUTILIZAÇÃO: Containers são reutilizados entre execuções (se configurado)
7. DEBUGGING: Suporte a DEBUG_TEST_CONTAINERS para logs detalhados
8. DEPENDÊNCIAS EXTERNAS: Suporte para usar instâncias externas via env vars

========================================================================================
VARIÁVEIS DE AMBIENTE SUPORTADAS
========================================================================================

# Elasticsearch
USE_EXTERNAL_ES=true
ES_URL=http://localhost:9200

# MongoDB  
USE_EXTERNAL_MONGO=true
MONGO_URL=mongodb://localhost:27017

# PostgreSQL
USE_EXTERNAL_PG=true
PG_URL="host=localhost port=5432 user=test password=test sslmode=disable"

# Debugging e Comportamento
DEBUG_TEST_CONTAINERS=true     # Ativa logs detalhados
TEST_CONTAINER_REUSE=true      # Reutiliza containers (padrão: true)

========================================================================================
MIGRAÇÃO DO test/builder
========================================================================================

Para migrar do test/builder para testhelper, mude:

ANTES (test/builder):
   builder := setup_tests.NewTestDependenciesBuilder()
   deps, err := builder.WithPostgres().WithMongo().Build()

DEPOIS (testhelper):
   builder := testhelper.NewTestDependenciesBuilder()  
   deps, err := builder.WithPostgres().WithMongo().Build()

A API é idêntica, mas agora usa containers compartilhados e é mais eficiente!

========================================================================================
*/