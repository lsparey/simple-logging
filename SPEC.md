# simple-logging — Technical Specification

## 1. Overview

`simple-logging` is a Go application that runs inside a Kubernetes cluster, continuously streams logs from every pod across all namespaces, persists them to local files on a PersistentVolumeClaim (PVC), and exposes those logs to a frontend web application over a gRPC-Web API.

---

## 2. Goals

| # | Goal |
|---|------|
| G1 | Collect live log streams from all pods in all namespaces automatically. |
| G2 | Persist logs to disk, one file per pod, on a PVC-backed volume. |
| G3 | Automatically discover new pods as they start and begin streaming their logs. |
| G4 | Retain log files for 30 days after their last write, then delete them. |
| G5 | Expose a gRPC-Web API for frontend consumption (no HTTP/REST gateway). |
| G6 | Require no authentication on the API. |

## 3. Non-Goals

- Log shipping to external systems (e.g. Elasticsearch, Loki, Datadog).
- Collection of init container logs.
- Collection of logs from non-default containers in multi-container pods.
- Log compression or archival beyond the 30-day retention window.
- Authentication or authorisation on the API.
- Horizontal scaling (single Deployment replica assumed).

---

## 4. Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                        │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    simple-logging Pod                     │   │
│  │                                                           │   │
│  │  ┌─────────────┐   streams logs    ┌──────────────────┐  │   │
│  │  │  Log        │ ◄──────────────── │  Kubernetes API  │  │   │
│  │  │  Collector  │                   │  (in-cluster)    │  │   │
│  │  └──────┬──────┘                   └──────────────────┘  │   │
│  │         │ writes                                          │   │
│  │  ┌──────▼──────────────────────────┐                     │   │
│  │  │  PersistentVolumeClaim          │                     │   │
│  │  │  /logs/<namespace>/<pod>.log    │                     │   │
│  │  └──────┬──────────────────────────┘                     │   │
│  │         │ reads                                          │   │
│  │  ┌──────▼──────┐                                         │   │
│  │  │  gRPC-Web   │ ◄── frontend web app                    │   │
│  │  │  API Server │                                         │   │
│  │  └─────────────┘                                         │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### Components

| Component | Responsibility |
|-----------|---------------|
| **Log Collector** | Watches the Kubernetes API for pod events (add/delete), spawns a goroutine per pod to stream logs via `follow=true`, writes lines to the pod's log file. |
| **Pod Watcher** | Uses a Kubernetes `Informer` (or `Watch`) to detect new pods and signal the Log Collector. |
| **Retention Manager** | Periodically (e.g. daily) scans the log directory and deletes files not written to in the last 30 days. |
| **gRPC-Web API Server** | Serves the gRPC service over HTTP/1.1 (with gRPC-Web framing) so browser clients can connect directly. |

---

## 5. Kubernetes Integration

- **Access mode**: In-cluster — uses `rest.InClusterConfig()` from `client-go`.
- **RBAC**: Requires a `ClusterRole` with `get`, `list`, `watch` on `pods` and `get` on `pods/log`.
- **Pod discovery**: A shared `Informer` on `v1/Pod` across all namespaces provides add/delete events.
- **Log streaming**: `client-go` `CoreV1().Pods(namespace).GetLogs(podName, &PodLogOptions{Follow: true})` for the default container only.
- **Resumed streaming**: If the app restarts, streaming resumes from the current live position (not from the beginning of the file) to avoid duplicate entries.

---

## 6. Log Storage

### File Layout

```
<LOGS_ROOT>/
  <namespace>/
    <pod-name>.log        # active pod
    <pod-name>.log        # terminated pod (kept until retention expires)
```

`LOGS_ROOT` defaults to `/var/pod-logs` and is configurable via the `LOGS_ROOT` environment variable (mapped from the PVC mount path).

### File Format

Each line in a log file is the raw log line as returned by the Kubernetes API, with a prepended RFC3339 timestamp and source metadata:

```
<RFC3339 timestamp> [<namespace>/<pod>/<container>] <original log line>
```

Example:
```
2026-05-20T14:32:01Z [default/my-api-pod-7f9d/my-api] INFO server started on :8080
```

### Concurrency

Each pod is managed by a single goroutine. File writes are sequential per file. A `sync.Mutex` guards concurrent access if multiple goroutines could write to the same file (e.g. pod restarts causing a new goroutine before the old one exits).

