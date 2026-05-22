#!/bin/sh
set -e

# Write runtime configuration for the frontend.
# When running the combined image locally, the browser reaches the backend
# on the same host via port 8080.  Override GRPC_WEB_URL for other environments.
cat > /usr/share/nginx/html/config.js <<EOF
window.__CONFIG__ = {
  grpcWebUrl: "${GRPC_WEB_URL:-http://localhost:8080}"
};
EOF

# Start the backend gRPC-Web server in the background.
/simple-logging &

# Start nginx in the foreground (keeps the container alive).
exec "$@"
