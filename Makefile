.PHONY: test-unit test-integration test-all test-coverage clean deps help

# Variáveis
GO_FILES := $(shell find . -name "*.go" -type f)
TEST_TIMEOUT := 60s
COVERAGE_FILE := coverage.out

# Comandos principais
help: ## Mostra esta mensagem de ajuda
	@echo "Comandos disponíveis:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

deps: ## Instala dependências do projeto
	@echo "📦 Instalando dependências..."
	go mod tidy
	go mod download

test-unit: ## Executa apenas testes unitários (sem containers)
	@echo "🧪 Executando testes unitários..."
	go test -timeout $(TEST_TIMEOUT) -short ./...

test-integration: ## Executa testes de integração com container compartilhado
	@echo "🐳 Executando testes de integração..."
	TEST_CONTAINER_REUSE=true go test -v -timeout $(TEST_TIMEOUT) -v ./internal/...

test-integration-clean: ## Executa testes de integração sem reutilizar containers
	@echo "🐳 Executando testes de integração (sem reutilização)..."
	TEST_CONTAINER_REUSE=false go test -timeout $(TEST_TIMEOUT) -v ./internal/...

test-integration-external: ## Executa testes usando Elasticsearch externo
	@echo "🔗 Executando testes com Elasticsearch externo..."
	USE_EXTERNAL_ES=true ES_URL=http://localhost:9209 go test -timeout $(TEST_TIMEOUT) -v ./internal/... -count=1

test-all: ## Executa todos os testes
	@echo "🚀 Executando todos os testes..."
	TEST_CONTAINER_REUSE=true go test -timeout $(TEST_TIMEOUT) ./...

test-coverage: ## Executa testes com relatório de cobertura
	@echo "📊 Executando testes com cobertura..."
	TEST_CONTAINER_REUSE=true go test -timeout $(TEST_TIMEOUT) -coverprofile=$(COVERAGE_FILE) ./...
	go tool cover -html=$(COVERAGE_FILE) -o coverage.html
	@echo "📄 Relatório de cobertura gerado em coverage.html"

test-parallel: ## Executa testes em paralelo (demonstração)
	@echo "⚡ Executando testes em paralelo..."
	TEST_CONTAINER_REUSE=true go test -timeout $(TEST_TIMEOUT) -parallel 4 ./...

test-debug: ## Executa testes com logs de debug
	@echo "🔍 Executando testes com debug..."
	DEBUG_TEST_CONTAINERS=true TEST_CONTAINER_REUSE=true go test -timeout $(TEST_TIMEOUT) -v ./...

test-benchmark: ## Executa benchmarks comparando antes/depois
	@echo "⏱️ Executando benchmarks..."
	go test -bench=. -benchmem ./...

clean: ## Remove arquivos temporários e containers órfãos
	@echo "🧹 Limpando arquivos temporários..."
	rm -f $(COVERAGE_FILE) coverage.html
	go clean -testcache
	@echo "🐳 Removendo containers órfãos..."
	-docker container prune -f
	-docker volume prune -f

fmt: ## Formata código Go
	@echo "🎨 Formatando código..."
	go fmt ./...

lint: ## Executa linter (se disponível)
	@echo "🔍 Executando linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "⚠️  golangci-lint não encontrado. Instalando..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
		golangci-lint run; \
	fi

vet: ## Executa go vet
	@echo "🔍 Executando go vet..."
	go vet ./...

# Comandos para desenvolvimento
dev-setup: deps ## Configura ambiente de desenvolvimento
	@echo "⚙️ Configurando ambiente de desenvolvimento..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "✅ Ambiente configurado!"

dev-test: fmt vet test-integration ## Executa pipeline completo de desenvolvimento
	@echo "✅ Pipeline de desenvolvimento concluído!"

# Comandos para Elasticsearch externo via Docker Compose
es-up: ## Inicia Elasticsearch via Docker Compose
	@echo "🚀 Iniciando Elasticsearch com Docker Compose..."
	docker-compose up -d elasticsearch
	@echo "⏳ Aguardando Elasticsearch ficar pronto..."
	@for i in $$(seq 1 30); do \
		if curl -f http://localhost:9209/_cluster/health >/dev/null 2>&1; then \
			echo "✅ Elasticsearch pronto em http://localhost:9209"; \
			exit 0; \
		fi; \
		sleep 2; \
	done; \
	echo "❌ Elasticsearch não ficou pronto em 60s"; \
	exit 1

es-down: ## Para Elasticsearch via Docker Compose
	@echo "🛑 Parando Elasticsearch..."
	docker-compose down

es-restart: ## Reinicia Elasticsearch via Docker Compose
	@echo "🔄 Reiniciando Elasticsearch..."
	docker-compose restart elasticsearch

es-logs: ## Mostra logs do Elasticsearch
	@echo "📋 Logs do Elasticsearch:"
	docker-compose logs -f elasticsearch

es-status: ## Verifica status do Elasticsearch
	@echo "📊 Status do Elasticsearch:"
	@curl -f http://localhost:9209/_cluster/health?pretty 2>/dev/null || echo "❌ Elasticsearch não está acessível"

test-with-compose: es-up test-integration-external es-down ## Executa testes completos com Docker Compose

# Comandos para CI/CD
ci-test: ## Comando otimizado para CI/CD
	@echo "🤖 Executando testes no CI/CD..."
	USE_EXTERNAL_ES=true ES_URL=http://elasticsearch:9200 go test -timeout $(TEST_TIMEOUT) -v ./...

# Demonstrações e exemplos
demo-before-after: ## Demonstra diferença de performance antes/depois
	@echo "📈 Demonstração de performance..."
	@echo "🔴 ANTES: Cada teste cria seu próprio container"
	TEST_CONTAINER_REUSE=false time go test -timeout $(TEST_TIMEOUT) ./internal/repository/...
	@echo ""
	@echo "🟢 DEPOIS: Container compartilhado"
	TEST_CONTAINER_REUSE=true time go test -timeout $(TEST_TIMEOUT) ./internal/repository/...

demo-isolation: ## Demonstra isolamento entre testes
	@echo "🔒 Demonstração de isolamento..."
	DEBUG_TEST_CONTAINERS=true TEST_CONTAINER_REUSE=true go test -timeout $(TEST_TIMEOUT) -v -run "TestIsolation" ./...

# Utilitários
check-containers: ## Lista containers de teste ativos
	@echo "🐳 Containers de teste ativos:"
	docker ps --filter "name=elasticsearch" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

stop-test-containers: ## Para todos os containers de teste
	@echo "🛑 Parando containers de teste..."
	-docker stop $$(docker ps -q --filter "name=shared-elasticsearch-test")
	-docker rm $$(docker ps -aq --filter "name=shared-elasticsearch-test")

# Validações
validate: fmt vet lint test-all ## Executa todas as validações do projeto
	@echo "✅ Todas as validações passaram!"

# Informações do sistema
info: ## Mostra informações do ambiente
	@echo "ℹ️ Informações do ambiente:"
	@echo "Go version: $$(go version)"
	@echo "Docker version: $$(docker --version 2>/dev/null || echo 'Docker não disponível')"
	@echo "Testcontainers: $$(go list -m github.com/testcontainers/testcontainers-go 2>/dev/null || echo 'não instalado')"
	@echo "Elasticsearch client: $$(go list -m github.com/elastic/go-elasticsearch/v8 2>/dev/null || echo 'não instalado')"