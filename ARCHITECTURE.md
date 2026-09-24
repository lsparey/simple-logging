# Architecture

This describes simple-logging as of v1.0: what the pieces are, how logs get from a container to the UI, and the on-disk formats that later versions must keep reading. For installing and configuring it, see the [README](README.md). For working on it, see [CONTRIBUTING.md](CONTRIBUTING.md).

## Overview

simple-logging is one Go binary, run as a single-replica Deployment. It:

1. watches every pod in the cluster,
2. collects each container's logs, either by tailing the node's log files or by streaming them from the Kubernetes API,
3. writes them to a PersistentVolumeClaim, one file per container per UTC day,
4. deletes them once they're older than the retention window, and
5. serves a React UI and an API over the stored logs, all on one port.

There is no database, no queue and no agent on each node. The files on the PVC are the only state.

```
                  ┌───────────────────── simple-logging pod ─────────────────────┐
 kube-apiserver ──┤ pod watcher (informer) ──► collector ──► segment writers ─┐  │
  (pod events,    │                              ▲   ▲                        │  │
   remote logs) ──┼──────────── API log streams ─┘   │                        ▼  │
                  │                                  │                  PVC: <ns>/<pod>/<container>/<date>.log
 /var/log/pods ───┼──── file tail (local node) ──────┘                        │  │
  (hostPath)      │                                                           │  │
                  │ retention · disk guard · indexes ◄────────────────────────┤  │
                  │                                                           ▼  │
 browser / curl ──┤ :8080  UI · Connect/gRPC/gRPC-Web API · /download · /metrics │
                  └──────────────────────────────────────────────────────────────┘
```

## Server

The server lives in `server/`. `cmd/server/main.go` wires the pieces together in this order: configuration, self-metrics, the HTTP server (listening straight away so `/healthz` answers during a long startup), the storage permission check, the legacy layout migration, the pod watcher, the collector, retention, the disk guard, and finally the API service, at which point `/readyz` turns green.

| Package | Responsibility |
|---|---|
| `internal/config` | Reads every setting from environment variables, with defaults. |
| `internal/k8s` | In-cluster clientset and the pod informer. |
| `internal/collector` | One goroutine per container, reading logs from a node file or the Kubernetes API. |
| `internal/storage` | The on-disk layout: segment writer, pod metadata, retention, disk guard, legacy migration. |
| `internal/indexes` | Optional JSON key indexes over the stored logs. |
| `internal/api` | The `LogService` API, `/download`, the SPA handler and the HTTP server. |
| `internal/metrics` | Self-metrics on a private Prometheus registry. |
| `internal/auth` | Optional built-in HTTP basic auth. |
| `internal/ui` | The built frontend, embedded into the binary with `embed.FS`. |

### Watching pods

`k8s.PodWatcher` runs one shared informer over pods in all namespaces. It strips `managedFields` from each pod before caching it, since they're usually the largest part of a pod object and the collector never reads them. A pod is handed to the collector when it is first seen in any phase except `Pending`, or when it leaves `Pending` (for any phase, since a quick Job's pod can go straight to `Succeeded`). Deletions are passed on too.

### Collecting logs

For each pod, the collector starts one stream per container, including sidecars and native sidecars (init containers with `restartPolicy: Always`), but not ordinary init containers. Each stream decides where to read from:

- **File tail** is used in `fileTail` mode, and in `hybrid` mode for pods on the collector's own node (`NODE_NAME`). It reads `/var/log/pods/<namespace>_<pod>_<uid>/<container>/<restartCount>.log` from a read-only hostPath mount, and uses inotify with a polling fallback to wake when the file grows. For static pods (such as kubeadm's control plane) the directory uses the static pod's UID, which the kubelet records on the mirror pod as the `kubernetes.io/config.mirror` annotation. Lines are parsed as CRI (`<RFC3339Nano> <stream> <flag> <content>`) or as Docker JSON. When the container restarts, the stream moves on to the next `<restartCount>.log`.
- **API stream** is used in `api` mode, and in `hybrid` mode for pods on other nodes. It follows `GET /api/v1/namespaces/<ns>/pods/<pod>/log` with timestamps on. If the stream drops it reconnects with exponential backoff (1s, doubling up to 30s), resuming from the last line's timestamp. It stops once the pod has finished or been replaced.

A line's timestamp is always the one from the source, not the time it was received. Before its first line, each stream checks whether that container already has stored logs, which is what decides whether to skip history already on disk and whether to write a `--- pod restarted at … ---` separator.

The collector also:

