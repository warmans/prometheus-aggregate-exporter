ifndef MAKE_DEBUG
    MAKEFLAGS += -s
endif

GO    := go
PROMU ?= $(GOPATH)/bin/promu

GIT_TAG := $(shell git describe --tags --exact-match 2>/dev/null || echo "unknown")

BUILD_PREFIX  ?= $(shell pwd)/bin
BIN_DIR ?= $(shell pwd)

PROJECT_OWNER ?= warmans
PROJECT_NAME ?= aggregate-exporter
DOCKER_NAME ?= $(PROJECT_OWNER)/$(PROJECT_NAME)

.PHONY: check
check:
	which promu 1> /dev/null || go install github.com/prometheus/promu
	echo "${GIT_TAG}" > VERSION

.PHONY: build
build: check
	echo ">> building binaries"
	$(PROMU) build --prefix $(BUILD_PREFIX)

.PHONY: crossbuild
crossbuild: check
	echo ">> crossbuilding binaries"
	$(PROMU) crossbuild
	$(PROMU) crossbuild tarballs

.PHONY: release
release:
ifndef GITLAB_TOKEN
	$(error GITLAB_TOKEN is not set)
endif
	#requires GITHUB_TOKEN environment variable and a tag already pushed to github
	$(PROMU) release

# Packaging
#-----------------------------------------------------------------------

.PHONY: docker-build
docker-build:
	docker build -t $(DOCKER_NAME):$(GIT_TAG) -t $(DOCKER_NAME):latest . && echo ">> built $(DOCKER_NAME):$(GIT_TAG) and $(DOCKER_NAME):latest"

.PHONY: docker-publish
docker-publish:
	docker push $(DOCKER_NAME):$(GIT_TAG) && echo ">> published $(DOCKER_NAME):$(GIT_TAG)"

docker-run:
	docker run -it