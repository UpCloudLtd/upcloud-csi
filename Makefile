NAME=upcloud-csi-plugin
OS ?= linux
PKG ?= github.com/UpCloudLtd/upcloud-csi/cmd/upcloud-csi-plugin
GO_VERSION := 1.17.6

.PHONY: compile
compile:
	@echo "==> Building the project"
	@docker run --rm -e GOOS=${OS} -e GOARCH=amd64 -v ${PWD}/:/app -w /app golang:${GO_VERSION}-alpine sh -c 'apk add git && go build -mod=vendor -o cmd/upcloud-csi-plugin/${NAME} ${PKG}'


.Phony: docker-build
docker-build:
	# TODO add versions and tags -t $(DOCKER_REPO):$(VERSION)
	docker build -t upcloud_csi cmd/upcloud-csi-plugin -f cmd/upcloud-csi-plugin/Dockerfile
