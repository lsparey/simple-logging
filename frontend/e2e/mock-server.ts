/**
 * Mock ConnectRPC server that implements LogService with fixture data.
 * Run by Playwright's webServer config before E2E tests start.
 *
 * Supports the gRPC-Web protocol (used by the frontend's createGrpcWebTransport)
 * via @connectrpc/connect-node, plus a manual CORS wrapper so the Vite dev
 * server (port 5173) can reach it (port 8081) without proxy configuration.
 */
import { createServer } from 'node:http';
import type { IncomingMessage, ServerResponse } from 'node:http';
import { connectNodeAdapter } from '@connectrpc/connect-node';
import type { ConnectRouter } from '@connectrpc/connect';
import { LogService } from '../src/gen/simplelog/v1/log_service_pb.js';

// ---------------------------------------------------------------------------
// Fixture data
// ---------------------------------------------------------------------------

const NAMESPACES = ['default', 'kube-system'];

const PODS: Record<string, Array<{ name: string; namespace: string; active: boolean }>> = {
  default: [
    { name: 'web-app-6d8c7f', namespace: 'default', active: true },
    { name: 'api-server-5b4c9e', namespace: 'default', active: false },
  ],
  'kube-system': [
    { name: 'coredns-7d4f8b', namespace: 'kube-system', active: true },
  ],
};

const DEPLOYMENTS: Record<string, Array<{ name: string; namespace: string; active: boolean }>> = {
  default: [
    { name: 'web-app', namespace: 'default', active: true },
    { name: 'api-server', namespace: 'default', active: false },
  ],
  'kube-system': [
    { name: 'coredns', namespace: 'kube-system', active: true },
  ],
};

/**
 * Generates 2160 log lines spanning 3 days (2024-01-13 to 2024-01-15).
 *
 * Per day × per hour:
 *   - 20 evenly-spaced "log entry N" lines at distinct timestamps.
 *   - 10 "burst N" lines all sharing the same HH:00:00 timestamp, simulating
 *     a real-world burst of concurrent writes at the same second.
 *
 * Layout (3 × 24 × 30 = 2160 lines total, indexed 0–2159):
 *   Day 1 (2024-01-13): indices   0 –  719
 *   Day 2 (2024-01-14): indices 720 – 1439
 *   Day 3 (2024-01-15): indices 1440 – 2159
 *
 * The last page loaded by the frontend (loadLastPage=true, pageSize=200)
 * covers indices 1960–2159, all from 2024-01-15. Known landmarks:
 *   - First entry on last page: index 1960 →
 *       "2024-01-15T17:30:30Z INFO log entry 1961 from <source>"
 *   - Last burst block (hour 23, indices 2150–2159):
 *       "2024-01-15T23:00:00Z INFO burst 1..10 from <source>"
 *   - Very first line (index 0, never on last page):
 *       "2024-01-13T00:00:00Z INFO log entry 1 from <source>"
 */
function generateLogLines(source: string): string[] {
  const lines: string[] = [];
  const days = ['2024-01-13', '2024-01-14', '2024-01-15'];

  for (const day of days) {
    for (let h = 0; h < 24; h++) {
      const hh = h.toString().padStart(2, '0');

      // 20 normal entries spread across the hour
      for (let i = 0; i < 20; i++) {
        const mm = Math.floor((i / 20) * 60).toString().padStart(2, '0');
        const ss = ((i * 3) % 60).toString().padStart(2, '0');
        lines.push(`${day}T${hh}:${mm}:${ss}Z INFO log entry ${lines.length + 1} from ${source}`);
      }

      // 10 burst lines – all share the same timestamp (simulates concurrent writes)
      for (let b = 0; b < 10; b++) {
        lines.push(`${day}T${hh}:00:00Z INFO burst ${b + 1} from ${source}`);
      }
    }
  }

  return lines; // 3 × 24 × 30 = 2160 lines
}

// Eagerly build the fixture for every source so page() calls are O(1).
const LOG_LINES = new Map<string, string[]>();

