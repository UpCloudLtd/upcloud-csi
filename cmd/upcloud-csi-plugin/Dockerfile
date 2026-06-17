FROM alpine:3.23

RUN apk add ca-certificates \
    e2fsprogs \
    e2fsprogs-extra \
    xfsprogs \
    xfsprogs-extra \
    eudev \
    findmnt \
    blkid \
    util-linux \
    parted \
    sgdisk \
    cloud-utils-growpart

ADD upcloud-csi-plugin /bin/

ENTRYPOINT ["/bin/upcloud-csi-plugin"]