---

## 7. Retention Policy

- A background goroutine runs once per day (configurable via `RETENTION_CHECK_INTERVAL`).
- Any log file whose **last modification time** is older than `RETENTION_DAYS` (default: `30`) days is deleted.
- Empty namespace directories are removed after their last file is deleted.
- `RETENTION_DAYS` is configurable via environment variable.

---

## 8. gRPC-Web API

### Protocol

The server uses the `improbable-eng/grpc-web` wrapper (or equivalent) to expose a gRPC service over HTTP/1.1 with gRPC-Web framing, allowing browser clients to connect without a proxy.

**Default port**: `8080` (configurable via `GRPC_WEB_PORT`).

### Protobuf Service Definition

```protobuf
syntax = "proto3";

package simplelog.v1;

service LogService {
  // List all namespaces for which logs are available.
  rpc ListNamespaces(ListNamespacesRequest) returns (ListNamespacesResponse);

  // List all pods within a namespace for which logs are available.
  rpc ListPods(ListPodsRequest) returns (ListPodsResponse);

  // Fetch paginated log lines for a specific pod, with optional time range filtering.
  rpc GetLogs(GetLogsRequest) returns (GetLogsResponse);
}

message ListNamespacesRequest {}

message ListNamespacesResponse {
  repeated string namespaces = 1;
}

message ListPodsRequest {
  string namespace = 1;
}

message PodInfo {
  string name      = 1;
  string namespace = 2;
  bool   active    = 3;   // true if the pod is currently running and being streamed
}

message ListPodsResponse {
  repeated PodInfo pods = 1;
}

message GetLogsRequest {
  string namespace  = 1;
  string pod        = 2;
  int64  start_time = 3;  // Unix timestamp (seconds); 0 = no lower bound
  int64  end_time   = 4;  // Unix timestamp (seconds); 0 = no upper bound
  int32  page_size  = 5;  // max lines per page; default 200
  string page_token = 6;  // opaque cursor (byte offset) for pagination
}

message GetLogsResponse {
  repeated string lines          = 1;
  string          next_page_token = 2;  // empty string = last page
}
```

### API Behaviour

| RPC | Description |
|-----|-------------|
| `ListNamespaces` | Returns the list of namespace directory names under `LOGS_ROOT`. |
| `ListPods` | Returns log files found under `LOGS_ROOT/<namespace>/`, annotating each with whether the pod is currently active. |
| `GetLogs` | Reads log lines from the file, filters by optional time range, and returns a page of results. The `page_token` is a base64-encoded byte offset into the file. |

### CORS / gRPC-Web Headers

The server sets appropriate `grpc-web` response headers and handles `OPTIONS` preflight requests to support browser clients.

---

## 9. Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `LOGS_ROOT` | `/var/pod-logs` | Root directory for log file storage (PVC mount path). |
| `GRPC_WEB_PORT` | `8080` | Port the gRPC-Web server listens on. |
| `RETENTION_DAYS` | `30` | Number of days to retain log files after last write. |
| `RETENTION_CHECK_INTERVAL` | `24h` | How often the retention manager runs. |
| `LOG_LEVEL` | `info` | Application log level (`debug`, `info`, `warn`, `error`). |

---

## 10. Deployment

### Kubernetes Resources

- **Deployment** — single replica running the `simple-logging` container.
- **ServiceAccount** — bound to a `ClusterRole` granting read access to pods and pod logs.
- **ClusterRole / ClusterRoleBinding** — grants `get`, `list`, `watch` on `pods` and `get` on `pods/log`.
- **PersistentVolumeClaim** — mounted at `LOGS_ROOT`; size to be determined based on expected log volume and retention period.
- **Service** — `ClusterIP` (or `NodePort`/`LoadBalancer` depending on frontend access pattern) exposing `GRPC_WEB_PORT`.

