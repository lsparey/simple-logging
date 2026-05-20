# simple-logging UI — Frontend Technical Specification

## 1. Overview

`simple-logging-ui` is a React single-page application (SPA) that provides a browser-based log viewer for the `simple-logging` backend. It connects directly to the backend's gRPC-Web API, renders a namespace/pod tree in a sidebar, and displays log output in the main panel with live tail, historical pagination, text search, time-range filtering, and log-level colour coding.

The frontend is deployed as a separate Kubernetes container (Nginx serving the built SPA) in the same cluster as the backend.

---

## 2. Goals

| # | Goal |
|---|------|
| G1 | Browse namespaces and pods via a collapsible sidebar. |
| G2 | View historical logs with cursor-based pagination. |
| G3 | Live-tail logs in real time via a gRPC-Web server-streaming RPC. |
| G4 | Filter logs by free-text (client-side) and optional time range (server-side). |
| G5 | Colour-code log lines by detected log level (DEBUG / INFO / WARN / ERROR). |
| G6 | Full dark-mode support with user preference persisted in `localStorage`. |
| G7 | Deploy as a standalone Kubernetes `Deployment` with runtime-configurable backend URL. |

---

## 3. Non-Goals

- Simultaneous multi-pod view.
- Authentication or authorisation (matches the backend's no-auth policy).
- Log file download.
- Mobile-first / fully responsive layout.
- Log shipping or export to external systems.

---

## 4. Architecture

```
Browser
└── React SPA  (served by Nginx, port 80)
    └── @connectrpc/connect-web Transport
        └── ──── HTTP/1.1 + gRPC-Web ────►  simple-logging Service :8080
                                              ├── ListNamespaces
                                              ├── ListPods
                                              ├── GetLogs
                                              └── StreamLogs  (new — server-streaming)
```

### Kubernetes topology

```
┌──────────────────────────────────────────────────┐
│  Ingress (optional)                              │
│    /                    → simple-logging-ui :80  │
│    /simplelog.v1.*      → simple-logging     :8080│
└──────────────────────────────────────────────────┘
```

Routing all traffic through a single Ingress at the same origin eliminates CORS concerns entirely. If no Ingress is used, the backend's existing CORS headers handle cross-origin requests from the frontend's separate origin.

---

## 5. Required Backend Changes

A new **server-streaming** RPC must be added to the existing proto before frontend work begins.

### 5.1 Proto additions (`proto/simplelog/v1/log_service.proto`)

```protobuf
service LogService {
  // ... existing RPCs unchanged ...

  // StreamLogs tails a pod's log file and streams new lines as they are written.
  // The stream stays open until the client cancels it.
  rpc StreamLogs(StreamLogsRequest) returns (stream StreamLogsResponse);
}

message StreamLogsRequest {
  string namespace = 1;
  string pod       = 2;
}

message StreamLogsResponse {
  string line = 1;
}
```

### 5.2 Backend implementation (`internal/api/log_service.go`)

`StreamLogs` must:
1. Open the pod's log file at the current end-of-file offset (do not replay existing content).
2. Block in a poll loop (e.g. `time.Sleep(250ms)` between reads, or `inotify`-based watch) reading new bytes as they appear.
3. Send each complete line as a `StreamLogsResponse`.
4. Return cleanly when the client cancels the stream (check `ctx.Done()`).

---

## 6. UI Layout

```
┌──────────────────────────────────────────────────────────────────┐
│ AppBar:  simple-logging                        [☀/☾ dark toggle] │
├─────────────────┬────────────────────────────────────────────────┤
│ Sidebar (240px) │  Log Panel                                     │
│                 │                                                │
│ ▼ default       │  ┌──────────────────────────────────────────┐  │
│   • api-pod     │  │ LogToolbar                               │  │
│   ○ old-pod     │  │  [● Live]  [Start: ____] [End: ____]    │  │
│ ► kube-system   │  │  [Search: _________________________]    │  │
│ ► monitoring    │  ├──────────────────────────────────────────┤  │
│                 │  │ LogList (virtualised)                    │  │
│   • = active    │  │                                          │  │
│   ○ = inactive  │  │  14:32:01Z [default/api-pod/api] INFO…  │  │
│                 │  │  14:32:02Z [default/api-pod/api] WARN…  │  │
│                 │  │  14:32:03Z [default/api-pod/api] ERROR… │  │
│                 │  │                                          │  │
│                 │  ├──────────────────────────────────────────┤  │
│                 │  │  [← Load earlier]        [Load later →] │  │
│                 │  └──────────────────────────────────────────┘  │
└─────────────────┴────────────────────────────────────────────────┘
```

When no pod is selected the Log Panel shows an empty-state prompt.

---

## 7. Component Breakdown

### 7.1 `AppShell`
- MUI `AppBar` with app title and dark-mode toggle (`IconButton` using `Brightness4` / `Brightness7` icons).
- MUI `Drawer` (permanent, 240 px) hosting the `PodSidebar`.
- Main content area hosting the `LogPanel`.
- Reads/writes `localStorage` key `simple-logging.theme` (`"light"` | `"dark"`).

### 7.2 `PodSidebar`
- On mount: calls `ListNamespaces`; populates a list of collapsible `NamespaceNode` items.
- Expanding a `NamespaceNode`: calls `ListPods(namespace)`; renders `PodNode` children.
- **Active pods** (`PodInfo.active === true`): green dot badge.
- **Inactive pods** (`PodInfo.active === false`): grey dot badge.
- Clicking a `PodNode` sets it as the selected pod in the global store.
- Auto-refreshes the full list every **30 seconds** (uses a `setInterval`; refresh does not collapse open nodes).

### 7.3 `LogPanel`
Owns a state machine with four states:

| State | Description |
|-------|-------------|
| `idle` | No pod selected. Show empty-state prompt. |
| `loading` | Initial page load in progress. Show skeleton rows. |
| `history` | Displaying a page of historical log lines. |
| `live` | Live tail active; new lines arrive via `StreamLogs` stream. |

#### 7.3.1 `LogToolbar`
- **Live toggle** (`Switch`): transitions between `history` and `live` states.
  - Enabling live: cancels any active `GetLogs` request; opens `StreamLogs` stream.
  - Disabling live: cancels the stream; loads the most-recent page via `GetLogs`.
- **Start / End datetime inputs** (MUI `DateTimePicker`): bound to `start_time` / `end_time` on `GetLogs`; cleared when entering live mode.
- **Text search input**: free-text; applied client-side to the currently loaded lines.
- Filter changes reset the cursor and reload page 1.

#### 7.3.2 `LogList`
- Rendered with `react-window` `VariableSizeList` to handle thousands of lines without DOM thrashing.
- In **live** mode: new lines are appended and the list auto-scrolls to the bottom (unless the user has manually scrolled up, in which case auto-scroll is paused).
- In **history** mode: the visible lines are the current page; pagination controls are shown below.

#### 7.3.3 `LogLine`
Each row renders a single log line string. Level is detected by scanning the line for the first occurrence of a known keyword (case-insensitive):

| Keyword(s) | Colour token (dark) | Colour token (light) |
|---|---|---|
| `ERROR`, `FATAL`, `CRITICAL` | `error.light` (`red[400]`) | `error.dark` (`red[700]`) |
| `WARN`, `WARNING` | `warning.light` (`amber[400]`) | `warning.dark` (`amber[700]`) |
| `DEBUG`, `TRACE` | `text.disabled` | `text.disabled` |
| `INFO` / (none matched) | `text.primary` | `text.primary` |

The matched keyword within the line is rendered in bold to aid scanning.

Detection regex: `/\b(TRACE|DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL|CRITICAL)\b/i`

#### 7.3.4 Pagination controls
- **"← Load earlier"** button: fetches the previous page by passing the stored *start* cursor.
- **"Load later →"** button: fetches the next page by passing `next_page_token`.
- Buttons are hidden when in live mode.
- "Load later →" is disabled when `next_page_token` is empty (last page reached).

---

## 8. State Management

A single **Zustand** store (`src/store/logStore.ts`) holds:

```typescript
interface LogStore {
  // Pod selection
  selectedNamespace: string | null;
  selectedPod: string | null;
  setSelectedPod(namespace: string, pod: string): void;

  // Display mode
  mode: 'idle' | 'loading' | 'history' | 'live';
  setMode(mode: LogStore['mode']): void;

  // Log lines
  lines: string[];
  appendLines(lines: string[]): void;
  setLines(lines: string[]): void;
  clearLines(): void;

  // Pagination cursors
  prevPageToken: string;   // token to load earlier page
  nextPageToken: string;   // token to load later page
  setPaginationTokens(prev: string, next: string): void;

  // Filters
  searchText: string;
  setSearchText(text: string): void;
  startTime: number;  // Unix seconds, 0 = unset
  endTime: number;
  setTimeRange(start: number, end: number): void;

  // Dark mode
  darkMode: boolean;
  toggleDarkMode(): void;
}
```

Derived selector: `filteredLines` — `lines` filtered by `searchText` (case-insensitive substring match).

---

## 9. gRPC Client

### 9.1 Code generation

TypeScript stubs are generated from the existing proto file using `buf` with the Connect plugin stack:

**`buf.gen.yaml` additions:**

```yaml
plugins:
  - plugin: es
    out: ui/src/gen
    opt: target=ts
  - plugin: connect-es
    out: ui/src/gen
    opt: target=ts
```

Requires `npm` packages `@bufbuild/protoc-gen-es` and `@connectrpc/protoc-gen-connect-es` installed as dev-dependencies.

### 9.2 Transport (`src/grpc/client.ts`)

```typescript
import { createGrpcWebTransport } from '@connectrpc/connect-web';
import { createClient } from '@connectrpc/connect';
import { LogService } from '../gen/simplelog/v1/log_service_connect';

const transport = createGrpcWebTransport({
  baseUrl: window.__CONFIG__?.grpcWebUrl ?? import.meta.env.VITE_GRPC_WEB_URL,
});

export const logClient = createClient(LogService, transport);
```

The `@connectrpc/connect-web` gRPC-Web transport is fully compatible with the `improbable-eng/grpc-web` server already deployed.

### 9.3 Custom hooks

| Hook | Description |
|------|-------------|
| `useNamespaces()` | Calls `ListNamespaces` on mount; returns `{ namespaces, loading, error }`. |
| `usePodList(namespace)` | Calls `ListPods` when namespace changes; returns `{ pods, loading, error }`. |
| `useLogHistory(namespace, pod, filters, pageToken)` | Calls `GetLogs`; returns `{ lines, nextPageToken, prevPageToken, loading }`. |
| `useLogStream(namespace, pod, enabled)` | Calls `StreamLogs` server-stream when `enabled`; appends lines to store; cleans up stream on disable or unmount. |

---

## 10. Configuration

### Build-time (Vite env var)

| Variable | Default | Description |
|----------|---------|-------------|
| `VITE_GRPC_WEB_URL` | `http://localhost:8080` | gRPC-Web backend base URL (used during local dev). |

### Runtime (Kubernetes)

To allow the same Docker image to target different backend URLs without rebuilding, the Nginx container startup script substitutes environment variables into a `config.js` file served at `/config.js`:

```javascript
// /config.js (generated at container start)
window.__CONFIG__ = {
  grpcWebUrl: "http://simple-logging.simple-logging.svc.cluster.local:8080"
};
```

`index.html` includes `<script src="/config.js"></script>` before the app bundle.

The Helm chart exposes a `config.grpcWebUrl` value that is written into a `ConfigMap` and mounted into the container.

---

## 11. Project Structure

```
ui/                                      ← lives inside the simple-logging repo
├── src/
│   ├── main.tsx
│   ├── App.tsx
│   ├── theme.ts                         # MUI createTheme (light + dark)
│   ├── components/
│   │   ├── AppShell/
│   │   │   └── AppShell.tsx
│   │   ├── PodSidebar/
│   │   │   ├── PodSidebar.tsx
│   │   │   ├── NamespaceNode.tsx
│   │   │   └── PodNode.tsx
│   │   ├── LogPanel/
│   │   │   ├── LogPanel.tsx
│   │   │   ├── LogToolbar.tsx
│   │   │   ├── LogList.tsx
│   │   │   └── LogLine.tsx
│   │   └── TimeRangePicker/
│   │       └── TimeRangePicker.tsx
│   ├── hooks/
│   │   ├── useNamespaces.ts
│   │   ├── usePodList.ts
│   │   ├── useLogHistory.ts
│   │   └── useLogStream.ts
│   ├── grpc/
│   │   └── client.ts                   # Transport + exported logClient
│   ├── store/
│   │   └── logStore.ts                 # Zustand store
│   └── gen/                            # buf-generated TypeScript stubs
│       └── simplelog/v1/
│           ├── log_service_pb.ts
│           └── log_service_connect.ts
├── public/
│   ├── config.js.tmpl                  # Runtime config template (envsubst)
│   └── index.html
├── nginx/
│   └── nginx.conf                      # SPA fallback routing
├── Dockerfile
├── vite.config.ts
├── tsconfig.json
├── package.json
└── buf.gen.yaml                        # (or extend root buf.gen.yaml)
```

The Helm chart for the frontend lives at `deploy/helm/simple-logging-ui/`.

---

## 12. Key Dependencies

| Package | Purpose |
|---------|---------|
| `react` + `react-dom` | UI framework |
| `@mui/material` + `@mui/icons-material` | Component library (Material Design) |
| `@mui/x-date-pickers` | `DateTimePicker` for time range filter |
| `@emotion/react` + `@emotion/styled` | MUI styling engine |
| `@connectrpc/connect` + `@connectrpc/connect-web` | gRPC-Web client (compatible with `improbable-eng/grpc-web` server) |
| `@bufbuild/protobuf` | Protobuf runtime for TypeScript |
| `react-window` | Virtualised list for large log output |
| `zustand` | Lightweight global state management |
| `dayjs` | Date/time parsing and formatting |
| `vite` + `@vitejs/plugin-react` | Build tooling |
| **Dev** `@bufbuild/protoc-gen-es` | buf TypeScript protobuf codegen plugin |
| **Dev** `@connectrpc/protoc-gen-connect-es` | buf Connect TypeScript codegen plugin |
| **Dev** `vitest` + `@testing-library/react` | Unit testing |

---

## 13. Deployment

### 13.1 Dockerfile (`ui/Dockerfile`)

Multi-stage build:
1. **Build stage** (`node:22-alpine`): `npm ci` → `npm run build` → outputs to `dist/`.
2. **Runtime stage** (`nginx:1.27-alpine`):
   - Copies `dist/` to `/usr/share/nginx/html/`.
   - Copies `nginx/nginx.conf` to `/etc/nginx/conf.d/default.conf`.
   - Copies `public/config.js.tmpl` to `/docker-entrypoint.d/40-config.sh` (uses `envsubst` to generate `/usr/share/nginx/html/config.js` from `$GRPC_WEB_URL` at startup).

### 13.2 Nginx config (`nginx/nginx.conf`)

```nginx
server {
    listen 80;
    root /usr/share/nginx/html;
    index index.html;

    # Serve pre-built assets with long-lived cache
    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    # SPA fallback — all unmatched paths return index.html
    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

### 13.3 Helm chart (`deploy/helm/simple-logging-ui/`)

| Template | Resource |
|----------|----------|
| `deployment.yaml` | Single-replica `Deployment`; sets `GRPC_WEB_URL` env var from `config.grpcWebUrl` value. |
| `service.yaml` | `ClusterIP` Service on port 80. |
| `ingress.yaml` | Optional `Ingress` (conditional on `ingress.enabled`); routes `/` to frontend and `/simplelog.v1.` to backend service. |
| `configmap.yaml` | Stores `grpcWebUrl` for the deployment env var. |

Key `values.yaml` additions:

```yaml
image:
  repository: ghcr.io/your-org/simple-logging-ui
  tag: latest
  pullPolicy: IfNotPresent

config:
  grpcWebUrl: "http://simple-logging:8080"   # in-cluster default

ingress:
  enabled: false
  className: nginx
  host: logs.example.com
  # When enabled, rewrites /simplelog.v1.* paths to the backend service
  backendServiceName: simple-logging
  backendServicePort: 8080
```

### 13.4 Install

```bash
helm install simple-logging-ui deploy/helm/simple-logging-ui \
  --namespace simple-logging \
  --set image.repository=ghcr.io/your-org/simple-logging-ui \
  --set image.tag=v1.0.0 \
  --set config.grpcWebUrl=http://simple-logging:8080 \
  --set ingress.enabled=true \
  --set ingress.host=logs.example.com
```

---

## 14. Implementation Task List

### Phase 1 — Backend: Proto Extension
- [ ] `1.1` Add `StreamLogs` RPC, `StreamLogsRequest`, and `StreamLogsResponse` to `proto/simplelog/v1/log_service.proto`.
- [ ] `1.2` Implement `StreamLogs` in `internal/api/log_service.go`: seek to EOF, poll for new lines, send each line, respect `ctx.Done()`.
- [ ] `1.3` Regenerate Go stubs (`make generate`).
- [ ] `1.4` Add unit/integration test for `StreamLogs` (write to file, assert streamed lines received).

### Phase 2 — TypeScript Codegen
- [ ] `2.1` Add `@bufbuild/protoc-gen-es` and `@connectrpc/protoc-gen-connect-es` to `buf.gen.yaml` (or a separate `ui/buf.gen.yaml`).
- [ ] `2.2` Add a `make generate-ts` (or `npm run generate`) target that runs `buf generate` outputting to `ui/src/gen/`.
- [ ] `2.3` Commit generated TypeScript stubs.

### Phase 3 — Project Scaffolding
- [ ] `3.1` Initialise Vite + React + TypeScript project under `ui/` (`npm create vite@latest ui -- --template react-ts`).
- [ ] `3.2` Install runtime dependencies: MUI, `@mui/x-date-pickers`, Emotion, Connect, Protobuf, `react-window`, Zustand, dayjs.
- [ ] `3.3` Install dev dependencies: vitest, `@testing-library/react`, `@testing-library/user-event`.
- [ ] `3.4` Create `src/theme.ts` with MUI `createTheme` for both light and dark palettes.
- [ ] `3.5` Create `src/grpc/client.ts` with `createGrpcWebTransport` and exported `logClient`.
- [ ] `3.6` Create `src/store/logStore.ts` (Zustand store as described in §8).
- [ ] `3.7` Create `public/config.js.tmpl` and the Nginx startup snippet that runs `envsubst`.

### Phase 4 — PodSidebar
- [ ] `4.1` Implement `useNamespaces()` hook.
- [ ] `4.2` Implement `usePodList(namespace)` hook.
- [ ] `4.3` Implement `NamespaceNode` (collapsible, triggers `usePodList` on expand).
- [ ] `4.4` Implement `PodNode` (active/inactive badge, click to select).
- [ ] `4.5` Implement `PodSidebar` with 30-second auto-refresh.

### Phase 5 — Log History View
- [ ] `5.1` Implement `useLogHistory(namespace, pod, filters, pageToken)` hook.
- [ ] `5.2` Implement `LogLine` with level-detection regex and colour mapping.
- [ ] `5.3` Implement `LogList` with `react-window` `VariableSizeList`.
- [ ] `5.4` Implement pagination controls ("← Load earlier" / "Load later →").
- [ ] `5.5` Implement `TimeRangePicker` and wire `start_time` / `end_time` to the hook.

### Phase 6 — Live Tail
- [ ] `6.1` Implement `useLogStream(namespace, pod, enabled)` hook.
- [ ] `6.2` Add Live toggle to `LogToolbar`; wire to `useLogStream`.
- [ ] `6.3` Auto-scroll `LogList` to bottom when live mode is active and user has not manually scrolled.
- [ ] `6.4` Pause auto-scroll when user scrolls up; resume when user scrolls back to bottom.

### Phase 7 — Filtering & Search
- [ ] `7.1` Add search text input to `LogToolbar`; wire to `logStore.searchText`.
- [ ] `7.2` Apply `filteredLines` derived selector in `LogList` render.
- [ ] `7.3` Reset cursor and reload when search text or time range changes.

### Phase 8 — AppShell & Theme
- [ ] `8.1` Implement `AppShell` (AppBar, permanent Drawer, main content area).
- [ ] `8.2` Wire dark-mode toggle to `logStore.darkMode`; persist to `localStorage`.
- [ ] `8.3` Pass `mode` prop to MUI `ThemeProvider` based on store value.

### Phase 9 — Deployment
- [ ] `9.1` Write `ui/Dockerfile` (multi-stage node build → nginx runtime).
- [ ] `9.2` Write `ui/nginx/nginx.conf` (SPA fallback, assets caching).
- [ ] `9.3` Write Helm chart under `deploy/helm/simple-logging-ui/` (Deployment, Service, Ingress, ConfigMap templates + `values.yaml`).
- [ ] `9.4` Add frontend image build to `Makefile`.

### Phase 10 — Testing
- [ ] `10.1` Unit tests for `LogLine` level detection + colour mapping (all keyword variants).
- [ ] `10.2` Unit tests for `useLogHistory` with a mocked `logClient`.
- [ ] `10.3` Unit tests for `useLogStream` with a mocked streaming `logClient`.
- [ ] `10.4` Snapshot / interaction tests for `PodSidebar`, `LogToolbar`, `LogPanel` using `@testing-library/react`.

---

## 15. Open Questions / Future Work

- **Ingress TLS**: If the frontend is exposed externally over HTTPS, the backend gRPC-Web service also needs TLS (cert-manager or provided cert), since browsers block mixed-content requests.
- **Large line buffer**: `StreamLogs` may produce a very high line rate; consider a client-side line-count cap (e.g. keep only the last 5,000 live lines) to avoid unbounded memory growth.
- **Pod log history on live switch**: When the user switches from live back to history, decide whether to reload from the server or keep the in-memory lines.
- **Namespace auto-refresh on collapse**: Currently open namespace nodes are not re-fetched when a pod is added; a more granular refresh strategy may be needed.
- **Websocket fallback**: Some corporate proxies strip `Transfer-Encoding: chunked`, which breaks gRPC-Web streams. A WebSocket fallback transport could be considered.
