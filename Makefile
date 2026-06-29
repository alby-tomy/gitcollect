.PHONY: build test lint clean install release

build:
	go build -ldflags="-s -w -X main.version=$$(git describe --tags --always)" -o bin/gitcollect .

test:
	go test ./... -race -cover -coverprofile=coverage.out

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ dist/ coverage.out

install:
	go install -ldflags="-s -w" .

release:
	goreleaser release --clean
