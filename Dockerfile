# =============================================================================
# Multi-stage Dockerfile
# Stage 1 (dev)  – live-reload with Air
# Stage 2 (prod) – minimal scratch binary
# =============================================================================

# ---------- dev stage ---------------------------------------------------------
FROM golang:1.24-alpine AS dev

RUN apk add --no-cache git curl && \
    go install github.com/air-verse/air@v1.61.7

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

CMD ["air", "-c", ".air.toml"]

# ---------- builder stage -----------------------------------------------------
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /dubai-api ./cmd/server

# ---------- production stage --------------------------------------------------
FROM gcr.io/distroless/static:nonroot AS prod

COPY --from=builder /dubai-api /dubai-api
EXPOSE 8080
ENTRYPOINT ["/dubai-api"]
