# ======================
# Builder
# ======================
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install dependency untuk CGO + sqlite
RUN apk add --no-cache \
    gcc \
    musl-dev \
    sqlite-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# WAJIB: CGO ENABLED
ENV CGO_ENABLED=1
ENV GOOS=linux

RUN go build -o main ./cmd/api/main.go

# ======================
# Runtime
# ======================
FROM alpine:latest

# runtime sqlite + ssl
RUN apk add --no-cache \
    ca-certificates \
    sqlite-libs

WORKDIR /app

COPY --from=builder /app/main .
COPY --from=builder /app/.env .

EXPOSE 3003

CMD ["./main"]
