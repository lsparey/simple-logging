ARG VERSION=dev

# Both build stages run natively on the build machine ($BUILDPLATFORM) for
# every target platform: the frontend build is architecture-independent and
# Go cross-compiles. Only the final stage is per-platform, and it only copies
# the binary, so multi-arch builds never execute anything under QEMU (where
# `npm ci` for arm64 took minutes and sometimes hung indefinitely).
FROM --platform=$BUILDPLATFORM node:25-alpine AS frontend-build
ARG VERSION
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ .
RUN npm pkg set version="${VERSION#v}" && npm run build

# Build the server with the frontend embedded (see server/internal/ui).
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend-build
ARG VERSION
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ .
COPY --from=frontend-build /app/dist ./internal/ui/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
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
