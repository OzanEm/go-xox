.DEFAULT_GOAL := help
.PHONY: help tidy fmt vet lint test test-race cover build run dev up down logs ps db-up db-reset docker-build clean

# Load .env so `make run` / `make dev` see the same config the containers do.
ifneq (,$(wildcard .env))
include .env
export
endif

BIN := bin/server

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-14s\033[0m %s\n", $$1, $$2}'

## --- Go ----------------------------------------------------------------------

tidy: ## Sync go.mod / go.sum
	go mod tidy

fmt: ## Format all Go code
	go fmt ./...

vet: ## Run go vet
	go vet ./...

lint: fmt vet ## Format and vet

test: ## Run tests
	go test ./...

test-race: ## Run tests with the race detector
	go test -race ./...

cover: ## Run tests and open an HTML coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "wrote coverage.html"

build: ## Compile the server to bin/
	go build -o $(BIN) ./cmd/server

run: ## Run the server against a local Postgres (needs: make db-up)
	go run ./cmd/server

dev: ## Run with live reload via air (needs: make db-up)
	air

## --- Docker ------------------------------------------------------------------

up: ## Build and start the full stack (server + postgres)
	docker compose up --build -d
	@echo "api on http://localhost:$${HTTP_PORT:-8080}"

down: ## Stop the stack, keeping data
	docker compose down

logs: ## Tail the server logs
	docker compose logs -f server

ps: ## Show container status
	docker compose ps

db-up: ## Start only Postgres, for local `make run` / `make dev`
	docker compose up -d postgres

db-reset: ## Destroy the database volume and re-run migrations from scratch
	docker compose down -v
	docker compose up -d postgres

docker-build: ## Build the server image without starting anything
	docker compose build server

clean: ## Remove build artifacts
	rm -rf bin tmp out coverage.out coverage.html
