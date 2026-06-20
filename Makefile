IMG ?= shaowenchen/memoryfs:latest
VERSION ?= 0.1.3
HELM_CHART = https://github.com/shaowenchen/memoryfs/releases/download/v$(VERSION)/memoryfs-$(VERSION).tgz

.PHONY: proto build test tidy docker-build deploy-scripts help

help:
	@echo "Targets: proto build test tidy docker-build deploy-scripts helm-install helm-install-local"
	@echo "  deploy-status   - cluster status (requires port-forward or local node)"
	@echo "  status          - cluster storage status"
	@echo "  benchmark       - storage throughput test"
	@echo "  helm-install    - install from Release chart"
	@echo "  helm-install-local - install from ./deploy/helm/memoryfs"

proto:
	protoc --go_out=. --go_opt=module=github.com/shaowenchen/memoryfs \
		--go-grpc_out=. --go-grpc_opt=module=github.com/shaowenchen/memoryfs \
		api/proto/memoryfs/v1/memoryfs.proto

build:
	GO111MODULE=on CGO_ENABLED=0 GOOS=$${TARGETOS:-linux} GOARCH=$${TARGETARCH:-amd64} \
		go build -o bin/node ./cmd/node
	GO111MODULE=on CGO_ENABLED=0 GOOS=$${TARGETOS:-linux} GOARCH=$${TARGETARCH:-amd64} \
		go build -o bin/mount ./cmd/mount
	GO111MODULE=on CGO_ENABLED=0 GOOS=$${TARGETOS:-linux} GOARCH=$${TARGETARCH:-amd64} \
		go build -o bin/status ./cmd/status
	GO111MODULE=on CGO_ENABLED=0 GOOS=$${TARGETOS:-linux} GOARCH=$${TARGETARCH:-amd64} \
		go build -o bin/benchmark ./cmd/benchmark

test:
	go test ./...

tidy:
	go mod tidy

docker-build:
	docker build -t $(IMG) .

deploy-scripts:
	chmod +x deploy/scripts/*.sh

deploy-status: deploy-scripts
	./bin/status -nodes http://127.0.0.1:8080 || ./deploy/scripts/cluster-status.sh http://127.0.0.1:8080

status:
	go run ./cmd/status -nodes http://127.0.0.1:8080

benchmark:
	go run ./cmd/benchmark -nodes http://127.0.0.1:8080 -writes 20 -reads 20 -workers 4

helm-install:
	helm upgrade --install memoryfs ./deploy/helm/memoryfs \
		--namespace memoryfs --create-namespace

helm-install-local: helm-install

helm-template:
	helm template memoryfs ./deploy/helm/memoryfs

# Local dev shortcuts
node:
	go run ./cmd/node -standalone -id n1 -http :8080 -grpc :9090 -data ./data

mount:
	go run ./cmd/mount -mount /tmp/memoryfs -nodes http://127.0.0.1:8080 -f
