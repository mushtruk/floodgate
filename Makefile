.PHONY: test
test:
	go test -v -race ./...

.PHONY: coverage
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: bench
bench:
	go test -bench=. -benchmem ./...

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: fmt
fmt:
	go fmt ./...
	goimports -w .

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: examples
examples:
	go run examples/basic/main.go

.PHONY: clean
clean:
	rm -f coverage.out coverage.html
	go clean -testcache

.PHONY: install-tools
install-tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  test          - Run tests with race detector"
	@echo "  coverage      - Generate coverage report"
	@echo "  bench         - Run benchmarks"
	@echo "  lint          - Run linters"
	@echo "  fmt           - Format code"
	@echo "  tidy          - Tidy go.mod"
	@echo "  examples      - Run examples"
	@echo "  clean         - Clean build artifacts"
	@echo "  install-tools - Install development tools"
