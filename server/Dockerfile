# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache dependency downloads separately from source compilation.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
  -o /out/simple-logging ./cmd/server

# ── Stage 2: runtime ──────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/simple-logging /simple-logging

# Default storage path; override at runtime via LOGS_ROOT env var.
ENV LOGS_ROOT=/var/pod-logs
ENV GRPC_WEB_PORT=8080

EXPOSE 8080

ENTRYPOINT ["/simple-logging"]
