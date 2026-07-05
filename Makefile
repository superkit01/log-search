.PHONY: run build test tidy vet seed-loki seed-es
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
seed-loki:
	@echo "生成 Loki 测试数据..."
	@./scripts/seed_loki.py
seed-es:
	@echo "生成 ES 测试数据..."
	@ES_URL=$${ES_URL:-http://localhost:9200} ./scripts/seed_es.py --url $${ES_URL:-http://localhost:9200}
