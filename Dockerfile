# Multi-stage Dockerfile for Go services
# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arg to select which binary to build
ARG SERVICE_NAME=gateway

# Build only the selected service binary
RUN CGO_ENABLED=0 go build -o /service ./cmd/${SERVICE_NAME}

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates && \
    adduser -D -u 1001 appuser

WORKDIR /app

# Copy only the selected binary
COPY --from=builder /service /app/service

USER appuser

EXPOSE 8080

ENTRYPOINT ["/app/service"]
