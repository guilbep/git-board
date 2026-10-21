.PHONY: build test lint fmt vet ci install install-latest web-dev web-build dist clean

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

lint: vet fmt

# Everything CI/a pre-push hook should run — the same sequence used to vet
# every change in this repo before it's committed.
ci: build vet test
	@test -z "$$(gofmt -l .)" || (echo "gofmt needs to be run on:"; gofmt -l .; exit 1)

install:
	go install ./cmd/git-board

# The latest GitHub release's git-board for this machine, web app inside,
# checked against its checksums.txt and put where go install puts it.
install-latest:
	@set -e; d=$$(mktemp -d); trap 'rm -rf "$$d"' EXIT; \
	gh release download -D "$$d" -p "*_$$(go env GOOS)_$$(go env GOARCH).tar.gz" -p checksums.txt; \
	(cd "$$d" && shasum -a 256 -c checksums.txt --ignore-missing && tar -xzf git-board_*.tar.gz); \
	bin=$$(go env GOBIN); bin=$${bin:-$$(go env GOPATH)/bin}; \
	rm -f "$$bin/git-board" && cp "$$d"/git-board_*/git-board "$$bin/" && "$$bin/git-board" --version

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

# A release's archives, the web app inside, in dist/ with checksums.txt:
# run on a clean checkout of the tag, after `cd web && npm ci`, so the
# commands report that version.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
dist: web-build
	rm -rf dist && mkdir dist
	@set -e; v=$$(git describe --tags --always --dirty); for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; d=git-board_$${v}_$${os}_$${arch}; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "-s -w" -o dist/$$d/ ./cmd/git-board; \
		cp LICENSE README.md dist/$$d/; \
		if [ $$os = windows ]; then (cd dist && zip -qr $$d.zip $$d); else tar -C dist -czf dist/$$d.tar.gz $$d; fi; \
		rm -rf dist/$$d; echo "dist/$$d"; \
	done; cd dist && shasum -a 256 * > checksums.txt

clean:
	rm -rf site/
