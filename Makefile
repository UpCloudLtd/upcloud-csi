PLUGIN_NAME=upcloud-csi-plugin
PLUGIN_PKG ?= github.com/UpCloudLtd/upcloud-csi/cmd/upcloud-csi-plugin
MANIFEST_NAME=upcloud-csi-manifest
MANIFEST_PKG ?= github.com/UpCloudLtd/upcloud-csi/cmd/upcloud-csi-manifest
OS ?= linux
GO_VERSION := 1.19
ARCH := amd64
CGO_ENABLED := 1
MANIFEST_ZONES := de-fra1 fi-hel2 sg-sin1 uk-lon1 nl-ams1 us-chi1

.PHONY: compile
compile:
	@echo "==> Building the project"
	@docker run --rm -e CGO_ENABLED=${CGO_ENABLED} -e GOOS=${OS} -e GOARCH=${ARCH} -v ${PWD}/:/app -w /app golang:${GO_VERSION}-alpine sh -c \
		'apk add git && \
		go build -buildvcs=false -ldflags "-w -s" -o cmd/upcloud-csi-plugin/${PLUGIN_NAME} ${PLUGIN_PKG} && \
		go build -buildvcs=false -ldflags "-w -s" -o cmd/upcloud-csi-manifest/${MANIFEST_NAME} ${MANIFEST_PKG}'


.PHONY: docker-build
docker-build:
	# TODO add versions and tags -t $(DOCKER_REPO):$(VERSION)
	docker build --platform linux/x86_64 -t ghcr.io/upcloudltd/upcloud-csi:main cmd/upcloud-csi-plugin -f cmd/upcloud-csi-plugin/Dockerfile

.PHONY: deploy-csi
deploy-csi:
	KUBECONFIG=$(KUBECONFIG) kubectl apply -f test/e2e/secret.yaml
	KUBECONFIG=$(KUBECONFIG) kubectl apply -f deploy/kubernetes/beta/driver.yaml

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

build-manifest:
	CGO_ENABLED=0 go build -ldflags "-w -s" -o cmd/upcloud-csi-manifest/${MANIFEST_NAME} ${MANIFEST_PKG}

.PHONY: build
build: build-plugin build-manifest

render-manifests: build-manifest
	test $${PROJECT_NAME?PROJECT_NAME not set}
	test $${VERSION?VERSION not set}
	mkdir -p ./temp
	for zone in ${MANIFEST_ZONES}; do \
		./cmd/upcloud-csi-manifest/upcloud-csi-manifest \
			--crd=false \
			--rbac=true \
			--setup=true \
			--zone=$${zone} \
			--output=./temp/${PROJECT_NAME}-setup-$${zone}.yaml \
			--driver-version=${VERSION} ; \
	done