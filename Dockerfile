# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY backend/go.mod ./
RUN go mod download

# Copy source code
COPY backend/ .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install certificates for HTTPS and sqlite3 CLI
RUN apk add --no-cache ca-certificates sqlite

# Copy binary from builder
COPY --from=builder /server .

# Create directory for database
RUN mkdir -p /app/data

ENV PORT=8080
ENV DB_PATH=/app/data/odds.db

EXPOSE 8080

CMD ["./server"]
