.PHONY: test lint test-containers check verify build go-test

GO_BUILD := cd cli && CGO_ENABLED=0 go build -trimpath -o dist/dev .

SHELL_SCRIPTS := install.sh ci/*.sh

## Fast checks: shellcheck, go vet + tests, render tests (no Docker, never touches $HOME)
check: lint go-test test

lint:
	shellcheck -s sh install.sh
	shellcheck ci/*.sh

test: build
	ci/test-render.sh $(FILTER)

go-test:
	cd cli && go vet ./... && go test ./...

## The dev binary for this machine: cli/dist/dev
build:
	$(GO_BUILD)

## Assert this machine matches its profile (same checks as the containers)
verify: build
	ci/verify.sh

## Full install + verification in clean Ubuntu/Debian containers (needs Docker)
test-containers:
	ci/run-containers.sh $(BASES)
