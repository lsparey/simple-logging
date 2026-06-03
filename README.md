<p align="center">
  <img src="frontend/public/logo.svg" alt="simple-logging logo" width="180" />
</p>

# simple-logging

Simple, lightweight log aggregation for Kubernetes. simple-logging automatically collects logs from every pod across all namespaces, persists them to disk, and surfaces them in a clean web UI — no external dependencies, no complex configuration.

## Features

- **Live log streaming** — real-time log tailing from all pods across all namespaces via a gRPC-Web API
- **Persisted log storage** — logs are written to a PersistentVolumeClaim (one file per pod) and retained for 30 days
- **Automatic pod discovery** — new pods are detected and streamed as soon as they start
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
| `config.logCollectionMode` | `fileTail` | Log collection mode: `fileTail` or `api` (see above) |
| `config.nodeLogsRoot` | `/var/log/pods` | Host path for CRI pod logs (fileTail mode only) |
| `config.dockerLogsRoot` | `/var/lib/docker/containers` | Host path for Docker log content (fileTail + Docker only) |
| `config.retentionDays` | `30` | Days to keep log files after last write |
| `persistence.size` | `20Gi` | PVC size for log storage |
| `persistence.storageClass` | `""` | StorageClass name (empty = cluster default) |

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

simple-logging supports two ways to collect pod logs, controlled by `config.logCollectionMode` in the Helm values.

### `fileTail` (default)

The collector mounts the node's CRI log directory (`/var/log/pods`) as a `hostPath` volume and tails log files directly on the node filesystem using filesystem events (`inotify`). No persistent HTTP connections are opened to kube-apiserver, kubelet, or containerd.

**Recommended for:** single-node clusters, k3s, Docker Desktop, or any setup where the simple-logging pod always runs on the same node as the pods it monitors.

**Not suitable for:** multi-node clusters — simple-logging is a single Deployment replica and cannot see the log files of pods scheduled on other nodes.

To use this mode you must also set:

| Value | Default | Description |
|---|---|---|
| `config.nodeLogsRoot` | `/var/log/pods` | Host path where the runtime writes pod log symlinks |
| `config.dockerLogsRoot` | `/var/lib/docker/containers` | Only needed when the runtime is Docker; leave empty for containerd |

### `api`

The collector opens one persistent HTTP streaming connection per pod via the Kubernetes log API (`client-go` `GetLogs` with `follow=true`). A shared Informer watches for pod add/delete events so new pods are picked up automatically.

**Recommended for:** multi-node clusters where simple-logging cannot access host filesystems of other nodes.

**Trade-off:** on busy clusters with many pods this can cause elevated CPU usage in kubelet and containerd due to the number of open log-streaming connections.

```bash
helm install simple-logging simple-logging/simple-logging \
  --namespace simple-logging \
  --create-namespace \
  --set config.logCollectionMode=api
```

## Upgrading

```bash
helm repo update
helm upgrade simple-logging simple-logging/simple-logging --namespace simple-logging
```

## Uninstalling

```bash
helm uninstall simple-logging --namespace simple-logging
```

> **Note:** Uninstalling does not delete the PVC. To remove persisted logs, delete the PVC manually: `kubectl delete pvc -n simple-logging -l app.kubernetes.io/instance=simple-logging`