### RBAC Manifest (outline)

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: simple-logging
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]
```

---

## 11. Project Structure (Go)

```
simple-logging/
├── cmd/
│   └── server/
│       └── main.go              # Entry point
├── internal/
│   ├── collector/
│   │   ├── collector.go         # Log Collector — goroutine management
│   │   └── watcher.go           # Pod Watcher — Informer/Watch integration
│   ├── storage/
│   │   ├── writer.go            # File writer with mutex per pod
│   │   └── retention.go         # Retention Manager
│   └── api/
│       ├── server.go            # gRPC-Web server setup
│       └── log_service.go       # LogService implementation
├── proto/
│   └── simplelog/v1/
│       └── log_service.proto    # Protobuf definitions
├── gen/
│   └── simplelog/v1/            # Generated Go gRPC stubs
├── deploy/
│   ├── deployment.yaml
│   ├── clusterrole.yaml
│   ├── clusterrolebinding.yaml
│   ├── serviceaccount.yaml
│   ├── pvc.yaml
│   └── service.yaml
├── Dockerfile
├── go.mod
├── go.sum
└── SPEC.md
```

---

## 12. Key Dependencies

| Package | Purpose |
|---------|---------|
| `k8s.io/client-go` | Kubernetes API client (in-cluster config, pod watch, log streaming). |
| `google.golang.org/grpc` | gRPC runtime. |
| `github.com/improbable-eng/grpc-web` | gRPC-Web wrapper for browser compatibility. |
| `google.golang.org/protobuf` | Protobuf runtime for Go. |
| `go.uber.org/zap` | Structured application logging. |

---

## 13. Implementation Task List

### Phase 1 — Project Scaffolding ✅
- [x] `1.1` Initialise Go module (`go mod init`)
- [x] `1.2` Create directory structure (`cmd/`, `internal/`, `proto/`, `gen/`, `deploy/`)
- [x] `1.3` Add key dependencies to `go.mod` (`client-go`, `grpc`, `grpc-web`, `protobuf`, `zap`)
- [x] `1.4` Create `cmd/server/main.go` entry point (wires all components together, reads env config)
- [x] `1.5` Create `Dockerfile` (multi-stage: build → minimal runtime image)

### Phase 2 — Configuration ✅
- [x] `2.1` Define a `Config` struct in `internal/config/config.go`
- [x] `2.2` Implement env-var loading for all variables (`LOGS_ROOT`, `GRPC_WEB_PORT`, `RETENTION_DAYS`, `RETENTION_CHECK_INTERVAL`, `LOG_LEVEL`)
- [x] `2.3` Add validation (e.g. `LOGS_ROOT` must be writable, port must be valid)

### Phase 3 — Protobuf & gRPC Stubs ✅
- [x] `3.1` Write `proto/simplelog/v1/log_service.proto` with all messages and the `LogService` service
- [x] `3.2` Add `buf.gen.yaml` (or `Makefile` target) for `protoc` / `buf` code generation
- [x] `3.3` Generate Go stubs into `gen/simplelog/v1/`
- [x] `3.4` Commit generated files and document the `make generate` workflow

### Phase 4 — Kubernetes Client ✅
- [x] `4.1` Create `internal/k8s/client.go` — build in-cluster `rest.Config` and typed `clientset`
- [x] `4.2` Create `internal/k8s/informer.go` — set up a shared `PodInformer` across all namespaces
- [x] `4.3` Expose `AddHandler` / `DeleteHandler` callbacks for pod add/delete events

### Phase 5 — Log Collector ✅
- [x] `5.1` Create `internal/collector/collector.go` — manages a map of `podKey → cancel func` for active streams
- [x] `5.2` Implement `startStream(pod)` — opens `pods/log` with `Follow: true` for the default container, reads line-by-line
- [x] `5.3` Prefix each line with RFC3339 timestamp and `[namespace/pod/container]` metadata before writing
- [x] `5.4` Implement `stopStream(pod)` — cancels the context, closes the stream goroutine cleanly
- [x] `5.5` Handle pod restarts (same pod key, new UID) — write a separator line to the log file on restart
- [x] `5.6` Wire Pod Watcher events to `startStream` / `stopStream`

### Phase 6 — Storage / File Writer ✅
- [x] `6.1` Create `internal/storage/writer.go` — per-pod `FileWriter` with a `sync.Mutex`
- [x] `6.2` Implement directory creation (`LOGS_ROOT/<namespace>/`) on first write
- [x] `6.3` Expose a `Write(line string) error` method that appends to the pod's log file
- [x] `6.4` Implement `Close()` to flush and close the file handle gracefully on pod delete

### Phase 7 — Retention Manager ✅
- [x] `7.1` Create `internal/storage/retention.go` — background goroutine on a configurable ticker
- [x] `7.2` Walk `LOGS_ROOT`, stat each `.log` file, delete if `mtime` is older than `RETENTION_DAYS`
- [x] `7.3` Remove empty namespace directories after deleting their last file
- [x] `7.4` Log each deletion at `info` level

### Phase 8 — gRPC Service Implementation ✅
- [x] `8.1` Create `internal/api/log_service.go` — implement the `LogServiceServer` interface
- [x] `8.2` Implement `ListNamespaces` — enumerate subdirectories of `LOGS_ROOT`
- [x] `8.3` Implement `ListPods` — enumerate `.log` files in `LOGS_ROOT/<namespace>/`, cross-reference active stream map for `active` flag
- [x] `8.4` Implement `GetLogs` — read lines from a pod's log file, parse the RFC3339 prefix, apply `start_time`/`end_time` filters
- [x] `8.5` Implement cursor-based pagination — encode/decode byte offset as `page_token` (base64)
- [x] `8.6` Apply default and maximum `page_size` guard (default 200, max configurable)

### Phase 9 — gRPC-Web Server ✅
- [x] `9.1` Create `internal/api/server.go` — instantiate `grpc.Server`, register `LogService`
- [x] `9.2` Wrap with `improbable-eng/grpc-web` handler for HTTP/1.1 + gRPC-Web framing
- [x] `9.3` Set CORS headers to allow browser clients (configurable allowed origins)
- [x] `9.4` Handle `OPTIONS` preflight requests
- [x] `9.5` Start HTTP listener on `GRPC_WEB_PORT`

### Phase 10 — Kubernetes Manifests ✅
- [x] `10.1` `deploy/helm/simple-logging/templates/serviceaccount.yaml` — dedicated `ServiceAccount` (conditional on `serviceAccount.create`)
- [x] `10.2` `deploy/helm/simple-logging/templates/clusterrole.yaml` — `get`/`list`/`watch` on `pods`; `get` on `pods/log` (conditional on `rbac.create`)
- [x] `10.3` `deploy/helm/simple-logging/templates/clusterrolebinding.yaml` — bind role to service account
- [x] `10.4` `deploy/helm/simple-logging/templates/pvc.yaml` — `PersistentVolumeClaim` (conditional on `persistence.enabled`; falls back to `emptyDir`)
- [x] `10.5` `deploy/helm/simple-logging/templates/deployment.yaml` — single-replica Deployment, mounts PVC at `LOGS_ROOT`, sets all env vars, references service account
- [x] `10.6` `deploy/helm/simple-logging/templates/service.yaml` — `ClusterIP` Service exposing `GRPC_WEB_PORT`

> Manifests are packaged as a **Helm chart** at `deploy/helm/simple-logging/`.
> Install with:
> ```
> helm install simple-logging deploy/helm/simple-logging \
>   --namespace simple-logging --create-namespace \
>   --set image.repository=ghcr.io/your-org/simple-logging \
>   --set image.tag=v1.0.0
> ```

### Phase 11 — Testing ✅
- [x] `11.1` Unit tests for `Config` loading and validation
- [x] `11.2` Unit tests for `FileWriter` (write, mutex safety, close)
- [x] `11.3` Unit tests for Retention Manager (mock filesystem, verify deletion logic)
- [x] `11.4` Unit tests for `GetLogs` pagination and time-range filtering
- [x] `11.5` Integration test for Log Collector using a fake Kubernetes client (`k8s.io/client-go/kubernetes/fake`)
- [x] `11.6` Integration test for gRPC-Web API using an in-process server and a gRPC test client

---

## 14. Open Questions / Future Work

- **PVC sizing**: Exact PVC size needs to be calculated based on expected log throughput and the 30-day retention window.
- **Backpressure**: If a pod produces logs faster than the file system can write, a buffered channel strategy should be considered.
- **Pod restarts**: Handling a pod that crashes and restarts (same pod name, new container) — the file will be appended to; a separator line should be written to mark the restart boundary.
- **Large file pagination**: For very large log files, the byte-offset token approach should be validated for correctness when the underlying file is being actively written.
- **TLS**: If the frontend is hosted on HTTPS, the gRPC-Web server should support TLS (via cert-manager or a provided cert/key).
