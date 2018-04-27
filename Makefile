GO    := GO15VENDOREXPERIMENT=1 go
PROMU ?= $(GOPATH)/bin/promu

PREFIX  ?= $(shell pwd)
BIN_DIR ?= $(shell pwd)

PROJECT_OWNER=warmans
PROJECT_NAME=aggregate-exporter
PROJECT_VERSION=1.0.0
DOCKER_NAME=$(PROJECT_OWNER)/$(PROJECT_NAME)

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

# Packaging
#-----------------------------------------------------------------------

.PHONY: docker-build
docker-build:
	docker build -t $(DOCKER_NAME):$(PROJECT_VERSION) -t $(DOCKER_NAME):latest .

.PHONY: docker-publish
docker-publish:
	docker push $(DOCKER_NAME):$(PROJECT_VERSION)

docker-run:
	cd test; docker-compose up