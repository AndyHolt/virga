.PHONY: build run test fmt list tidy

build:
	go build -o bin/virga ./main.go

run:
	go run ./main.go

test:
	go test -race -cover ./...

fmt:
	go fmt ./...

lint:
	golangci-lint run

tidy:
	go mod tidy
