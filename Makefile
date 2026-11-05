.PHONY: help build test test-pg perf web-e2e lint fmt vet licences ci compat dogfood golden golden-boards agents install install-latest web-dev web-check web-lint web-build next-web-dev next-web-check next-web-build next-web-e2e dist docs docs-serve clean

# The public targets, each with its ## line: a bare make prints them.
help: ## List the make targets
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

build: ## Build every package
	go build ./...

test: ## Run the Go tests
	go test ./...

test-pg: ## Run the PostgreSQL tests too, the index's on PostgreSQL (needs GIT_BOARD_TEST_POSTGRES)
	@test -n "$$GIT_BOARD_TEST_POSTGRES" || { echo "GIT_BOARD_TEST_POSTGRES: a DSN whose user may create databases"; exit 1; }
	go test -count=1 ./core/... ./internal/store/...
	GIT_BOARD_TEST_INDEX=pg go test -count=1 ./core/index/

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

# Everything CI's test job runs: .github/workflows/ci.yml only sets up Go and
# Node, then runs this (its e2e job runs web-e2e). The web checks and build come first, in prerequisite order,
# because the binary embeds web/dist.
ci: web-check web-build next-web-check next-web-build build vet test compat ## Everything CI runs
	@test -z "$$(gofmt -l .)" || (echo "gofmt needs to be run on:"; gofmt -l .; exit 1)

# git-board-agents (contrib/board-agents): v1's agents and wtx, out of the
# binary (ADR 0021). A module of its own, so ./... above never reaches it,
# and not in ci: it builds and tests on its own, until it has its own repository.
agents: ## Build, vet and test contrib/board-agents, its own module (not in make ci)
	cd contrib/board-agents && go build ./... && go vet ./... && go test ./...

# v2's contract with v1 (docs/rewrite/compat.md): the golden boards in
# next/testdata/golden, v1's outputs on them, and the HTTP API's
# transcripts. compat runs a binary over the boards and compares, allowing
# only next/testdata/compat/allowed.txt's differences: v1's git-board,
# built now, unless BIN names another (BIN=$$(go env GOPATH)/bin/git-board-next).
BIN ?=
compat: ## Run a binary over the golden boards and compare with v1's outputs (BIN=path, default v1)
	@set -e; bin="$(BIN)"; if [ -z "$$bin" ]; then d=$$(mktemp -d); trap 'rm -rf "$$d"' EXIT; bin=$$d/git-board; go build -o $$bin ./cmd/git-board; fi; \
	GIT_BOARD_COMPAT_PENDING=$(if $(BIN),1,) GIT_BOARD_COMPAT_BIN=$$(cd $$(dirname $$bin) && pwd)/$$(basename $$bin) go test -count=1 -run 'TestCompat|TestGoldensComplete' ./next/golden/

# The dogfooding job (design.md, section 5; .github/workflows/dogfood.yml
# runs it daily on this repository's board): a real board read by two
# binaries, ls --json and show --json of every issue compared with
# make compat's normalisations (next/golden TestDogfood). DIR is the
# repository, never written (each binary reads its own copy). OLD is
# v1's binary, else v1 built from OLD_REF's ./cmd/git-board (default:
# the newest v0.11.* tag); NEW is v2's, else built now (next/'s, or the
# root's once next/ moved there). The differing cases' outputs go to
# DOGFOOD_OUT, with their diff in DOGFOOD_OUT/dogfood.diff.
DIR ?=
OLD ?=
NEW ?=
OLD_REF ?= $(shell git tag -l 'v0.11.*' --sort=-v:refname | head -1)
DOGFOOD_OUT ?= dogfood-out
dogfood: ## Diff ls/show --json of every issue of a real board, v1 vs v2 (DIR=repo, OLD=bin or OLD_REF=tag, NEW=bin)
	@set -e; test -n "$(DIR)" || { echo "make dogfood DIR=<repository> [OLD=<binary> | OLD_REF=<ref>] [NEW=<binary>]" >&2; exit 2; }; \
	d=$$(mktemp -d); trap 'rm -rf "$$d"' EXIT; old="$(OLD)"; new="$(NEW)"; \
	if [ -z "$$old" ]; then test -n "$(OLD_REF)" || { echo "dogfood: no v0.11 tag here (git fetch --tags), pass OLD_REF or OLD" >&2; exit 2; }; \
		echo "dogfood: OLD is v1 built from $(OLD_REF)"; mkdir "$$d/src"; git archive "$(OLD_REF)" | tar -x -C "$$d/src"; \
		(cd "$$d/src" && go build -o "$$d/old" ./cmd/git-board); old="$$d/old"; fi; \
	if [ -z "$$new" ]; then pkg=./cmd/git-board; if [ -d next/cmd/git-board-next ]; then pkg=./next/cmd/git-board-next; fi; \
		echo "dogfood: NEW is $$pkg, built now"; go build -o "$$d/new" $$pkg; new="$$d/new"; fi; \
	rm -rf "$(DOGFOOD_OUT)"; mkdir -p "$(DOGFOOD_OUT)"; out=$$(cd "$(DOGFOOD_OUT)" && pwd); \
	status=0; GIT_BOARD_DOGFOOD_DIR="$$(cd "$(DIR)" && pwd)" GIT_BOARD_DOGFOOD_OLD="$$old" GIT_BOARD_DOGFOOD_NEW="$$new" \
		GIT_BOARD_DOGFOOD_OUT="$$out" go test -count=1 -timeout 180m -run '^TestDogfood$$' ./next/golden/ || status=$$?; \
	if [ -d "$$out/old" ]; then (cd "$$out" && diff -ru old new > dogfood.diff || true); echo "dogfood: the differences are in $$out/dogfood.diff"; fi; \
	exit $$status

