IMG ?= shaowenchen/memoryfs:latest

.PHONY: proto build test tidy docker-build deploy-scripts help

help:
	@echo "Targets: proto build test tidy docker-build deploy-scripts"
	@echo "  deploy-up     - start 3-node docker cluster"
	@echo "  deploy-status - show cluster status"

proto:
	protoc --go_out=. --go_opt=module=github.com/shaowenchen/memoryfs \
		--go-grpc_out=. --go-grpc_opt=module=github.com/shaowenchen/memoryfs \
		api/proto/memoryfs/v1/memoryfs.proto

build:
	GO111MODULE=on CGO_ENABLED=0 GOOS=$${TARGETOS:-linux} GOARCH=$${TARGETARCH:-amd64} \
		go build -o bin/node ./cmd/node
	GO111MODULE=on CGO_ENABLED=0 GOOS=$${TARGETOS:-linux} GOARCH=$${TARGETARCH:-amd64} \
		go build -o bin/mount ./cmd/mount

test:
	go test ./...

tidy:
	go mod tidy

docker-build:
	docker build -t $(IMG) .

deploy-scripts:
	chmod +x deploy/scripts/*.sh

deploy-up: deploy-scripts
	docker compose -f deploy/docker-compose.cluster.yml up -d

deploy-down:
	docker compose -f deploy/docker-compose.cluster.yml down

deploy-status: deploy-scripts
	./deploy/scripts/cluster-status.sh http://127.0.0.1:8080

helm-install:
	helm upgrade --install memoryfs ./deploy/helm/memoryfs \
		--namespace memoryfs --create-namespace

helm-template:
	helm template memoryfs ./deploy/helm/memoryfs

# Local dev shortcuts
node:
	go run ./cmd/node -standalone -id n1 -http :8080 -grpc :9090 -data ./data

mount:
	go run ./cmd/mount -mount /tmp/memoryfs -nodes http://127.0.0.1:8080 -f
