# AGENTS.md

simple-logging: one Go binary (`server/`) that collects Kubernetes pod logs to a PVC and serves a React UI (`frontend/`) and a Connect API (`proto/`) on one port, installed by the Helm chart in `deploy/helm/simple-logging`.

Read [ARCHITECTURE.md](ARCHITECTURE.md) before changing the collector, the storage layout, indexes, retention, or the API. Its "Compatibility promises" section lists what v1 must keep reading.

## Commands

The root `Makefile` is the entry point (`make lint`, `make test-go`, `make test-unit`, `make test-e2e`, `make generate`, `make build`, `make smoke`); each target's comment says what it needs. Things the Makefile doesn't tell you:

- The server only runs inside a cluster (in-cluster config). To see a change working, build an image, load it into kind, and run `scripts/smoke-test.sh`; its header shows how. For UI work, run the frontend against the mock backend: `npx tsx e2e/mock-server.ts` plus `VITE_API_URL=http://localhost:8081 npm run dev` in `frontend/`.
- After editing `proto/`, run `make generate` and commit the regenerated `server/gen/` and `frontend/src/gen/` with it. Generated code is only ever changed that way.

## Invariants

- **On-disk format is v1-stable.** Segment paths `<ns>/<pod>/<container>/<YYYY-MM-DD>.log`, the line format `<RFC3339Nano> [<ns>/<pod>/<container>] <line>`, `meta.json`, and the index manifest and shard format (`formatVersion` 3, magic `SLI3`). A change to any of them ships with a migration that runs at startup, and bumps the major version.
- **A line's segment comes from the line's own timestamp**, taken from the source (CRI, Docker, or the API's timestamps), in UTC. Retention deletes whole segments by the date in the file name. That is what makes the retention guarantee exact.
- **Storage write failures drop and count the line.** The stream keeps reading, and the drop shows up in `simplelog_lines_dropped_total`.
- **API changes are additive within v1.** Add RPCs and fields with new field numbers. Removing or renumbering needs a major version.
- **Opt-in features leave the default install unchanged.** New chart features default off, and `helm template` with default values renders exactly as before, so check it against `main`. A removed or renamed values key goes into `simple-logging.failOnRemovedValues` in `_helpers.tpl`, so upgrades fail with a message instead of ignoring it.
- **The Deployment stays a singleton**: `replicas: 1`, `Recreate`, one ReadWriteOnce PVC. Multi-node coverage comes from hybrid mode, not extra replicas or a DaemonSet.
- **The image runs as non-root** (UID 65532, read-only root filesystem, no capabilities). Host log access comes from supplemental group 0, not root.

## Conventions

- Go tests use the standard library only, with the fake clientset for Kubernetes. Frontend unit tests use Vitest and Testing Library; e2e tests use Playwright against `e2e/mock-server.ts`, so a new RPC also needs a mock implementation there.
- Releases are cut by the `Release` workflow (see CONTRIBUTING.md). Leave `Chart.yaml` versions and `CHANGELOG.md` to it.
