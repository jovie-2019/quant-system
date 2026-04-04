.PHONY: build test vet lint up down logs ps

# --- Go ---
build:
	go build ./...

test:
	go test ./... -race -count=1

vet:
	go vet ./...

lint: vet
	@echo "lint passed (go vet)"

check: build vet test
	@echo "all checks passed"

# --- Docker Compose ---
up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

ps:
	docker compose ps

rebuild:
	docker compose up -d --build

# --- Frontend ---
web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build
