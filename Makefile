.PHONY: build test vet lint staticcheck govulncheck check run create-user set-password import-samples tidy clean

BIN_DIR := bin

# Wczytanie zmiennych z .env, jeśli plik istnieje.
load_env = if [ -f .env ]; then set -a; . ./.env; set +a; fi

build:
	CGO_ENABLED=0 go build -o $(BIN_DIR)/server ./cmd/server
	CGO_ENABLED=0 go build -o $(BIN_DIR)/admin ./cmd/admin

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

govulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

check: lint test govulncheck

run:
	@$(load_env); go run ./cmd/server

# Użycie: make create-user USER=genetyk
create-user:
	@$(load_env); go run ./cmd/admin create-user $(USER)

# Użycie: make set-password USER=genetyk
set-password:
	@$(load_env); go run ./cmd/admin set-password $(USER)

# Użycie: make import-samples CSV=testdata/samples.csv
import-samples:
	@$(load_env); go run ./cmd/admin import-samples $(CSV)

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)
