.PHONY: build worker mount tidy

build:
	go build -o bin/worker ./cmd/worker
	go build -o bin/mount ./cmd/mount

worker:
	go run ./cmd/worker -addr :8080 -redis 127.0.0.1:6379

mount:
	go run ./cmd/mount -mount /tmp/memoryfs -redis 127.0.0.1:6379 -f

tidy:
	go mod tidy
