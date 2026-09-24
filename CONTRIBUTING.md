# Contributing

Thanks for helping. This covers setting up, running and testing simple-logging, and how releases are cut. [ARCHITECTURE.md](ARCHITECTURE.md) explains how it works.

## Prerequisites

| Tool | Used for |
|---|---|
| Go (the version in `server/go.mod`) | The server |
| Node.js 22 | The frontend |
| [buf](https://buf.build/docs/installation) | Regenerating API code from `proto/` |
| [golangci-lint](https://golangci-lint.run) v2 | Linting the server |
| Helm 3 | Linting and installing the chart |
| Docker and [kind](https://kind.sigs.k8s.io) | Running it in a real cluster, and the smoke test |

## Layout

| Path | What |
|---|---|
| `server/` | The Go binary: collector, storage, API, embedded UI |
| `frontend/` | The React UI |
| `proto/` | The API definition, source for `server/gen/` and `frontend/src/gen/` |
| `deploy/helm/simple-logging/` | The Helm chart |
| `scripts/` | The smoke test and the pre-push hook |

## Working on the UI

The frontend's e2e tests use a mock backend that also works for development, with no cluster needed:

```bash
cd frontend
npm ci
npx tsx e2e/mock-server.ts                        # mock API on :8081
VITE_API_URL=http://localhost:8081 npm run dev    # UI on :5173, in another terminal
```

A new RPC needs an implementation in `e2e/mock-server.ts` too.

## Running the server

The server reads its Kubernetes credentials from the in-cluster service account, so it runs inside a cluster. kind is the quickest way:

```bash
kind create cluster --name sl-dev
docker build -t simple-logging:dev .            # needs BuildKit (docker buildx)
kind load docker-image simple-logging:dev --name sl-dev
helm install sl deploy/helm/simple-logging -n simple-logging --create-namespace \
  --set image.repository=simple-logging --set image.tag=dev --set image.pullPolicy=Never
kubectl -n simple-logging port-forward svc/sl-simple-logging 8080:80
```

Then open http://localhost:8080. To try a new build, rebuild, `kind load` again, and `kubectl -n simple-logging rollout restart deploy/sl-simple-logging`.

## Changing the API

Edit `proto/simplelog/v1/log_service.proto`, then:

```bash
make generate
```

Commit the regenerated `server/gen/` and `frontend/src/gen/` along with the proto change. Within v1, changes to the API must be additive; see [Compatibility promises](ARCHITECTURE.md#compatibility-promises).

## Checks

```bash
make lint        # golangci-lint, eslint, tsc (app and e2e), helm lint
make test-go     # Go tests
make test-unit   # frontend unit tests (Vitest)
make test-e2e    # Playwright e2e tests against the mock backend
make test        # all three test suites
```

`make install-hooks` installs a pre-push hook that runs the linters and every test suite.

The smoke test installs the chart on a real cluster, runs a pod that logs, and checks the lines come back through the API. Point `kubectl` at a disposable cluster with the image loaded:

```bash
make smoke IMAGE=simple-logging:dev
```

CI runs all of this on every pull request: the linters and tests, `ct lint`, a Trivy scan of the image, `ct install` for each `deploy/helm/simple-logging/ci/*-values.yaml`, and the smoke test in hybrid and api modes.

If a chart change adds a feature, keep it off by default, so that `helm template` with default values renders exactly as before.

## Pull requests

- Keep each pull request to one change, and describe what it changes for users.
- Add or update tests alongside the change.
- Update the README for anything user-visible, and ARCHITECTURE.md for changes to how it works.

## Releases

Maintainers cut releases from the **Release** workflow in GitHub Actions (Actions → Release → Run workflow), choosing `patch`, `minor` or `major`, with optional release notes. Without notes, the commit subjects since the last tag are used. The workflow:

1. bumps `version` and `appVersion` in `Chart.yaml`,
2. adds the release notes to the top of `CHANGELOG.md`,
3. commits both, tags `vX.Y.Z` and pushes,
4. builds and pushes the multi-arch image as `lsparey/simple-logging:X.Y.Z` and `:X.Y`, and
5. publishes the chart to the Helm repository on `gh-pages`.

Versioning follows [SemVer](https://semver.org). A major version is needed for anything that breaks a [compatibility promise](ARCHITECTURE.md#compatibility-promises): the on-disk format, the API, the chart's values or the environment variables.
