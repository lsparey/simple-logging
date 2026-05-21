<p align="center">
  <img src="frontend/public/logo.svg" alt="simple-logging logo" width="180" />
</p>

# simple-logging

Simple, lightweight log aggregation for Kubernetes. simple-logging automatically collects logs from every pod across all namespaces, persists them to disk, and surfaces them in a clean web UI — no external dependencies, no complex configuration.

## Features

- **Live log streaming** — real-time log tailing from all pods across all namespaces via a gRPC-Web API
- **Persisted log storage** — logs are written to a PersistentVolumeClaim (one file per pod) and retained for 30 days
- **Automatic pod discovery** — new pods are detected and streamed as soon as they start
- **Simple helm install** — deploy the full stack (backend + UI) with two `helm install` commands
- **Very low resource requirements** — the backend requests only 100m CPU / 128Mi memory; the UI requests 50m CPU / 32Mi memory

## Tech stack

### Backend — Go

The log collection service is written in Go and runs as a single pod inside your cluster. It uses the official `client-go` library to connect to the Kubernetes API in-cluster and opens a streaming log connection (`follow=true`) for every running pod across all namespaces. A shared Kubernetes Informer watches for pod add/delete events, so new pods are picked up automatically without any polling. Each pod's log stream is handled by a dedicated goroutine, and lines are written directly to disk on the PVC in the format:

```
2026-05-20T14:32:01Z [default/my-api-pod/my-api] INFO server started on :8080
```

Logs are served to the frontend over a gRPC-Web API (HTTP/1.1 with gRPC-Web framing), which means the browser can connect directly without a separate REST gateway. A background retention process runs daily and removes log files that have not been written to in the last 30 days.

### Frontend — React

The UI is a React 19 single-page application built with Vite and TypeScript, served by a minimal nginx container. It connects to the backend using `@connectrpc/connect-web` — the same protobuf-first transport used by the server — so there is no REST translation layer at all. The component library is MUI (Material UI), virtual scrolling is handled by `react-window` to keep rendering fast even with large log volumes, and global state is managed with Zustand. The built static assets are tiny, and the running container requests just 50m CPU and 32Mi memory.

## Architecture

```
Kubernetes Cluster
│
├── simple-logging (backend)
│   ├── Streams logs from all pods via the Kubernetes API
│   ├── Writes logs to a PVC at /logs/<namespace>/<pod>.log
│   └── Exposes a gRPC-Web API on port 8080
│
└── simple-logging-ui (frontend)
    ├── React + Vite SPA served by nginx
    └── Connects to the backend via gRPC-Web through an Ingress
```

## Installation

### Prerequisites

- Kubernetes cluster (1.24+)
- Helm 3
- A default StorageClass (or specify one explicitly)
- An Ingress controller (e.g. Traefik, nginx) if you want the UI exposed externally

### 1. Install the backend

```bash
helm install simple-logging ./deploy/helm/simple-logging \
  --namespace simple-logging \
  --create-namespace
```

Key values you may want to override:

| Value | Default | Description |
|---|---|---|
| `config.retentionDays` | `30` | Days to keep log files after last write |
| `persistence.size` | `20Gi` | PVC size for log storage |
| `persistence.storageClass` | `""` | StorageClass name (empty = cluster default) |
| `resources.requests.memory` | `128Mi` | Memory request for the backend pod |

### 2. Install the UI

```bash
helm install simple-logging-ui ./deploy/helm/simple-logging-ui \
  --namespace simple-logging \
  --set ingress.enabled=true \
  --set ingress.host=logs.example.com \
  --set ingress.className=traefik
```

Replace `logs.example.com` with your desired hostname and `traefik` with your Ingress controller class.

Once both charts are running, open `http://logs.example.com` in your browser to view logs.

### Full example with custom values

```bash
# Backend — larger PVC, 60-day retention
helm install simple-logging ./deploy/helm/simple-logging \
  --namespace simple-logging \
  --create-namespace \
  --set persistence.size=50Gi \
  --set config.retentionDays=60

# UI — exposed on a custom hostname via nginx ingress
helm install simple-logging-ui ./deploy/helm/simple-logging-ui \
  --namespace simple-logging \
  --set ingress.enabled=true \
  --set ingress.host=logs.example.com \
  --set ingress.className=nginx
```

### Upgrading

```bash
helm upgrade simple-logging ./deploy/helm/simple-logging --namespace simple-logging
helm upgrade simple-logging-ui ./deploy/helm/simple-logging-ui --namespace simple-logging
```

### Uninstalling

```bash
helm uninstall simple-logging --namespace simple-logging
helm uninstall simple-logging-ui --namespace simple-logging
```

> **Note:** Uninstalling does not delete the PVC. To remove persisted logs, delete the PVC manually: `kubectl delete pvc -n simple-logging -l app.kubernetes.io/instance=simple-logging`
