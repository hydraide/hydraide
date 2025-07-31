FROM golang:1.24.2 AS builder

WORKDIR /app

COPY . .

RUN go mod tidy

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o hydraide ./app/server

# Final stage
FROM alpine:latest

# Install tools
RUN apk --no-cache add ca-certificates curl shadow su-exec

# Create app folder
WORKDIR /hydraide

# Copy built binary
COPY --from=builder /app/hydraide .

# Copy entrypoint
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
CMD ["./hydraide"]

HEALTHCHECK --interval=10s --timeout=3s --start-period=3s --retries=3 \
  CMD curl --fail http://localhost:4445/health || exit 1
