# Step 1 build
FROM golang:alpine AS build

RUN apk add ca-certificates \
    e2fsprogs \
    eudev \
    findmnt \
    xfsprogs \
    blkid \
    e2fsprogs-extra \
    util-linux

ADD upcloud-csi-plugin /bin/

ENTRYPOINT ["/bin/upcloud-csi-plugin"]