.PHONY: build test vet lint staticcheck govulncheck check run tidy clean

BIN_DIR := bin

build:
	CGO_ENABLED=0 go build -o $(BIN_DIR)/server ./cmd/server
	# cmd/admin dodawany w Etapie 2

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
	go run ./cmd/server

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)
