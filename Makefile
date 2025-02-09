OUTDIR ?= $(CURDIR)/out

PKG=github.com/welteki/zvol-snapshotter
VERSION=$(shell git describe --match 'v[0-9]*' --dirty='.m' --always --tags)
REVISION=$(shell git rev-parse HEAD)$(shell if ! git diff --no-ext-diff --quiet --exit-code; then echo .m; fi)
GO_BUILD_LDFLAGS ?= -s -w
GO_LD_FLAGS=-ldflags '$(GO_BUILD_LDFLAGS) -X $(PKG)/version.Version=$(VERSION) -X $(PKG)/version.Revision=$(REVISION) $(GO_EXTRA_LDFLAGS)'

build: containerd-zvol-snapshotter

.PHONY: containerd-zvol-snapshotter
containerd-zvol-snapshotter:
	cd cmd/ ; GO111MODULE=auto GOARCH=amd64 go build -o $(OUTDIR)/$@-amd64 $(GO_LD_FLAGS) -v .
	cd cmd/ ; GO111MODULE=auto GOARCH=arm64 go build -o $(OUTDIR)/$@-arm64 $(GO_LD_FLAGS) -v .

.PHONY: clean
clean:
	@echo "$@"
	@rm -rf $(OUTDIR)