- **resolves the owning workload** from `ownerReferences`: a ReplicaSet with a `pod-template-hash` label is a Deployment; StatefulSet, DaemonSet and Job are used as named; a Job named `<cronjob>-<digits>` is also listed under that CronJob; and a pod with no owner is its own `Pod` workload. The result is stored in the pod's `meta.json`, so grouping survives the pod and the collector being deleted.
- **detects JSON logging** by sampling a container's first lines, for the UI's JSON formatting.

### Storage layout

Everything lives under `LOGS_ROOT` (the PVC mount, `/var/pod-logs` by default):

```
<LOGS_ROOT>/
  <namespace>/
    <pod>/
      meta.json              # namespace, pod, containers, first/last seen, owner kind/name, cronJob
      <container>/
        2026-09-20.log       # one segment per UTC day
        2026-09-21.log
  .indexes/                  # only when JSON key indexes exist; see below
```

Each line in a segment is:

```
<RFC3339Nano timestamp> [<namespace>/<pod>/<container>] <original log line>
```

Every stored line has this prefix, restart separators included (the collector formats them all with one function, `storedLine`). A line goes into the segment for its own timestamp's UTC date, so a line arriving late for yesterday is written to yesterday's file and expires with it. `SegmentWriter` caps a stored line at 1 MiB (`storage.MaxLineBytes`), ending a longer one with `…[truncated]`; every segment reader uses `storage.NewLineScanner`, which is sized for that cap.

**Write failures don't stop collection.** If a write fails (the volume is full or read-only, say), the line is dropped and counted, and that writer backs off from 1s up to 30s before trying again. Meanwhile the stream keeps reading from its source. The count shows up as `simplelog_lines_dropped_total` and on the storage dashboard.

### Retention and the disk guard

