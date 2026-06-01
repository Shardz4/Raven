# Build stage
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git build-base

WORKDIR /build

# Copy go.mod and go.sum
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Compile all binaries
RUN go build -o api-server main.go
RUN go build -o store-service agents/store/main.go
RUN go build -o orchestrator agents/orchestrator/main.go
RUN go build -o solver agents/solver/main.go
RUN go build -o safety agents/safety/main.go
RUN go build -o sandbox-agent agents/sandbox/main.go
RUN go build -o consensus agents/consensus/main.go
RUN go build -o pr agents/pr/main.go

# Final stage
FROM alpine:latest
RUN apk add --no-cache ca-certificates git docker-cli bash

WORKDIR /app

# Copy compiled binaries from builder
COPY --from=builder /build/api-server .
COPY --from=builder /build/store-service .
COPY --from=builder /build/orchestrator .
COPY --from=builder /build/solver .
COPY --from=builder /build/safety .
COPY --from=builder /build/sandbox-agent .
COPY --from=builder /build/consensus .
COPY --from=builder /build/pr .

EXPOSE 8080 8081
