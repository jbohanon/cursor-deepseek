# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source files
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /cursor-deepseek ./cmd

# Use a minimal base image
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Set working directory
WORKDIR /app

# Copy the binary from builder
COPY --from=builder /cursor-deepseek .

# Copy any additional files needed (like .env.example)
COPY .env.example .env

EXPOSE 9000

# Run the binary
ENTRYPOINT ["./cursor-deepseek"] 
