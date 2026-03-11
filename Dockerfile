# Stage 1: Build the Go binary
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy dependency files first so Docker can cache this layer
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a statically linked binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o opengrok-mcp-server ./cmd/server/

# Stage 2: Minimal runtime image
# We use alpine (not scratch) so we have a shell available for docker exec
FROM alpine:latest

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/opengrok-mcp-server /opengrok-mcp-server

# The container stays alive via 'command: sleep infinity' in docker-compose.
# Claude Code then runs the binary via: docker exec -i opengrok-mcp /opengrok-mcp-server
CMD ["/opengrok-mcp-server"]
