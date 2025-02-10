OUTDIR ?= $(CURDIR)/out

PKG=github.com/welteki/zvol-snapshotter
VERSION=$(shell git describe --match 'v[0-9]*' --dirty='.dirty' --always --tags)
REVISION=$(shell git rev-parse HEAD)$(shell if ! git diff --no-ext-diff --quiet --exit-code; then echo .dirty; fi)
GO111MODULE_VALUE=auto
GO_BUILD_LDFLAGS ?= -s -w
GO_LD_FLAGS=-ldflags '$(GO_BUILD_LDFLAGS) -X $(PKG)/version.Version=$(VERSION) -X $(PKG)/version.Revision=$(REVISION) $(GO_EXTRA_LDFLAGS)'

CMD = containerd-zvol-grpc
build: $(CMD)

.PHONY: containerd-zvol-grpc
containerd-zvol-grpc:
	cd cmd/ ; GO111MODULE=$(GO111MODULE_VALUE) GOARCH=amd64 go build -o $(OUTDIR)/$@-amd64 $(GO_LD_FLAGS) -v .
	cd cmd/ ; GO111MODULE=$(GO111MODULE_VALUE) GOARCH=arm64 go build -o $(OUTDIR)/$@-arm64 $(GO_LD_FLAGS) -v .

.PHONY: clean
clean:
	@echo "$@"
	@rm -rf $(OUTDIR)/

.PHONY: test
test:
	@echo "$@"
	@GO111MODULE=$(GO111MODULE_VALUE) go test -race ./...qq