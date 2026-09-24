ARG VERSION=dev

FROM node:22-alpine AS frontend-build
ARG VERSION
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ .
RUN npm pkg set version="${VERSION#v}" && npm run build

# Build the server with the frontend embedded (see server/internal/ui).
FROM golang:1.26-alpine AS backend-build
ARG VERSION
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ .
COPY --from=frontend-build /app/dist ./internal/ui/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/simple-logging ./cmd/server

# Runtime: a single static binary, no shell, running as UID/GID 65532.
FROM gcr.io/distroless/static:nonroot

COPY --from=backend-build /out/simple-logging /simple-logging

ENV LOGS_ROOT=/var/pod-logs
ENV PORT=8080

# UI, API (gRPC, gRPC-Web and Connect), /download, /healthz and /readyz.
EXPOSE 8080

USER 65532:65532
ENTRYPOINT ["/simple-logging"]
