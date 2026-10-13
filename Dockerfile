
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -a -installsuffix cgo \
    -ldflags="-w -s" \
    -o settlement-api ./cmd/settlement-api

FROM alpine:3.18

RUN apk add --no-cache ca-certificates curl postgresql-client

RUN addgroup -S settlement && adduser -S settlement -G settlement

WORKDIR /home/settlement

COPY --from=builder /app/settlement-api .
COPY --chown=settlement:settlement migrations ./migrations

USER settlement

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

EXPOSE 8080

CMD ["./settlement-api"]
