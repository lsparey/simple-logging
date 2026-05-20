#!/bin/sh
set -e

# Write runtime configuration from environment variables.
# GRPC_WEB_URL should be the in-cluster URL of the simple-logging gRPC-Web service.
cat > /usr/share/nginx/html/config.js <<EOF
window.__CONFIG__ = {
  grpcWebUrl: "${GRPC_WEB_URL:-http://simple-logging:8080}"
};
EOF

exec "$@"
