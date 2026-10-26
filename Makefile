.PHONY: help build test lint fmt vet licences ci install install-latest web-dev web-check web-build dist docs docs-serve clean

# The public targets, each with its ## line: a bare make prints them.
help: ## List the make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

build: ## Build every package
	go build ./...

test: ## Run the Go tests
	go test ./...

vet: ## Run go vet
	go vet ./...

fmt: ## List the files gofmt would change
	gofmt -l .

lint: vet fmt ## vet, then fmt

# Every module the build and its tests use, with its licence from the
# module cache; fails on any GPL, LGPL or AGPL. Then the web app's runtime
# packages, from web/package-lock.json; fails on any licence outside the
# permissive ones (go test runs both).
licences: ## Check every licence, Go modules and the web app's packages
	go test -count=1 -run TestLicences -v ./cmd/git-board/

# Everything CI runs: .github/workflows/ci.yml only sets up Go and Node, then
# runs this. The web checks and build come first, in prerequisite order,
# because the binary embeds web/dist.
ci: web-check web-build build vet test ## Everything CI runs
	@test -z "$$(gofmt -l .)" || (echo "gofmt needs to be run on:"; gofmt -l .; exit 1)

# The web app first: the binary embeds whatever web/dist holds.
install: web-build ## Build the web app, then install git-board in GOBIN
	go install ./cmd/git-board

# The latest GitHub release's git-board for this machine, web app inside,
# checked against its checksums.txt and put where go install puts it.
install-latest: ## Install the latest GitHub release's git-board, checksums checked
	@set -e; d=$$(mktemp -d); trap 'rm -rf "$$d"' EXIT; \
	gh release download -D "$$d" -p "*_$$(go env GOOS)_$$(go env GOARCH).tar.gz" -p checksums.txt; \
	(cd "$$d" && shasum -a 256 -c checksums.txt --ignore-missing && tar -xzf git-board_*.tar.gz); \
	bin=$$(go env GOBIN); bin=$${bin:-$$(go env GOPATH)/bin}; \
	rm -f "$$bin/git-board" && cp "$$d"/git-board_*/git-board "$$bin/" && "$$bin/git-board" --version

web-dev: ## Run the web app's dev server
	cd web && npm run dev

# The web app's dependencies, installed only when the lockfile is newer than
# web/node_modules: a local run stays fast, and CI goes through the same path.
web/node_modules: web/package-lock.json
	cd web && npm ci
	@touch web/node_modules

web-check: web/node_modules ## Typecheck and test the web app
	cd web && npm run typecheck && npm test

web-build: web/node_modules ## Build the web app into web/dist
	cd web && npm run build

# A release's archives, the web app inside, in dist/ with checksums.txt:
# run on a clean checkout of the tag, so the commands report that version.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
dist: web-build ## Build the release archives and checksums.txt into dist/
	rm -rf dist && mkdir dist
	@set -e; v=$$(git describe --tags --always --dirty); for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; d=git-board_$${v}_$${os}_$${arch}; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "-s -w" -o dist/$$d/ ./cmd/git-board; \
		cp LICENSE README.md dist/$$d/; \
		if [ $$os = windows ]; then (cd dist && zip -qr $$d.zip $$d); else tar -C dist -czf dist/$$d.tar.gz $$d; fi; \
		rm -rf dist/$$d; echo "dist/$$d"; \
	done; cd dist && shasum -a 256 * > checksums.txt

# The docs site (zensical.toml: docs/, the guide, the core's spec, the
# decisions) in site/. Strict: a broken link or a missing page fails the
# build.
docs: ## Build the docs site (guide, spec, decisions) into site/ (strict)
	uvx --from zensical==0.0.72 zensical build --strict

docs-serve: ## Preview the docs site on localhost, rebuilt on save
	uvx --from zensical==0.0.72 zensical serve

clean: ## Remove the built site
	rm -rf site/
