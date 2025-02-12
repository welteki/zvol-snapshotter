# Base path used to install.
CMD_DESTDIR ?= /usr/local/bin
OUTDIR ?= $(CURDIR)/out

PKG=github.com/welteki/zvol-snapshotter
VERSION=$(shell git describe --match 'v[0-9]*' --dirty='.dirty' --always --tags)
REVISION=$(shell git rev-parse HEAD)$(shell if ! git diff --no-ext-diff --quiet --exit-code; then echo .dirty; fi)
GO111MODULE_VALUE=auto
GO_BUILD_LDFLAGS ?= -s -w
GO_LD_FLAGS=-ldflags '$(GO_BUILD_LDFLAGS) -X $(PKG)/version.Version=$(VERSION) -X $(PKG)/version.Revision=$(REVISION) $(GO_EXTRA_LDFLAGS)'

CMD = containerd-zvol-grpc
CMD_BINARIES=$(addprefix $(OUTDIR)/,$(CMD))

ZVOL_SNAPSHOTTER_PROJECT_ROOT ?= $(shell pwd)

all: build

build: $(CMD)

.PHONY: containerd-zvol-grpc
containerd-zvol-grpc:
	cd cmd/ ; GO111MODULE=$(GO111MODULE_VALUE) go build -o $(OUTDIR)/$@ $(GO_LD_FLAGS) -v .

.PHONY: install
install:
	@echo "$@"
	@mkdir -p $(CMD_DESTDIR)
	@install $(CMD_BINARIES) $(CMD_DESTDIR)

.PHONY: uninstall
uninstall:
	@echo "$@"
	@rm -f $(addprefix $(CMD_DESTDIR)/,$(notdir $(CMD_BINARIES)))

.PHONY: clean
clean:
	@echo "$@"
	@rm -rf $(OUTDIR)/*

.PHONY: test
test:
	@echo "$@"
	@GO111MODULE=$(GO111MODULE_VALUE) go test -race ./...

.PHONY: release
release:
	@echo "$@"
	@$(ZVOL_SNAPSHOTTER_PROJECT_ROOT)/scripts/create-release.sh $(RELEASE_TAG)