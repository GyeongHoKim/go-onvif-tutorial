GOLANGCI_LINT_VERSION := v2.11.4

.PHONY: lint format

lint:
	golangci-lint run ./...

format:
	golangci-lint fmt ./...
