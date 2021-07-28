ifndef MAKE_DEBUG
    MAKEFLAGS += -s
endif

GO    := go
PROMU ?= $(GOPATH)/bin/promu

GIT_TAG := $(shell git describe --tags --exact-match 2>/dev/null || echo "unknown")

BIN_DIR ?= $(shell pwd)/bin

PROJECT_OWNER ?= warmans
PROJECT_NAME ?= aggregate-exporter
DOCKER_NAME ?= $(PROJECT_OWNER)/$(PROJECT_NAME)

.PHONY: build
build:
	echo ">> building binaries"
	go build -o ${BIN_DIR}/prometheus-aggregate-exporter -ldflags "-X main.Version=${GIT_TAG}" ./cmd


# Manual Testing
#----------------------------------------------------------------------
.PHONY: test.run-fixture-server
test.run-fixture-server:
	cd fixture; go run serve.go

.PHONY: test.run
test.run: build
	./bin/prometheus-aggregate-exporter \
	-targets="t1=http://localhost:3000/histogram.txt,t2=http://localhost:3000/histogram-2.txt" \
	-server.bind=":8080" \
	-verbose=true \
	-targets.dynamic.registration=true \
	-targets.cache.path=".cache"

.PHONY: test.scrape
test.scrape:
	curl localhost:8080/metrics

test.unregister:
	curl -X POST -H "Content-Type: application/x-www-form-urlencoded" -d "name=t1&address=localhost:3000/histogram.txt" localhost:8080/unregister

test.register:
	curl -X POST -H "Content-Type: application/x-www-form-urlencoded" -d "name=t1&address=localhost:3000/histogram.txt" localhost:8080/register

# Packaging
#-----------------------------------------------------------------------

.PHONY: docker-build
docker-build:
	docker build -t $(DOCKER_NAME):$(GIT_TAG) -t $(DOCKER_NAME):latest . && echo ">> built $(DOCKER_NAME):$(GIT_TAG) and $(DOCKER_NAME):latest"

.PHONY: buildah-f30
buildah-f30:
	buildah build-using-dockerfile -t $(DOCKER_NAME)-f30:$(GIT_TAG) -t $(DOCKER_NAME)-f30:latest -f Dockerfile.f30-mini . && echo ">> built $(DOCKER_NAME):$(GIT_TAG) and $(DOCKER_NAME):latest"

.PHONY: docker-publish
docker-publish:
	docker push $(DOCKER_NAME):$(GIT_TAG) && echo ">> published $(DOCKER_NAME):$(GIT_TAG)"

.PHONY: buildah-publish
buildah-publish:
	buildah push $(DOCKER_NAME)-f30:$(GIT_TAG) docker://quay.io/$(DOCKER_NAME)-f30:$(GIT_TAG) && echo ">> published $(DOCKER_NAME):$(GIT_TAG)"

docker-run:
	docker run -it