`storage.RetentionManager` deletes any segment whose date is strictly older than today (UTC) minus `RETENTION_DAYS`. It then removes any container, pod and namespace directories left empty (keeping `meta.json` for a pod that's still running, so it keeps its workload grouping), and compacts the indexes. It sweeps at startup, every `RETENTION_CHECK_INTERVAL`, and at 00:05 UTC each day, so no line is kept for more than `RETENTION_DAYS` + 1 day. The expiry decision uses only the date in the file name, never file modification times.

`storage.DiskGuard` is a safety net that checks volume usage every minute. At or above `DISK_HIGH_WATER_PERCENT` (90) it deletes the oldest segments in the whole store, across all namespaces and pods, until usage drops below `DISK_LOW_WATER_PERCENT` (80). It never deletes a segment a writer still has open, since that frees nothing until the file is closed. Each deletion is logged as a warning and counted, and indexes are compacted afterwards.

### Indexes

An index is created from the UI for one top-level JSON key, such as `companyUuid`. For every stored line whose payload is a JSON object with that key set to a string, number or bool, the index records where the line is. It doesn't store a second copy of the line. Creating an index backfills it from the existing segments, and after that the collector updates it as lines are written.

```
.indexes/
  indexes.json                     # {"keys": [...], "formatVersion": 3}
  keys/<base64url(key)>/shards/<00–ff>.idx
```

A value's shard is the first byte of its SHA-256. A shard file starts with the magic bytes `SLI3`, followed by length-prefixed binary records. Each record holds a flag for whether the timestamp is valid, the timestamp, the offset and length within the segment, and the namespace, pod, container, segment date and value. Compaction drops records whose segment no longer exists. If `formatVersion` doesn't match, the value files are rebuilt from the segments in the background at startup; index queries wait until that finishes, and nothing else does.

### API

The API is `simplelog.v1.LogService`, defined in [`proto/simplelog/v1/log_service.proto`](proto/simplelog/v1/log_service.proto). It's served with [connect-go](https://connectrpc.com), so the one endpoint speaks gRPC (including plaintext HTTP/2), gRPC-Web and Connect, whose unary calls are plain JSON over HTTP.

| Group | RPCs |
|---|---|
| Browse | `ListNamespaces`, `ListPods`, `ListWorkloads` |
| Read | `GetLogs` (one pod), `GetWorkloadLogs` (every pod of a workload, merged by timestamp) |
| Tail | `StreamLogs`, `StreamWorkloadLogs` (with an empty kind and name it tails a whole namespace). A tail follows every container's latest segment and delivers lines as they're written. |
| Search | `SearchLogs`, streamed |
| Indexes | `ListIndexes`, `CreateIndex`, `DeleteIndex`, `ListIndexValues`, `GetIndexLogs` |
| Status | `ListLogFiles` (storage summary and disk usage), `GetStats` (self-metrics) |

**Reading pages.** `GetLogs` and `GetWorkloadLogs` return pages in time order, with opaque page tokens for moving forwards and backwards. A workload's pages, and a multi-container pod's, come from a heap merge across every container's segments, holding only one page's worth of lines at a time. `GetLogs` on a single-container pod pages by byte offset instead, which avoids rescanning. Lines with equal timestamps keep their order within the file.

**Search.** `SearchLogs` works out which segments could hold matches by date, from the requested time range, and scans them concurrently with one worker per CPU. Matching runs against the log message, not the stored prefix, and is either a case-insensitive substring or an RE2 regex. Results stream back as they're found, up to `max_results`, and the final message says whether the results were cut off. Cancelling the request stops the workers.

**Other endpoints**, on the same port:

| Path | Purpose |
|---|---|
| `/download` | Stored logs for a pod or workload as a plain-text file. |
| `/healthz` | Liveness. |
| `/readyz` | Readiness: 503 until startup, including any migration, has finished. |
| `/metrics` | Prometheus metrics; only when `METRICS_ENABLED=true`. |
| `/config.js` | Runtime settings for the frontend. |
| `/` | The UI. Unknown paths fall back to `index.html`, so client-side routes work. |

**Middleware.** CORS is off unless `CORS_ALLOWED_ORIGINS` is set. Basic auth, when enabled, wraps everything except `/healthz`, `/readyz` and `/metrics`. CORS wraps auth, so preflight requests (which never carry credentials) are answered first.

### Migration from v0.11

v0.11 stored one file per pod, `<ns>/<pod>.log`. At startup, before anything reads the store, `storage.MigrateLegacyLayout` converts each of those files into the day-segment layout above. It takes each line's timestamp to pick its segment and the container name from the line's tag, writes `meta.json`, and deletes the legacy file only after every new segment has been fsynced. The migration is idempotent, and `/readyz` stays failing until it finishes. Setting `MIGRATE_LEGACY=false` leaves legacy files where they are; retention then deletes them by modification time. This code stays until v2.

## Frontend

The frontend lives in `frontend/`: React, TypeScript, Vite and MUI. The API client is generated from the proto with `protoc-gen-es` into `src/gen/`, and talks to the server with Connect-ES over the Connect protocol. That keeps payloads readable in the browser's dev tools, and server streaming works over HTTP/1.1. By default the client uses the page's own origin; `window.__CONFIG__.apiUrl` from `/config.js` can override it.

| Path | View |
|---|---|
| `/` | Redirects to `/ns/default`. |
| `/ns/<ns>` · `/ns/<ns>/<kind>` · `/ns/<ns>/<kind>/<name>` | The sidebar tree of namespace, workload kind and workload, and the merged log view. |
| `/indexes` · `/index/<key>` | Index keys, a key's values, and the lines matching one value. |
| `/search` | Server-side search. |
| `/dashboard` | Storage and collection dashboard. |

Global state is a single zustand store (`src/store/logStore.ts`) holding the current selection, the loaded lines, paging tokens and filters. Data fetching lives in hooks under `src/hooks/`. Filtering by search text within the loaded page happens in the browser; server-side search is its own page. A namespace page (`/ns/<ns>`) can also start a live tail of every pod in the namespace.

## Deployment

The chart in `deploy/helm/simple-logging` installs one Deployment with `replicas: 1` and the `Recreate` strategy, since the PVC is ReadWriteOnce. It also creates:

- a ClusterRole to list and watch pods and read their logs,
- the PVC,
- a Service, and
- optionally an Ingress, a NetworkPolicy, and a ServiceMonitor or PodMonitor.

In `hybrid` and `fileTail` modes it mounts `/var/log/pods` read-only, plus the Docker container directory when Docker is the runtime, and adds supplemental group 0 so the non-root user can read those files.

The image is the binary on `gcr.io/distroless/static:nonroot`, running as UID/GID 65532 with a read-only root filesystem, no capabilities and the `RuntimeDefault` seccomp profile.

## Compatibility promises

From v1.0 these are stable, and a change that breaks one bumps the major version and ships with a migration:

- **The on-disk format:** the storage layout, the line format, `meta.json`, and the index manifest and shard format (version 3).
- **The API:** `simplelog.v1.LogService`. New RPCs and fields may be added in minor versions. Nothing is removed or renumbered without a major version.
- **The chart's values schema:** keys may be added in minor versions. Removing or renaming one needs a major version, and the chart fails with an explanation if a removed key is still set.
- **Environment variables:** the configuration names in `internal/config`.
