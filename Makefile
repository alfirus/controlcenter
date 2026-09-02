.PHONY: gen migrate\:up migrate\:down supabase\:start supabase\:stop lint test build

gen:
	@echo ">> oapi-codegen (requires: go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest)"
	oapi-codegen -config backend/api/oapi-codegen.yaml backend/api/openapi.yaml

migrate\:up:
	migrate -path backend/migrations -database "$$DATABASE_URL" up

migrate\:down:
	migrate -path backend/migrations -database "$$DATABASE_URL" down 1

migrate\:seed:
	psql "$$DATABASE_URL" -f backend/migrations/seed.sql

db\:up:
	docker compose up -d db

db\:down:
	docker compose down

supabase\:start:
	supabase start

supabase\:stop:
	supabase stop

lint:
	golangci-lint run ./backend/...

test:
	go test ./backend/... -count=1

build:
	go build -o backend/bin/server ./backend/cmd/server
