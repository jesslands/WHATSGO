# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev sqlite-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o whatsgo .

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates sqlite-libs

# Create app directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/whatsgo .

# Copy public files
COPY --from=builder /app/public ./public

# Create sessions directory
RUN mkdir -p /app/sessions

# Expose port
EXPOSE 12021

# Run the application
CMD ["./whatsgo"]
