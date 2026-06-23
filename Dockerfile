# Build the memoryfs binaries (Linux only)
FROM golang:1.26-bookworm AS builder
ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /workspace
COPY . .

RUN apt-get update && apt-get install -y protobuf-compiler && \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1 && \
    make proto

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} make build

FROM ubuntu:22.04

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      fuse3 ca-certificates curl && \
    rm -rf /var/lib/apt/lists/* /var/cache/apt/* /tmp/*

WORKDIR /app
COPY --from=builder /workspace/bin/memoryfs /app/memoryfs
RUN chmod +x /app/memoryfs && \
    ln -s /app/memoryfs /usr/local/bin/memoryfs

ENTRYPOINT ["/app/memoryfs"]
CMD ["node", "-standalone", "-id", "n1", "-http", ":19800", "-data", "/data"]
