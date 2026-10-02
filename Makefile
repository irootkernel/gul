.PHONY: toolchain-check generate-contract contract-check generate-frontend frontend-check generate-api api-check test-prepare test-unit test-int test-e2e test

toolchain-check:
	@./scripts/toolchain-check.sh

generate-contract:
	@./scripts/contract-command.sh generate

contract-check:
	@./scripts/contract-command.sh check

generate-frontend:
	@./scripts/toolchain-check.sh --bun-only
	@bun install --frozen-lockfile --ignore-scripts
	@node scripts/validate-product-package.mjs
	@bun frontend/scripts/build.mjs

frontend-check:
	@./scripts/toolchain-check.sh --bun-only
	@bun install --frozen-lockfile --ignore-scripts
	@node scripts/validate-product-package.mjs
	@bun frontend/scripts/check-bundle.mjs

generate-api:
	@./scripts/toolchain-check.sh --api-only
	@cd contract && bun install --frozen-lockfile --ignore-scripts
	@node api/scripts/api-command.mjs generate
	@node contract/port/generate-errors.mjs generate

api-check:
	@./scripts/toolchain-check.sh --api-only
	@cd contract && bun install --frozen-lockfile --ignore-scripts
	@node api/scripts/api-command.mjs check
	@node contract/port/generate-errors.mjs check

test-prepare:
	@./scripts/toolchain-check.sh --manifest-only
	@./scripts/toolchain-check.sh --go-only
	@./scripts/toolchain-check.sh --bun-only
	@bash -n scripts/*.sh
	@bash -n contract/*.sh
	@node scripts/test-validate-product-go-manifest.mjs
	@node scripts/test-validate-product-package.mjs
	@node scripts/test-validate-product-makefile.mjs
	@node scripts/test-validate-frontend-config.mjs
	@node scripts/test-foundation-boundaries.mjs
	@node contract/scripts/test-validate-go-manifest.mjs
	@GOTOOLCHAIN=local go mod download
	@bun install --frozen-lockfile --ignore-scripts
	@cd contract && GOTOOLCHAIN=local go mod download
	@cd contract && bun install --frozen-lockfile --ignore-scripts

test-unit:
	@./scripts/toolchain-check.sh --go-only
	@node contract/scripts/test-release-policy.mjs
	@node scripts/test-validate-product-go-manifest.mjs
	@node scripts/test-validate-product-package.mjs
	@node scripts/test-validate-product-makefile.mjs
	@node scripts/test-validate-frontend-config.mjs
	@node scripts/test-foundation-boundaries.mjs
	@node contract/scripts/test-validate-go-manifest.mjs
	@bun run typecheck
	@bun run test
	@GOTOOLCHAIN=local go test -race ./...
	@cd contract && GOTOOLCHAIN=local go test -race ./...
	@./contract/test-breaking-check.sh
	@./scripts/test-contract-command.sh

test-int:
	@./scripts/toolchain-check.sh --manifest-only
	@./scripts/toolchain-check.sh --go-only
	@GOTOOLCHAIN=local go mod tidy -diff
	@$(MAKE) --no-print-directory frontend-check
	@$(MAKE) --no-print-directory api-check
	@node scripts/test-sot.mjs
	@./scripts/check-sot.sh
	@./scripts/contract-command.sh check
	@git --no-pager diff --check

test-e2e:
	@./scripts/test-toolchain-check.sh

test:
	@$(MAKE) --no-print-directory test-prepare
	@$(MAKE) --no-print-directory test-unit
	@$(MAKE) --no-print-directory test-int
	@$(MAKE) --no-print-directory test-e2e
