.PHONY: toolchain-check generate-contract contract-check test-prepare test-unit test-int test-e2e test

toolchain-check:
	@./scripts/toolchain-check.sh

generate-contract:
	@./scripts/contract-command.sh generate

contract-check:
	@./scripts/contract-command.sh check

test-prepare:
	@./scripts/toolchain-check.sh --manifest-only
	@./scripts/toolchain-check.sh --go-only
	@./scripts/toolchain-check.sh --bun-only
	@bash -n scripts/*.sh
	@bash -n contract/*.sh
	@node scripts/test-validate-product-go-manifest.mjs
	@node contract/scripts/test-validate-go-manifest.mjs
	@GOTOOLCHAIN=local go mod download
	@cd contract && GOTOOLCHAIN=local go mod download
	@cd contract && bun install --frozen-lockfile --ignore-scripts

test-unit:
	@./scripts/toolchain-check.sh --go-only
	@node scripts/test-validate-product-go-manifest.mjs
	@node contract/scripts/test-validate-go-manifest.mjs
	@GOTOOLCHAIN=local go test ./...
	@cd contract && GOTOOLCHAIN=local go test ./...
	@./contract/test-breaking-check.sh
	@./scripts/test-contract-command.sh

test-int:
	@./scripts/toolchain-check.sh --manifest-only
	@./scripts/toolchain-check.sh --go-only
	@GOTOOLCHAIN=local go mod tidy -diff
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
