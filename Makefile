SHELL:=/bin/bash
GO = go
GO_FMT = $(GO) tool gofumpt
GO_IMPORTS = $(GO) tool goimports
STATIC_CHECK = $(GO) tool staticcheck
AQUA = aqua
PINACT = pinact

GO_VET_OPTS = -v
GO_TEST_OPTS = -v -race
GO_FMT_OPTS = -l -w
GO_IMPORTS_OPTS = -w -local github.com/chez-shanpu/tastecheck

.PHONY: fmt
fmt:
	$(GO_FMT) $(GO_FMT_OPTS) .
	$(GO_IMPORTS) $(GO_IMPORTS_OPTS) .

.PHONY: fix
fix:
	$(GO) fix ./...

.PHONY: mod
mod:
	$(GO) mod tidy

.PHONY: check-diff
check-diff: mod fmt fix
	git diff --exit-code --name-only

.PHONY: vet
vet:
	$(GO) vet $(GO_VET_OPTS) ./...

.PHONY: test
test:
	$(STATIC_CHECK) ./...
	$(GO) test $(GO_TEST_OPTS) ./...

.PHONY: build
build:
	$(GO) build -ldflags "-X github.com/chez-shanpu/tastecheck/cmd.version=dev \
	  -X github.com/chez-shanpu/tastecheck/cmd.commit=$$(git rev-parse --short HEAD 2>/dev/null || echo 'none')" \
	  -o bin/tastecheck .

.PHONY: clean
clean:
	-$(GO) clean
	-rm -rf bin/

.PHONY: test-e2e
test-e2e:
	@$(MAKE) -C test test

.PHONY: check-goreleaser
check-goreleaser:
	goreleaser check

.PHONY: check
check: vet check-diff test check-goreleaser

.PHONY: check-all
check-all: check build test-e2e

.PHONY: update-aqua
update-aqua:
	$(AQUA) update
	$(AQUA) update-checksum --prune

.PHONY: update-aqua-version
update-aqua-version:
	v=$$(gh api repos/aquaproj/aqua/releases/latest --jq .tag_name) && \
	  sed -i.bak "s|aqua_version: v.*|aqua_version: $$v|" .github/workflows/*.yaml && \
	  rm -f .github/workflows/*.bak

.PHONY: update-actions
update-actions:
	$(PINACT) run -u

.PHONY: update-go-tools
update-go-tools:
	$(GO) get tool
	$(GO) mod tidy

.PHONY: update
update: update-aqua update-aqua-version update-actions update-go-tools

.PHONY: all
all: check build

.DEFAULT_GOAL=all
