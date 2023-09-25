PLUGIN_NAME=upcloud-csi-plugin
PLUGIN_PKG ?= github.com/UpCloudLtd/upcloud-csi/cmd/upcloud-csi-plugin
OS ?= linux
GO_VERSION := 1.20
ARCH := amd64
CGO_ENABLED := 1

.PHONY: compile
compile:
	@echo "==> Building the project"
	@docker run --rm -e CGO_ENABLED=${CGO_ENABLED} -e GOOS=${OS} -e GOARCH=${ARCH} -v ${PWD}/:/app -w /app golang:${GO_VERSION}-alpine sh -c \
		'apk add git make build-base && \
		go build -buildvcs=false -ldflags "-w -s" -o cmd/upcloud-csi-plugin/${PLUGIN_NAME} ${PLUGIN_PKG}'


.PHONY: docker-build
docker-build:
	# TODO add versions and tags -t $(DOCKER_REPO):$(VERSION)
	docker build --platform linux/x86_64 -t ghcr.io/upcloudltd/upcloud-csi:main cmd/upcloud-csi-plugin -f cmd/upcloud-csi-plugin/Dockerfile

.PHONY: clean-tests
clean-tests:
	KUBECONFIG=$(KUBECONFIG) kubectl delete all --all
	KUBECONFIG=$(KUBECONFIG) kubectl delete persistentvolumeclaims --all

.PHONY: test
test:
	go test -race ./...

test-integration:
	make -C test/integration test


build-plugin:
	CGO_ENABLED=0 go build -ldflags "-w -s" -o cmd/upcloud-csi-plugin/${PLUGIN_NAME} ${PLUGIN_PKG}

.PHONY: build
build: build-plugin
