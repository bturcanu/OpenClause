# Multi-stage Dockerfile for Go services
# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy dependency files
COPY go.mod go.sum* ./
RUN go mod download || true

# Copy source code
COPY . .

# Build all binaries
RUN CGO_ENABLED=0 go build -o /gateway ./cmd/gateway && \
    CGO_ENABLED=0 go build -o /approvals ./cmd/approvals && \
    CGO_ENABLED=0 go build -o /connector-slack ./cmd/connector-slack && \
    CGO_ENABLED=0 go build -o /connector-jira ./cmd/connector-jira

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app

# Build arg to select which binary to run
ARG SERVICE_NAME=gateway
ENV SERVICE_NAME=${SERVICE_NAME}

# Copy all binaries
COPY --from=builder /gateway /app/gateway
COPY --from=builder /approvals /app/approvals
COPY --from=builder /connector-slack /app/connector-slack
COPY --from=builder /connector-jira /app/connector-jira

# Expose default port (overridden per service in compose)
EXPOSE 8080

# Run the selected service
ENTRYPOINT ["/bin/sh", "-c", "exec /app/${SERVICE_NAME}"]
