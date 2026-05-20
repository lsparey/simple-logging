import { createGrpcWebTransport } from '@connectrpc/connect-web';
import { createClient } from '@connectrpc/connect';
import { LogService } from '../gen/simplelog/v1/log_service_pb.js';

declare global {
  interface Window {
    __CONFIG__?: { grpcWebUrl?: string };
  }
}

const baseUrl =
  window.__CONFIG__?.grpcWebUrl ??
  (import.meta.env.VITE_GRPC_WEB_URL as string | undefined) ??
  'http://localhost:8080';

const transport = createGrpcWebTransport({ baseUrl });

export const logClient = createClient(LogService, transport);
