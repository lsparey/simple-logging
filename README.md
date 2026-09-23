<p align="center">
  <img src="frontend/public/logo.svg" alt="simple-logging logo" width="180" />
</p>

# simple-logging

Simple, lightweight log aggregation for Kubernetes. simple-logging automatically collects logs from every pod across all namespaces, persists them to disk, and surfaces them in a clean web UI — no external dependencies, no complex configuration.

## Features

- **Live log streaming** — real-time log tailing from all pods across all namespaces via a gRPC-Web API
- **Every container** — every non-init container in a pod is collected independently, including sidecars
- **Persisted log storage** — logs are written to a PersistentVolumeClaim, one segment per container per day, and deleted once they're older than 30 days (see [retention](#retention) below)
- **Automatic pod discovery** — new pods are detected and streamed as soon as they start
- **Multi-node from a single replica** — pods on the local node are tailed straight from disk; pods on other nodes are streamed via the Kubernetes API, so no DaemonSet is needed
- **Single helm install** — deploy the full stack with one `helm install` command
- **Very low resource requirements** — requests only 150m CPU / 160Mi memory

## Installation

### Prerequisites

- Kubernetes cluster (1.24+)
- Helm 3
- A default StorageClass (or specify one explicitly)
- An Ingress controller (e.g. Traefik, nginx) if you want the UI exposed externally

### 1. Add the Helm repository

```bash
helm repo add simple-logging https://lsparey.github.io/simple-logging
helm repo update
```

### 2. Install

```bash
helm install simple-logging simple-logging/simple-logging \
  --namespace simple-logging \
  --create-namespace \
  --set ingress.enabled=true \
  --set ingress.host=logs.example.com \
  --set ingress.className=traefik
```

Replace `logs.example.com` with your desired hostname and `traefik` with your Ingress controller class.

Once the pod is running, open `http://logs.example.com` in your browser to view logs.

### Key values

