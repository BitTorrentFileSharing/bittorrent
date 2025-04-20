.PHONY: install-lint

install-lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.5

lint: install-lint
	golangci-lint run -c ./golangci.yaml
