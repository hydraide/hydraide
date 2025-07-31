FROM golang:1.24.2 AS builder

WORKDIR /app

COPY . .

RUN go mod tidy

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o hydraide ./app/server

FROM alpine:latest

RUN apk --no-cache add ca-certificates curl

WORKDIR /hydraide/

COPY --from=builder /app/hydraide .

CMD ["./hydraide"]

HEALTHCHECK --interval=10s --timeout=3s --start-period=3s --retries=3 \
  CMD curl --fail http://localhost:4445/health || exit 1
