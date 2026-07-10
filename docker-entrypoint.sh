#!/bin/sh
set -e

# Write runtime configuration for the frontend.
# Leave GRPC_WEB_URL blank to use the frontend's current browser origin.
# Set it only when the gRPC-Web backend is exposed at a different origin.
cat > /usr/share/nginx/html/config.js <<EOF
window.__CONFIG__ = {
  grpcWebUrl: "${GRPC_WEB_URL:-}"
};
EOF

# Start the backend gRPC-Web server in the background.
/simple-logging &

# Start nginx in the foreground (keeps the container alive).
exec "$@"
