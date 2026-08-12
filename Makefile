
.PHONY: test
test:
	go test -race ./...

.PHONY: vet
	go vet ./...

.PHONY: lint
lint:
	golangci-lint run ./...
