.PHONY: test lint test-containers check

SHELL_SCRIPTS := install.sh ci/*.sh

## Fast checks: shellcheck + render tests (no Docker, never touches $HOME)
check: lint test

lint:
	shellcheck -s sh install.sh
	shellcheck ci/*.sh

test:
	ci/test-render.sh $(FILTER)

## Full install + verification in clean Ubuntu/Debian containers (needs Docker)
test-containers:
	ci/run-containers.sh $(BASES)