function logLinesFor(source: string): string[] {
  if (!LOG_LINES.has(source)) {
    LOG_LINES.set(source, generateLogLines(source));
  }
  return LOG_LINES.get(source)!;
}

/**
 * Returns a paginated slice of allLines.
 *
 * Token convention (mirrors the real server):
 *   prevPageToken = String(startIndex of the returned slice)
 *
 * Callers use prevPageToken as pageToken on the next request to load the
 * page that ends immediately before the previously returned slice, allowing
 * backwards pagination (load-older).
 *
 * When pageToken = "X", the server returns allLines[X-200 .. X-1].
 * When loadLastPage = true, the server returns the last 200 lines.
 */
function getPage(
  allLines: string[],
  req: { loadLastPage?: boolean; pageToken?: string; pageSize?: number },
): { lines: string[]; nextPageToken: string; prevPageToken: string } {
  const n = allLines.length;
  const size = Math.min(req.pageSize ?? 200, 200);

  let endIndex: number;
  if (req.loadLastPage) {
    endIndex = n;
  } else if (req.pageToken) {
    endIndex = parseInt(req.pageToken, 10);
    if (isNaN(endIndex) || endIndex > n) endIndex = n;
  } else {
    endIndex = Math.min(size, n);
  }

  const startIndex = Math.max(0, endIndex - size);
  return {
    lines: allLines.slice(startIndex, endIndex),
    nextPageToken: '',
    prevPageToken: startIndex > 0 ? String(startIndex) : '',
  };
}

// ---------------------------------------------------------------------------
// Service implementation
// ---------------------------------------------------------------------------

function routes(router: ConnectRouter) {
  router.service(LogService, {
    listNamespaces() {
      return { namespaces: NAMESPACES };
    },

    listPods(req) {
      return { pods: PODS[req.namespace] ?? [] };
    },

    getLogs(req) {
      return getPage(logLinesFor(req.pod), {
        loadLastPage: req.loadLastPage,
        pageToken: req.pageToken,
        pageSize: req.pageSize,
      });
    },

    async *streamLogs(req) {
      for (let i = 0; i < 5; i++) {
        yield { line: `2024-01-15T10:00:0${i}Z INFO live line ${i + 1} from ${req.pod}` };
        await new Promise<void>((resolve) => setTimeout(resolve, 200));
      }
    },

    listDeployments(req) {
      return { deployments: DEPLOYMENTS[req.namespace] ?? [] };
    },

    getDeploymentLogs(req) {
      return getPage(logLinesFor(req.deployment), {
        loadLastPage: req.loadLastPage,
        pageToken: req.pageToken,
        pageSize: req.pageSize,
      });
    },

    async *streamDeploymentLogs(req) {
      for (let i = 0; i < 5; i++) {
        yield { line: `2024-01-15T10:00:0${i}Z INFO live deployment line ${i + 1} from ${req.deployment}` };
        await new Promise<void>((resolve) => setTimeout(resolve, 200));
      }
    },
  });
}

// ---------------------------------------------------------------------------
// HTTP server with CORS wrapper
// ---------------------------------------------------------------------------

const connectHandler = connectNodeAdapter({ routes });

function withCors(req: IncomingMessage, res: ServerResponse) {
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'POST, OPTIONS');
  res.setHeader(
    'Access-Control-Allow-Headers',
    [
      'Content-Type',
      'Connect-Protocol-Version',
      'Connect-Accept-Encoding',
      'Connect-Content-Encoding',
      'Accept-Encoding',
      'X-Grpc-Web',
      'X-User-Agent',
      'Grpc-Timeout',
    ].join(', '),
  );
  res.setHeader('Access-Control-Expose-Headers', 'Grpc-Status, Grpc-Message, Content-Type');

  if (req.method === 'OPTIONS') {
    res.writeHead(204);
    res.end();
    return;
  }

  connectHandler(req, res);
}

const PORT = 8081;
const server = createServer(withCors);
server.listen(PORT, () => {
  console.log(`Mock ConnectRPC server listening on http://localhost:${PORT}`);
});