| Value | Default | Description |
|---|---|---|
| `ingress.enabled` | `false` | Expose the UI via an Ingress |
| `ingress.host` | `""` | Hostname for the Ingress rule |
| `ingress.className` | `""` | Ingress controller class (e.g. `traefik`, `nginx`) |
| `config.logCollectionMode` | `hybrid` | Log collection mode: `hybrid`, `fileTail` or `api` (see below) |
| `config.nodeLogsRoot` | `/var/log/pods` | Host path for CRI pod logs (hybrid and fileTail modes) |
| `config.dockerLogsRoot` | `/var/lib/docker/containers` | Host path for Docker log content (hybrid/fileTail + Docker only) |
| `config.retentionDays` | `30` | Days of log history to keep (see [Retention](#retention)) |
| `config.diskHighWaterPercent` / `config.diskLowWaterPercent` | `90` / `80` | PVC-full safety net: at/above the high mark, oldest segments are deleted until usage is back below the low mark |
| `persistence.size` | `20Gi` | PVC size for log storage |
| `persistence.storageClass` | `""` | StorageClass name (empty = cluster default) |
| `persistence.existingClaim` | `""` | Existing PVC to mount instead of creating one |
| `persistence.claimSuffix` | `logs-v3` | Suffix for the chart-created PVC |
| `config.restDebug` | `false` | Enable plain JSON REST endpoints at `/debug/*` for testing |
| `nodeSelector` | `{}` | Pin the pod to specific nodes |
| `affinity` | `{}` | Node/pod affinity rules |
| `tolerations` | `[]` | Tolerations for tainted nodes |
| `priorityClassName` | `""` | Pod priority class |
| `podAnnotations` | `{}` | Extra annotations on the pod template |
| `extraVolumes` / `extraVolumeMounts` | `[]` | Extra volumes and mounts, e.g. for a custom cert |

### Full example with custom values

```bash
helm install simple-logging simple-logging/simple-logging \
  --namespace simple-logging \
  --create-namespace \
  --set ingress.enabled=true \
  --set ingress.host=logs.example.com \
  --set ingress.className=nginx \
  --set persistence.size=50Gi \
  --set config.retentionDays=60
```

## Log collection modes

simple-logging supports three ways to collect pod logs, controlled by `config.logCollectionMode` in the Helm values. All three run as a single Deployment replica — there is never more than one copy of the service.

### `hybrid` (default)

The collector mounts the node's CRI log directory (`/var/log/pods`) as a `hostPath` volume and learns which node it is scheduled on via the Downward API. Pods on that node are tailed directly from the filesystem using `inotify`; pods on every other node are streamed through the Kubernetes log API. Remote streams are reopened automatically with backoff if the connection drops or the container restarts, resuming from the last line received.

**Recommended for:** multi-node clusters. Only pods on remote nodes cost a kube-apiserver/kubelet connection, so scheduling simple-logging on your busiest node keeps API load to a minimum. Pin it there with `nodeSelector` or `affinity` (see [Key values](#key-values)) — otherwise the scheduler may place it anywhere. On a single-node cluster this is identical to `fileTail`.

Uses the same `config.nodeLogsRoot` / `config.dockerLogsRoot` values as `fileTail`.

### `fileTail`

The collector mounts the node's CRI log directory (`/var/log/pods`) as a `hostPath` volume and tails log files directly on the node filesystem using filesystem events (`inotify`). No persistent HTTP connections are opened to kube-apiserver, kubelet, or containerd.

**Recommended for:** single-node clusters, k3s, Docker Desktop, or any setup where the simple-logging pod always runs on the same node as the pods it monitors.

**Not suitable for:** multi-node clusters — pods scheduled on other nodes are not collected. Use `hybrid` instead.

To use this mode you must also set:

| Value | Default | Description |
|---|---|---|
| `config.nodeLogsRoot` | `/var/log/pods` | Host path where the runtime writes pod log symlinks |
| `config.dockerLogsRoot` | `/var/lib/docker/containers` | Only needed when the runtime is Docker; leave empty for containerd |

```bash
helm install simple-logging simple-logging/simple-logging \
  --namespace simple-logging \
  --create-namespace \
  --set config.logCollectionMode=fileTail
```

### `api`

The collector opens one persistent HTTP streaming connection per container via the Kubernetes log API (`client-go` `GetLogs` with `follow=true`). A shared Informer watches for pod add/delete events so new pods are picked up automatically, and dropped streams are reopened with backoff.

**Recommended for:** clusters where `hostPath` volumes are not permitted by security policy, so neither `hybrid` nor `fileTail` can mount the node's log directory.

**Trade-off:** on busy clusters with many pods this can cause elevated CPU usage in kubelet and containerd due to the number of open log-streaming connections. `hybrid` avoids this for every pod on the local node.

```bash
helm install simple-logging simple-logging/simple-logging \
  --namespace simple-logging \
  --create-namespace \
  --set config.logCollectionMode=api
```

## Retention

`config.retentionDays` (default 30) controls how long log lines are kept. Logs are stored as one file per container per UTC day; retention deletes any day's file once it is strictly older than `retentionDays`, independent of whether the pod is still logging. Worst-case overshoot is under 24 hours (a day's file isn't deleted until the day itself has fully expired), which is the normal reading of "retain for `retentionDays`".

Upgrading from a v0.11 install migrates existing `<namespace>/<pod>.log` files into this layout automatically on first startup (see [Upgrading](#upgrading)); set `MIGRATE_LEGACY=false` to opt out and leave legacy files in place, in which case they're swept by their file modification time instead (matching the old, less precise behaviour) rather than participating in the day-based cutoff.

As a safety net for when retention alone doesn't keep up (e.g. a burst of unusually verbose logging), a background check deletes the globally oldest segments — across every namespace and pod — whenever PVC usage reaches `config.diskHighWaterPercent` (default 90), continuing until usage is back below `config.diskLowWaterPercent` (default 80). This should rarely trigger; `retentionDays` is the primary control.

## Upgrading

```bash
helm repo update
helm upgrade simple-logging simple-logging/simple-logging --namespace simple-logging
```

Upgrading from a v0.11 install (or older) triggers a one-time, automatic migration of existing logs to the current on-disk layout the first time the new version starts. The pod's readiness probe stays failing until migration completes, so `kubectl rollout status` will simply take longer than usual on a large PVC rather than reporting a healthy pod prematurely; existing history is preserved. The PVC itself is reused — no `persistence.claimSuffix` change is needed.

## Image tags

Two independent tag series are published to [`lsparey/simple-logging`](https://hub.docker.com/r/lsparey/simple-logging):

- **Releases** (`X.Y.Z`, `X.Y`) — built from a tagged release commit and matched by the Helm chart's `appVersion`. This is what `helm install`/`helm upgrade` use by default; prefer these for anything other than local testing.
- **`latest` and `sha-<commit>`** — built from every push to `main`, ahead of the next tagged release. Useful for trying out unreleased fixes, not recommended for production.

## Uninstalling

```bash
helm uninstall simple-logging --namespace simple-logging
```

> **Note:** Uninstalling does not delete the PVC. To remove persisted logs, delete the PVC manually: `kubectl delete pvc -n simple-logging -l app.kubernetes.io/instance=simple-logging`

## License

[MIT](LICENSE)
