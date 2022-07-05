NAME=upcloud-csi-plugin
OS ?= linux
PKG ?= github.com/UpCloudLtd/upcloud-csi/cmd/upcloud-csi-plugin
ARCH := amd64

.PHONY: test
test:
	go test ./...

.PHONY: build
build:
	go build -mod=vendor -ldflags "-w -s" -o ${NAME} ${PKG}

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

