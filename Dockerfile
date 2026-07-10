FROM node:22-alpine AS frontend-build
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# Build backend
FROM golang:1.26-alpine AS backend-build
ARG VERSION=dev
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/simple-logging ./cmd/server

# Runtime
FROM nginx:alpine

# Backend binary
COPY --from=backend-build /out/simple-logging /simple-logging

# Frontend static assets
COPY --from=frontend-build /app/dist /usr/share/nginx/html

# nginx config — reuse the frontend config but serve on port 80
# so it doesn't clash with the backend on port 8080.
COPY frontend/nginx/nginx.conf /etc/nginx/conf.d/default.conf

# Entrypoint that injects config.js and starts both processes
COPY docker-entrypoint.sh /docker-entrypoint.sh

RUN sed -i 's/listen 8080/listen 80/' /etc/nginx/conf.d/default.conf \
    && chmod +x /docker-entrypoint.sh

ENV LOGS_ROOT=/var/pod-logs
ENV GRPC_WEB_PORT=8080

# 80  → nginx (frontend SPA)
# 8080 → Go gRPC-Web backend
EXPOSE 80 8080

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
