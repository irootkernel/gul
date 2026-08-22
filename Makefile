.PHONY: toolchain-check generate-contract contract-check test-prepare test-unit test-int test-e2e test

toolchain-check:
	@./scripts/toolchain-check.sh

generate-contract:
	@./scripts/contract-command.sh generate

contract-check:
	@./scripts/contract-command.sh check

test-prepare:
	@./scripts/toolchain-check.sh --manifest-only
	@bash -n scripts/*.sh

test-unit:
	@./scripts/test-contract-command.sh

test-int:
	@./scripts/toolchain-check.sh --manifest-only
	@./scripts/check-sot.sh
	@git diff --check

test-e2e:
	@./scripts/test-toolchain-check.sh

test:
	@$(MAKE) --no-print-directory test-prepare
	@$(MAKE) --no-print-directory test-unit
	@$(MAKE) --no-print-directory test-int
	@$(MAKE) --no-print-directory test-e2e
