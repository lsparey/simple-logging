import { createGrpcWebTransport } from '@connectrpc/connect-web';
import { createClient } from '@connectrpc/connect';
import { LogService } from '../gen/simplelog/v1/log_service_pb.js';

declare global {
  interface Window {
    __CONFIG__?: { grpcWebUrl?: string };
  }
}

const configuredUrl =
  window.__CONFIG__?.grpcWebUrl ||
  (import.meta.env.VITE_GRPC_WEB_URL as string | undefined);

// In deployed browser builds, use the same origin as the frontend by default.
// This lets an ingress or reverse proxy route gRPC-Web without baking its
// externally-visible host, scheme, or port into the image.
const baseUrl = configuredUrl?.trim() || window.location.origin;

const transport = createGrpcWebTransport({ baseUrl });

export const logClient = createClient(LogService, transport);
