<p align="center">
  <img src="frontend/public/logo.svg" alt="simple-logging logo" width="180" />
</p>

# simple-logging

Simple, lightweight log aggregation for Kubernetes. simple-logging automatically collects logs from every pod across all namespaces, persists them to disk, and surfaces them in a clean web UI — no external dependencies, no complex configuration.

## Features

- **Live log streaming** — real-time log tailing from all pods across all namespaces
- **Every container** — every non-init container in a pod is collected independently, including sidecars
- **Every workload kind** — logs are grouped by Deployment, StatefulSet, DaemonSet, Job, CronJob or bare pod, and merged across a workload's pods (see [supported workloads](#supported-workloads))
- **Search, indexes and download** — server-side substring or regex search across any time range, optional indexes on a JSON key, and plain-text download
- **Persisted log storage** — logs are written to a PersistentVolumeClaim, one segment per container per day, and deleted once they're older than 30 days (see [retention](#retention) below)
- **Automatic pod discovery** — new pods are detected and streamed as soon as they start
- **Multi-node from a single replica** — pods on the local node are tailed straight from disk; pods on other nodes are streamed via the Kubernetes API, so no DaemonSet is needed
- **Single helm install** — deploy the full stack with one `helm install` command
- **One small, non-root binary** — the UI and the API are served by one Go binary on one port, in a ~49 MB distroless image running as a non-root user with a read-only root filesystem
- **Very low resource requirements** — requests only 150m CPU / 160Mi memory; idles at around 9 MiB of memory

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
| `service.port` | `80` | Service port for the UI and API |
| `cors.allowedOrigins` | `[]` | Browser origins allowed to call the API cross-origin; only needed if you host the UI elsewhere |
| `metrics.enabled` | `false` | Serve Prometheus metrics at `/metrics` (see [Metrics](#metrics)) |
| `metrics.serviceMonitor.enabled` / `metrics.podMonitor.enabled` | `false` | Create a prometheus-operator ServiceMonitor or PodMonitor |
| `auth.basic.enabled` | `false` | Built-in HTTP basic auth from an htpasswd Secret (see [Authentication](#authentication)) |
| `auth.basic.existingSecret` / `auth.basic.secretKey` | `""` / `htpasswd` | Secret holding the htpasswd file, and its key |
| `networkPolicy.enabled` | `false` | Restrict traffic to the ingress controller namespace in, and DNS + API server out (see [Security](#security)) |
| `volumePermissions.enabled` | `false` | Chown the PVC to the app user on start; see [Upgrading](#upgrading-to-v0140-single-binary) |
| `podSecurityContext` / `securityContext` | non-root, see [Security](#security) | Pod and container security contexts |
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

## Supported workloads

Pods are grouped by the workload that owns them, worked out from each pod's `ownerReferences` and stored with its logs, so a workload's history stays grouped after its pods are gone. The UI shows each workload's pods merged into one timeline, which can be filtered to one container; any single pod can be opened on its own under **Pods**.

| Workload | Grouped as | Notes |
|---|---|---|
| Deployment | Deployment | Through its ReplicaSets (matched by the `pod-template-hash` label), so every rollout's pods are grouped together. Other controllers that manage ReplicaSets the same way, such as Argo Rollouts, show up as a Deployment named after the controller. |
| StatefulSet | StatefulSet | |
| DaemonSet | DaemonSet | |
| Job | Job | |
| CronJob | CronJob, and each run as a Job | Matched by the `<cronjob>-<timestamp>` names CronJobs give their Jobs, which avoids needing permission to read Jobs. A Job renamed from that pattern shows up only as a Job. |
| Bare pod | Pod | Pods with no owner, including static pods such as a kubeadm control plane. |
| ReplicaSet not owned by a Deployment | ReplicaSet | Available through the API; the UI lists these pods under Pods only. |

Every pod is listed under **Pods** whatever owns it. All containers are collected, including sidecars; init containers are not.

## Security

The image is a single static binary on `gcr.io/distroless/static:nonroot`: no shell and no package manager. The chart runs it as UID/GID 65532 with `runAsNonRoot`, `readOnlyRootFilesystem`, `allowPrivilegeEscalation: false`, all capabilities dropped and the `RuntimeDefault` seccomp profile. `fsGroup: 65532` makes the log PVC writable.

In `hybrid` and `fileTail` modes the chart also adds supplemental group `0` to the pod. The container runtime writes `/var/log/pods` as `root:root` with mode `0750`, and log files as `0640`, so group membership is what lets the non-root user read them. No root UID or extra capabilities are needed on containerd or CRI-O. `api` mode doesn't touch the host at all.

**Docker exception:** on Docker (e.g. Docker Desktop), `/var/lib/docker/containers` is readable by root only, so `hybrid` and `fileTail` need `--set podSecurityContext.runAsUser=0 --set securityContext.runAsNonRoot=false`. Use `api` mode if you'd rather keep a non-root user there.

`networkPolicy.enabled=true` adds a NetworkPolicy that only accepts traffic from the ingress controller's namespace (`networkPolicy.ingressNamespace`, default `ingress-nginx`, plus any `networkPolicy.extraIngressFrom` peers), and only allows egress to DNS and the Kubernetes API server. Log streams from other nodes go through the API server, so kubelets never need to be reachable directly. Set `networkPolicy.apiServerCIDRs` to pin egress to your API server's address.

## Authentication

Authentication is off by default: anyone who can reach the service can read every pod's logs. If it is reachable by anyone who shouldn't, put authentication in front of it. The recommended way is at the edge, in your ingress, so simple-logging never sees an unauthenticated request. For small installs without an ingress to do that, there's a minimal [built-in option](#built-in-basic-auth).

The UI and API share one origin, and the browser sends the ingress's session cookie or basic credentials with every API call, so each of these works without any change to simple-logging.

### oauth2-proxy (OIDC) with ingress-nginx

Run [oauth2-proxy](https://oauth2-proxy.github.io/oauth2-proxy/) against your identity provider, serve it under `/oauth2` on the same host, and point the ingress at it:

```yaml
ingress:
  enabled: true
  className: nginx
  host: logs.example.com
  annotations:
    nginx.ingress.kubernetes.io/auth-url: "https://$host/oauth2/auth"
    nginx.ingress.kubernetes.io/auth-signin: "https://$host/oauth2/start?rd=$escaped_request_uri"
```

### Authelia with Traefik

With [Authelia](https://www.authelia.com) installed, create a ForwardAuth middleware and attach it to the ingress:

```yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: authelia
  namespace: simple-logging
spec:
  forwardAuth:
    address: http://authelia.authelia.svc.cluster.local/api/authz/forward-auth
    trustForwardHeader: true
    authResponseHeaders: [Remote-User, Remote-Groups, Remote-Email, Remote-Name]
```

```yaml
ingress:
  enabled: true
  className: traefik
  host: logs.example.com
  annotations:
    traefik.ingress.kubernetes.io/router.middlewares: simple-logging-authelia@kubernetescrd
```

### Basic auth in the ingress

ingress-nginx reads the htpasswd file from the `auth` key of a Secret:

```bash
htpasswd -c auth alice
kubectl create secret generic logs-basic-auth -n simple-logging --from-file=auth
```

```yaml
ingress:
  annotations:
    nginx.ingress.kubernetes.io/auth-type: basic
    nginx.ingress.kubernetes.io/auth-secret: logs-basic-auth
    nginx.ingress.kubernetes.io/auth-realm: simple-logging
```

Traefik reads it from the `users` key, through a BasicAuth middleware:

```bash
htpasswd -c users alice
kubectl create secret generic logs-basic-auth -n simple-logging --from-file=users
```

```yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: basic-auth
  namespace: simple-logging
spec:
  basicAuth:
    secret: logs-basic-auth
```

```yaml
ingress:
  annotations:
    traefik.ingress.kubernetes.io/router.middlewares: simple-logging-basic-auth@kubernetescrd
```

### Built-in basic auth

When there's no ingress to authenticate for you, simple-logging can check HTTP basic credentials itself. It reads users from an htpasswd file of bcrypt hashes (`htpasswd -B`; other hash schemes are rejected at startup), in an existing Secret:

```bash
htpasswd -cB htpasswd alice
kubectl create secret generic simple-logging-htpasswd -n simple-logging --from-file=htpasswd
```

```bash
helm upgrade simple-logging simple-logging/simple-logging -n simple-logging \
  --set auth.basic.enabled=true \
  --set auth.basic.existingSecret=simple-logging-htpasswd
```

Every request then needs credentials, except `/healthz` and `/readyz` (so probes keep working) and `/metrics` (so Prometheus can scrape it). The browser shows its own login prompt. The file is read at startup, so restart the pod after changing the Secret. Use HTTPS in front of it, since basic auth sends the password with every request.

This is deliberately minimal: no sessions, no logout, and every user can see everything. Use one of the edge options above if you need more.

## API

The UI and the log API share one port and one origin. The API is the `simplelog.v1.LogService` defined in [`proto/simplelog/v1/log_service.proto`](proto/simplelog/v1/log_service.proto), served with [Connect](https://connectrpc.com), so the same endpoint speaks gRPC (including plaintext HTTP/2), gRPC-Web, and Connect's plain JSON over HTTP. Any RPC can be called with `curl`:

```bash
kubectl port-forward -n simple-logging svc/simple-logging 8080:80

curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"namespace":"default"}' \
  http://localhost:8080/simplelog.v1.LogService/ListWorkloads
```

or with `grpcurl -plaintext -proto proto/simplelog/v1/log_service.proto localhost:8080 simplelog.v1.LogService/ListNamespaces`.

Server-streaming RPCs (`StreamLogs`, `StreamWorkloadLogs`, `SearchLogs`) use Connect's framed streaming protocol, so call them from a Connect or gRPC client rather than with a plain JSON POST.

`GET /download?ns=&pod=` (or `&kind=&name=` for a workload, plus optional `container`, `from` and `to` in Unix seconds) streams stored logs as a plain-text file. `/healthz` is the liveness endpoint, `/readyz` returns 503 until startup (including any storage migration) has finished, and `/metrics` serves Prometheus metrics when [enabled](#metrics).

## Retention

`config.retentionDays` (default 30) controls how long log lines are kept. Logs are stored as one file per container per UTC day; retention deletes any day's file once it is strictly older than `retentionDays`, independent of whether the pod is still logging. Retention sweeps at 00:05 UTC every day (as well as at startup and every `config.retentionCheckInterval`), so an expired day is deleted within five minutes of expiring.

The guarantee is therefore: no log line is kept for more than `retentionDays` + 1 day (plus those five minutes). A line written just after midnight UTC is kept for the full extra day, because its whole day's file expires together; a line written just before midnight is kept for barely more than `retentionDays`.

Upgrading from a v0.11 install migrates existing `<namespace>/<pod>.log` files into this layout automatically on first startup (see [Upgrading](#upgrading)); set `MIGRATE_LEGACY=false` to opt out and leave legacy files in place, in which case they're swept by their file modification time instead (matching the old, less precise behaviour) rather than participating in the day-based cutoff.

As a safety net for when retention alone doesn't keep up (e.g. a burst of unusually verbose logging), a background check deletes the globally oldest segments — across every namespace and pod — whenever PVC usage reaches `config.diskHighWaterPercent` (default 90), continuing until usage is back below `config.diskLowWaterPercent` (default 80). This should rarely trigger; `retentionDays` is the primary control.

## Metrics

simple-logging keeps counters about itself: active log streams (by source, node file or API), lines and bytes written per namespace, lines dropped because a storage write failed, API stream reconnects, segments deleted by retention and by the disk guard, disk usage, and server-side search duration and bytes scanned. The storage dashboard in the UI shows the collection counters, and warns if any lines have been dropped. They count from when the server last started.

To scrape them with Prometheus, set `metrics.enabled=true`, which serves them at `/metrics` on the main port. All series are prefixed `simplelog_`, alongside the standard Go runtime and process metrics. With the [prometheus-operator](https://prometheus-operator.dev), also set `metrics.serviceMonitor.enabled=true` (or `metrics.podMonitor.enabled=true`), plus `metrics.serviceMonitor.labels` if your Prometheus selects monitors by label.

`/metrics` is served on the same port as the UI, so with an ingress it's reachable from outside too. It contains counts and namespace names, but no log content. If you use `networkPolicy.enabled`, add your Prometheus namespace to `networkPolicy.extraIngressFrom`.

## Upgrading

```bash
helm repo update
helm upgrade simple-logging simple-logging/simple-logging --namespace simple-logging
```

Upgrading from a v0.11 install (or older) triggers a one-time, automatic migration of existing logs to the current on-disk layout the first time the new version starts. The pod's readiness probe stays failing until migration completes, so `kubectl rollout status` will simply take longer than usual on a large PVC rather than reporting a healthy pod prematurely; existing history is preserved. The PVC itself is reused — no `persistence.claimSuffix` change is needed.

### Upgrading to v1.0.0

v1.0.0 declares the on-disk format, the API, the chart's values and the environment variables stable (see [Versioning](#versioning)). Upgrading from 0.14.x needs no changes to your values, and the default install renders exactly as before. Things to know:

- **Removed RPCs.** `ListDeployments`, `GetDeploymentLogs` and `StreamDeploymentLogs` are gone. They were deprecated in 0.13 in favour of `ListWorkloads`, `GetWorkloadLogs` and `StreamWorkloadLogs` with `kind: "Deployment"`. The UI stopped using them in the same release, so this only affects your own API clients.
- **Metrics and built-in basic auth** are new and off by default; see [Metrics](#metrics) and [Authentication](#authentication).
- **Static pods are now collected** in `hybrid` and `fileTail` modes. Control-plane pods such as etcd and kube-apiserver were previously skipped, so expect them to appear, along with the storage they use.
- **Retention now also sweeps at 00:05 UTC every day**, so expired logs are deleted within minutes of expiring rather than up to a day later.

### Upgrading to v0.14.0 (single binary)

v0.14.0 replaces the nginx + Go image with a single non-root binary serving everything on port 8080. The chart handles the move, but check the following:

- **Removed values.** `grpcWebUrl`, `ingress.grpcPathPrefix`, `config.grpcWebPort`, `config.restDebug`, `service.httpPort` and `service.grpcWebPort` are gone, and `helm upgrade` fails with a list if any are still set. Use `config.port` and `service.port` instead. The Ingress now has a single `/` path. The `/debug/*` REST endpoints are gone because every RPC can now be called as JSON directly (see [API](#api)).
- **Renamed environment variables.** If you set them yourself (e.g. via `extraEnv`): `GRPC_WEB_PORT` is now `PORT`, `GRPC_WEB_URL` is gone (the UI always uses its own origin; `API_URL` exists for the rare split-origin setup), and `REST_DEBUG` is gone.
- **Existing log files are root-owned.** Older images ran as root. Most storage classes apply `fsGroup`, so Kubernetes fixes ownership on mount. Storage classes that ignore it (local-path and other hostPath-backed provisioners, e.g. k3s's default) leave the old files unwritable. The server detects this at startup and exits with an error rather than silently dropping logs. If that happens, upgrade once with `--set volumePermissions.enabled=true`, which chowns the PVC in a short-lived root init container. You can turn it off again afterwards.

## Versioning

simple-logging follows [Semantic Versioning](https://semver.org). From v1.0.0, the on-disk log and index format, the `simplelog.v1` API, the chart's values and the environment variables are stable: minor and patch releases only add to them. A release that breaks one of them is a new major version, and ships with an automatic migration and upgrade notes here. The details are in [ARCHITECTURE.md](ARCHITECTURE.md#compatibility-promises), and every release is listed in the [changelog](CHANGELOG.md).

## Image tags

Two independent tag series are published to [`lsparey/simple-logging`](https://hub.docker.com/r/lsparey/simple-logging):

- **Releases** (`X.Y.Z`, `X.Y`) — built from a tagged release commit and matched by the Helm chart's `appVersion`. This is what `helm install`/`helm upgrade` use by default; prefer these for anything other than local testing.
- **`latest` and `sha-<commit>`** — built from every push to `main`, ahead of the next tagged release. Useful for trying out unreleased fixes, not recommended for production.

## Uninstalling

```bash
helm uninstall simple-logging --namespace simple-logging
```

> **Note:** Uninstalling does not delete the PVC. To remove persisted logs, delete the PVC manually: `kubectl delete pvc -n simple-logging -l app.kubernetes.io/instance=simple-logging`

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setting up, testing and releasing, and [ARCHITECTURE.md](ARCHITECTURE.md) for how simple-logging works.

## License

[MIT](LICENSE)
