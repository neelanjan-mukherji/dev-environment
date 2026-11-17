.PHONY: test lint test-containers check verify

SHELL_SCRIPTS := install.sh ci/*.sh

## Fast checks: shellcheck + render tests (no Docker, never touches $HOME)
check: lint test

lint:
	shellcheck -s sh install.sh
	shellcheck ci/*.sh

test:
	ci/test-render.sh $(FILTER)

## Assert this machine matches its profile (same checks as the containers)
verify:
	ci/verify.sh $(or $(PROFILE),$(shell chezmoi data --format json | sed -n 's/.*"profile": "\(.*\)".*/\1/p'))

## Full install + verification in clean Ubuntu/Debian containers (needs Docker)
test-containers:
	ci/run-containers.sh $(BASES)
