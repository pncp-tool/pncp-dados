.PHONY: build vet test lint fmt migrate run-harvest

build:
	go build ./...

vet:
	go vet ./...

test:
	go test ./...

lint:
	go vet ./...

fmt:
	gofmt -l -w .

migrate:
	go run ./cmd/migrate

run-harvest:
	go run ./cmd/harvest $(ARGS)