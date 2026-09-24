#!/usr/bin/env bash
# End-to-end smoke test against a real cluster: installs the chart, runs a
# pod that logs, and checks its lines come back through the JSON API.
#
# Needs kubectl, helm, curl and jq, with the current kubectl context pointing
# at a disposable cluster (e.g. kind) where IMAGE is already available:
#
#   kind create cluster --name sl-smoke
#   kind load docker-image simple-logging:ci --name sl-smoke
#   IMAGE=simple-logging:ci scripts/smoke-test.sh
#
# Any extra arguments are passed to `helm install`, e.g.
# --set config.logCollectionMode=api to test the other collection mode.

set -euo pipefail

IMAGE="${IMAGE:?set IMAGE to the image to test, e.g. simple-logging:ci}"
RELEASE="${RELEASE:-smoke}"
NAMESPACE="${NAMESPACE:-simple-logging-smoke}"
LOGGER_NAMESPACE="${LOGGER_NAMESPACE:-smoke-logger}"
LOCAL_PORT="${LOCAL_PORT:-18090}"
MARKER="smoke-test-marker-$RANDOM$RANDOM"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PF_PID=""

cleanup() {
  status=$?
  [[ -n "$PF_PID" ]] && kill "$PF_PID" 2>/dev/null || true
  if [[ $status -ne 0 ]]; then
    echo "==> Smoke test failed; simple-logging's own logs:" >&2
    kubectl -n "$NAMESPACE" logs "deploy/$RELEASE-simple-logging" --tail=100 >&2 || true
  fi
  helm uninstall "$RELEASE" -n "$NAMESPACE" --wait >/dev/null 2>&1 || true
  kubectl delete namespace "$NAMESPACE" "$LOGGER_NAMESPACE" --ignore-not-found --timeout=90s >/dev/null 2>&1 || true
  exit $status
}
trap cleanup EXIT

rpc() {
  curl -sf -X POST -H 'Content-Type: application/json' -d "$2" \
    "http://localhost:$LOCAL_PORT/simplelog.v1.LogService/$1"
}

# stream_rpc calls a server-streaming RPC. Connect's streaming protocol
# frames each message with a flag byte and a 4-byte big-endian length, so the
# request is wrapped in one such envelope; the raw framed response is printed.
stream_rpc() {
  local body=$2 n=${#2}
  {
    printf '\x00'
    # shellcheck disable=SC2059 # the inner printf builds the \x escapes
    printf "$(printf '\\x%02x\\x%02x\\x%02x\\x%02x' $((n >> 24 & 255)) $((n >> 16 & 255)) $((n >> 8 & 255)) $((n & 255)))"
    printf '%s' "$body"
  } | curl -sf -X POST -H 'Content-Type: application/connect+json' --data-binary @- \
    "http://localhost:$LOCAL_PORT/simplelog.v1.LogService/$1"
}

# retry <seconds> <description> <command...> runs command until it succeeds.
retry() {
  local deadline=$((SECONDS + $1)) what=$2
  shift 2
  until "$@"; do
    if ((SECONDS >= deadline)); then
      echo "Timed out waiting for $what" >&2
      return 1
    fi
    sleep 2
  done
}

echo "==> Installing chart with $IMAGE"
helm install "$RELEASE" "$ROOT/deploy/helm/simple-logging" \
  --namespace "$NAMESPACE" --create-namespace \
  --set image.repository="${IMAGE%:*}" \
  --set image.tag="${IMAGE##*:}" \
  --set image.pullPolicy=Never \
  --wait --timeout 3m \
  "$@"

echo "==> Starting a pod that logs $MARKER"
kubectl create namespace "$LOGGER_NAMESPACE"
kubectl -n "$LOGGER_NAMESPACE" run logger --image=busybox:1.37 --restart=Never -- \
  sh -c "i=0; while true; do i=\$((i+1)); echo \"$MARKER line \$i\"; sleep 1; done"
kubectl -n "$LOGGER_NAMESPACE" wait --for=condition=Ready pod/logger --timeout=2m

kubectl -n "$NAMESPACE" port-forward "svc/$RELEASE-simple-logging" "$LOCAL_PORT:80" >/dev/null 2>&1 &
PF_PID=$!
retry 30 "the port-forward" curl -sf "http://localhost:$LOCAL_PORT/healthz" -o /dev/null

echo "==> Checking the UI is served"
curl -sf "http://localhost:$LOCAL_PORT/" | grep -q '<div id="root">'

has_logger_lines() {
  rpc GetWorkloadLogs "{\"namespace\":\"$LOGGER_NAMESPACE\",\"kind\":\"Pod\",\"name\":\"logger\",\"loadLastPage\":true}" |
    jq -e --arg m "$MARKER" '[.lines[]? | select(contains($m))] | length >= 3' >/dev/null
}
echo "==> Waiting for the pod's lines to be collected"
retry 90 "the logger pod's lines via GetWorkloadLogs" has_logger_lines

echo "==> Checking the namespace and pod are listed"
rpc ListNamespaces '{}' | jq -e --arg ns "$LOGGER_NAMESPACE" '.namespaces | index($ns) != null' >/dev/null
rpc ListPods "{\"namespace\":\"$LOGGER_NAMESPACE\"}" | jq -e '.pods[] | select(.name == "logger" and .active)' >/dev/null

echo "==> Checking server-side search finds the marker"
stream_rpc SearchLogs "{\"namespace\":\"$LOGGER_NAMESPACE\",\"query\":\"$MARKER\",\"maxResults\":1}" | grep -q "$MARKER"

echo "==> Checking download"
curl -sf "http://localhost:$LOCAL_PORT/download?ns=$LOGGER_NAMESPACE&pod=logger" | grep -q "$MARKER"

echo "==> Checking self-metrics"
rpc GetStats '{}' | jq -e '(.linesWrittenTotal | tonumber) > 0 and ((.streamsActiveFile // "0" | tonumber) + (.streamsActiveApi // "0" | tonumber)) > 0' >/dev/null

echo "Smoke test passed."
