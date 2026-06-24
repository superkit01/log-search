.PHONY: run build test tidy vet
run:
	go run ./cmd/server
build:
	mkdir -p bin && go build -o bin/log-search ./cmd/server
test:
	go test ./...
vet:
	go vet ./...
tidy:
	go mod tidy
