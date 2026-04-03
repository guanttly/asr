.PHONY: all build-backend build-frontend dev-backend dev-frontend lint clean

# ============================================================
# 语音转写系统 Monorepo Makefile
# ============================================================

# --- Backend ---
.PHONY: build-gateway build-asr-api build-admin-api build-nlp-api

build-backend:
	cd backend && go build -o ../bin/gateway ./cmd/gateway
	cd backend && go build -o ../bin/asr-api ./cmd/asr-api
	cd backend && go build -o ../bin/admin-api ./cmd/admin-api
	cd backend && go build -o ../bin/nlp-api ./cmd/nlp-api

build-gateway:
	cd backend && go build -o ../bin/gateway ./cmd/gateway

build-asr-api:
	cd backend && go build -o ../bin/asr-api ./cmd/asr-api

build-admin-api:
	cd backend && go build -o ../bin/admin-api ./cmd/admin-api

build-nlp-api:
	cd backend && go build -o ../bin/nlp-api ./cmd/nlp-api

dev-gateway:
	cd backend && go run ./cmd/gateway

dev-asr-api:
	cd backend && go run ./cmd/asr-api

dev-admin-api:
	cd backend && go run ./cmd/admin-api

dev-nlp-api:
	cd backend && go run ./cmd/nlp-api

# --- Frontend ---
dev-frontend:
	cd frontend && pnpm dev

build-frontend:
	cd frontend && pnpm build

# --- Quality ---
lint:
	cd frontend && pnpm lint
	cd backend && golangci-lint run ./...

# --- Docker ---
docker-up:
	cd deploy && docker compose up -d

docker-down:
	cd deploy && docker compose down

docker-build:
	cd deploy && docker compose build

# --- Utilities ---
clean:
	rm -rf bin/
	cd frontend && rm -rf dist node_modules/.vite

all: build-backend build-frontend
