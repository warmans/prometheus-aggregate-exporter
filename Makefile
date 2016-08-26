GO    := GO15VENDOREXPERIMENT=1 go
PROMU ?= $(GOPATH)/bin/promu

PREFIX  ?= $(shell pwd)
BIN_DIR ?= $(shell pwd)

.PHONY: prerequisites
prerequisites:
	$(GO) get -u github.com/prometheus/promu

.PHONY: build
build: prerequisites
	@echo ">> building binaries"
	$(PROMU) build --prefix $(PREFIX)

.PHONY: crossbuild
crossbuild: prerequisites
	@echo ">> crossbuilding binaries"
	$(PROMU) crossbuild
	$(PROMU) crossbuild tarballs

.PHONY: release
release:
	#requires GITHUB_TOKEN environment variable with valid token and an
	#already created release on github
	$(PROMU) release