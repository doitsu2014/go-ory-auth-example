# identity-service developer commands (docs/principles/02-backend-go.md §8).
.PHONY: generate build vet test test-integration run migrate

generate: ## oapi-codegen + sqlc (generated code is committed)
	go tool oapi-codegen -config oapi-codegen.yaml ../../api/openapi/identity-service.v1.yaml
	go tool sqlc generate

build:
	go build ./...

vet:
	go vet ./... && go vet -tags integration ./...

test: ## unit tests
	go test -race ./...

test-integration: ## needs the local stack: `make up` at the repo root
	go test -race -count=1 -tags integration ./...

run:
	go run ./cmd/identity-service serve

migrate:
	go run ./cmd/identity-service migrate up
