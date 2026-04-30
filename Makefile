ifndef MAKE_DEBUG
	MAKEFLAGS += -s
endif

# Setting CGO_ENABLED to 0 disables CGO (cf. https://pkg.go.dev/cmd/cgo)
CGO_ENABLED := 0

GIT_TAG ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "unknown")

BIN_DIR ?= $(shell pwd)/bin

PROJECT_OWNER ?= warmans
PROJECT_NAME ?= aggregate-exporter
DOCKER_NAME ?= $(PROJECT_OWNER)/$(PROJECT_NAME)

LOCAL_BIN := "$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))/.env/bin"

.PHONY: install.linter
install.linter:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(LOCAL_BIN) v1.62.0

.PHONY: lint.go
lint:
	$(LOCAL_BIN)/golangci-lint run

.PHONY: build
build:
	echo ">> building linux binary"
	CGO_ENABLED=$(CGO_ENABLED) go build -o ${BIN_DIR}/prometheus-aggregate-exporter -ldflags "-X main.Version=${GIT_TAG}" ./cmd

.PHONY: build-arch
build-arch:
ifndef GOOS
	echo "GOOS must be defined"; exit 1;
endif
ifndef GOARCH
	echo "GOARCH must be defined"; exit 1;
endif
	echo ">> building $(GOOS) $(GOARCH) binary"
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build -o ${BIN_DIR}/prometheus-aggregate-exporter-$(GOOS)-$(GOARCH) -ldflags "-X main.Version=${GIT_TAG}" ./cmd


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

.PHONY: test.fetch
test.fetch:
	curl localhost:8080/metrics

test.unregister:
	curl -X POST -H "Content-Type: application/x-www-form-urlencoded" -d "name=t1&address=localhost:3000/histogram.txt" localhost:8080/unregister

test.register:
	curl -X POST -H "Content-Type: application/x-www-form-urlencoded" -d "name=t1&address=localhost:3000/histogram.txt" localhost:8080/register

.PHONY: test.auth
test.auth: build
	mkdir -p .build
	echo "test-password-123" > .build/test-auth-password.txt
	echo "test-bearer-token-123" > .build/test-auth-token.txt
	cd fixture/auth; go run serve.go > ../../.build/test-auth-fixture.log 2>&1 & echo $$! > ../../.build/test-auth-fixture.pid
	sleep 1
	./bin/prometheus-aggregate-exporter \
	-targets="bad=http://prometheus_scraper:test-password-123@localhost:3011/histogram.txt" \
	-server.bind=":18080" \
	-verbose=true > .build/test-auth-failfast.log 2>&1 & echo $$! > .build/test-auth-failfast.pid
	sleep 1
	curl -s localhost:18080/metrics > .build/test-auth-failfast.metrics
	awk '/credentials in target URL are not allowed/ { found=1 } END { exit(found ? 0 : 1) }' .build/test-auth-failfast.log
	kill $$(cat .build/test-auth-failfast.pid) 2>/dev/null || true
	./bin/prometheus-aggregate-exporter \
	-targets="secure=http://localhost:3011/histogram.txt" \
	-server.bind=":18081" \
	-targets.auth.username="prometheus_scraper" \
	-targets.auth.password_file=".build/test-auth-password.txt" \
	-verbose=true > .build/test-auth-success.log 2>&1 & echo $$! > .build/test-auth-success.pid
	sleep 1
	curl -s localhost:18081/metrics > .build/test-auth-success.metrics
	awk '/^http_requests_total\{.*ae_source="secure"/ { found=1 } END { exit(found ? 0 : 1) }' .build/test-auth-success.metrics
	kill $$(cat .build/test-auth-success.pid) 2>/dev/null || true
	./bin/prometheus-aggregate-exporter \
	-targets="secure=http://localhost:3011/histogram.txt" \
	-server.bind=":18082" \
	-targets.auth.type="bearer" \
	-targets.auth.token_file=".build/test-auth-token.txt" \
	-verbose=true > .build/test-auth-bearer.log 2>&1 & echo $$! > .build/test-auth-bearer.pid
	sleep 1
	curl -s localhost:18082/metrics > .build/test-auth-bearer.metrics
	awk '/^http_requests_total\{.*ae_source="secure"/ { found=1 } END { exit(found ? 0 : 1) }' .build/test-auth-bearer.metrics
	kill $$(cat .build/test-auth-bearer.pid) 2>/dev/null || true
	kill $$(cat .build/test-auth-fixture.pid) 2>/dev/null || true

# Packaging
#-----------------------------------------------------------------------

.PHONY: docker-build
docker-build:
	docker build --build-arg GIT_TAG=$(GIT_TAG) -t $(DOCKER_NAME):$(GIT_TAG) -t $(DOCKER_NAME):latest . && echo ">> built $(DOCKER_NAME):$(GIT_TAG) and $(DOCKER_NAME):latest"

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
