
.PHONY: test
test:
	go test ./...

.PHONY: vet
	go vet ./...

.PHONY: lint
lint:
	golangci-lint run ./...
