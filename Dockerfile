FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app/worker ./cmd/worker && \
    CGO_ENABLED=0 go build -o /app/mount ./cmd/mount

FROM alpine:3.20
RUN apk add --no-cache ca-certificates fuse
COPY --from=builder /app/worker /app/mount /app/
WORKDIR /app
