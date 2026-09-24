import { createConnectTransport } from '@connectrpc/connect-web';
import { createClient } from '@connectrpc/connect';
import { LogService } from '../gen/simplelog/v1/log_service_pb.js';

declare global {
  interface Window {
    __CONFIG__?: { apiUrl?: string };
  }
}

const configuredUrl =
  window.__CONFIG__?.apiUrl ||
  (import.meta.env.VITE_API_URL as string | undefined);

// In deployed browser builds, use the same origin as the frontend by default.
// The server binary serves both the UI and the API, so an ingress or reverse
// proxy only needs to route / to it.
export const baseUrl = configuredUrl?.trim() || window.location.origin;

// The Connect protocol sends readable JSON, so requests and responses can be
// inspected in the browser's dev tools, and server-streaming works over
// HTTP/1.1 through any proxy.
const transport = createConnectTransport({ baseUrl });

export const logClient = createClient(LogService, transport);
