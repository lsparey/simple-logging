.PHONY: build generate lint lint-go lint-frontend lint-chart test test-go test-unit test-e2e smoke install-hooks

## build: build the frontend and a server binary with it embedded, into server/bin/
build:
	cd frontend && npm run build
	rm -rf server/internal/ui/dist/* && cp -r frontend/dist/. server/internal/ui/dist/
	$(MAKE) -C server build

## generate: regenerate the Go and TypeScript API code from proto/ (needs buf)
generate:
	$(MAKE) -C server generate

## lint: run every linter (golangci-lint, eslint + tsc, helm lint)
lint: lint-go lint-frontend lint-chart

## lint-go: run golangci-lint on the server (needs golangci-lint v2)
lint-go:
	cd server && golangci-lint run ./...

## lint-frontend: run eslint and typecheck the app and the e2e tests
lint-frontend:
	cd frontend && npm run lint -- --max-warnings 0 && npx tsc -b && npx tsc -p e2e --noEmit

## lint-chart: lint the Helm chart
lint-chart:
	helm lint deploy/helm/simple-logging

## test: run all tests (Go, frontend unit, and Playwright E2E)
test: test-go test-unit test-e2e

## test-go: run the Go server tests
test-go:
	$(MAKE) -C server test

## test-unit: run the frontend Vitest unit tests
test-unit:
	cd frontend && npm run test:run

## test-e2e: run the Playwright E2E tests
test-e2e:
	cd frontend && npm run test:e2e

## smoke: run the end-to-end smoke test against the current kubectl context
##   (a disposable cluster, e.g. kind); IMAGE must already be loaded into it
smoke:
	IMAGE=$(IMAGE) scripts/smoke-test.sh

## install-hooks: install the pre-push git hook
install-hooks:
	cp scripts/pre-push .git/hooks/pre-push
	chmod +x .git/hooks/pre-push
	@echo "pre-push hook installed"
