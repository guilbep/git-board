.PHONY: build test lint fmt vet ci install web-dev web-build clean

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
	go install ./cmd/git-board ./cmd/git-board-server

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

clean:
	rm -rf site/
