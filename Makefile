.PHONY: test test-go test-unit test-e2e install-hooks

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

## install-hooks: install the pre-push git hook
install-hooks:
	cp scripts/pre-push .git/hooks/pre-push
	chmod +x .git/hooks/pre-push
	@echo "pre-push hook installed"
