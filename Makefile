NAME=upcloud-csi-plugin
OS ?= linux
PKG ?= github.com/UpCloudLtd/upcloud-csi/cmd/upcloud-csi-plugin
GO_VERSION := 1.17.6
ARCH := amd64

.PHONY: compile
compile:
	@echo "==> Building the project"
	@docker run --rm -e GOOS=${OS} -e GOARCH=${ARCH} -v ${PWD}/:/app -w /app golang:${GO_VERSION}-alpine sh -c 'apk add git && go build -mod=vendor -ldflags "-w -s" -o cmd/upcloud-csi-plugin/${NAME} ${PKG}'


.PHONY: docker-build
docker-build:
	# TODO add versions and tags -t $(DOCKER_REPO):$(VERSION)
	docker build --platform linux/x86_64 -t ghcr.io/mescudi21/upcloud-csi:test cmd/upcloud-csi-plugin -f cmd/upcloud-csi-plugin/Dockerfile

.PHONY: deploy-csi
deploy-csi:
	KUBECONFIG=$(KUBECONFIG) kubectl apply -f test/e2e/secret.yaml
	KUBECONFIG=$(KUBECONFIG) kubectl apply -f deploy/kubernetes/beta/driver.yaml

.PHONY: clean-tests
clean-tests:
	KUBECONFIG=$(KUBECONFIG) kubectl delete all --all
	KUBECONFIG=$(KUBECONFIG) kubectl delete persistentvolumeclaims --all

