.PHONY: run build test test-web tidy vet
run:
	go run ./cmd/server
build:
	mkdir -p bin && go build -o bin/log-search ./cmd/server
test:
	go test ./...
test-web:
	node --test web/tests/*.test.js
vet:
	go vet ./...
tidy:
	go mod tidy