# The timed budgets (docs/guide/performance.md, v2): boardgen's boards of
# 5,000 and 20,000 issues, made once for a seed in PERF_CACHE, each run on
# a copy (the writes add to it); the CLI's and the server's benchmarks,
# PERF_COUNT runs of each; their medians against
# next/testdata/perf/budgets.txt. Fails when one is over its budget by
# more than PERF_TOLERANCE. Meant for a quiet machine (CI's nightly job),
# not make ci: a shared runner's noise would fail it.
PERF_CACHE ?= $(HOME)/.cache/git-board/perf
PERF_OUT ?= perf-results
PERF_COUNT ?= 5
PERF_TOLERANCE ?= 0.15
PERF_SEED ?= 1
PERF_BOARDS ?= 5k:5000 20k:20000
perf: ## Time the CLI and the server on 5k and 20k issues against next/testdata/perf/budgets.txt
	@set -e; mkdir -p "$(PERF_CACHE)" "$(PERF_OUT)"; args=""; \
	for spec in $(PERF_BOARDS); do label=$${spec%%:*}; n=$${spec#*:}; \
		board="$(PERF_CACHE)/boardgen-n$$n-seed$(PERF_SEED)"; \
		if [ ! -d "$$board" ]; then rm -rf "$$board.tmp"; go run ./cmd/boardgen -n $$n -seed $(PERF_SEED) "$$board.tmp"; mv "$$board.tmp" "$$board"; fi; \
		w=$$(mktemp -d); trap 'rm -rf "$$w"' EXIT; cp -r "$$board" "$$w/board"; \
		printf '\n[experimental]\nfeatures = ["cycle"]\n' >> "$$w/board/.git-board.toml"; \
		echo "perf: $$label ($$n issues)"; \
		GIT_BOARD_BENCH_REPO="$$w/board" go test ./next/surface/cli ./next/surface/http -run - -bench . -benchtime 5x -count $(PERF_COUNT) -timeout 60m | tee "$(PERF_OUT)/bench-$$label.txt"; \
		rm -rf "$$w"; args="$$args $$label=$(PERF_OUT)/bench-$$label.txt"; \
	done; \
	go run ./next/tools/perfcheck -budgets next/testdata/perf/budgets.txt -tolerance $(PERF_TOLERANCE) -report $(PERF_OUT)/report.md $$args

# Records what v1 prints on the golden boards, and what its API answers:
# run it when a board or a command changes, and review the diff. v1 must
# then pass make compat, byte for byte.
golden: ## Record v1's outputs on the golden boards and the API's transcripts
	@set -e; d=$$(mktemp -d); trap 'rm -rf "$$d"' EXIT; go build -o $$d/git-board ./cmd/git-board; \
	GIT_BOARD_COMPAT_BIN=$$d/git-board GIT_BOARD_GOLDEN_UPDATE=1 go test -count=1 -run TestCompat ./next/golden/; \
	GIT_BOARD_GOLDEN_UPDATE=1 go test -count=1 -run TestAPITranscripts ./internal/server/

# Makes the golden boards again with v1's code, then make golden. Their
# ids stay but the generated board's, whose nonces are random.
golden-boards: ## Make the golden boards again (next/golden/v1gen), then record their outputs
	go run ./next/golden/v1gen
	$(MAKE) golden

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

# The EPFL design system's adherence linter (web/README.md), from the skill
# cloned next to this repo. web/src/css/tokens holds the raw values, as the
# skill's own tokens folder does, so it is skipped. It reports and never fails: not part of ci until
# the count is 0 (docs/prd/issue-head-m2-plan.md, H-3).
EPFL_SKILL ?= ../epfl-design-skill
web-lint: ## Report the EPFL adherence linter's problems in web/src (needs EPFL_SKILL)
	@test -f $(EPFL_SKILL)/adherence/.stylelintrc.json || (echo "web-lint: no $(EPFL_SKILL)/adherence/.stylelintrc.json; clone EPFL-ENAC/epfl-design-skill there or set EPFL_SKILL"; exit 1)
	-npx --yes --package=stylelint@16 --package=postcss-html -- stylelint "web/src/**/*.{css,vue}" --ignore-pattern "web/src/css/tokens/**" --config $(EPFL_SKILL)/adherence/.stylelintrc.json

# The browser tests: web/e2e/serve.mjs builds the web app and the binary,
# then serves a throwaway repository (CONTRIBUTING.md, Workflow).
web-e2e: web/node_modules ## Run the browser tests (Playwright, web/e2e)
	cd web && npx playwright test

web-build: web/node_modules ## Build the web app into web/dist
	cd web && npm run build

# v2's web app (next/web), served by git-board-next (ADR 0021): the same
# targets as web's, until it replaces it at the switch.
next/web/node_modules: next/web/package-lock.json
	cd next/web && npm ci
	@touch next/web/node_modules

next-web-dev: ## Run v2's web app's dev server (next/web)
	cd next/web && npm run dev

next-web-check: next/web/node_modules ## Typecheck and test v2's web app (next/web)
	cd next/web && npm run typecheck && npm test

next-web-build: next/web/node_modules ## Build v2's web app into next/web/dist
	cd next/web && npm run build

# The browser tests on git-board-next serve: next/web/e2e/serve.mjs builds
# the web app and the binary, then serves throwaway boards (git, Markdown).
next-web-e2e: next/web/node_modules ## Run v2's browser tests (Playwright, next/web/e2e)
	cd next/web && npx playwright test

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
