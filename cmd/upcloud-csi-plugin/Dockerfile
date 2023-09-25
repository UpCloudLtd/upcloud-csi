FROM alpine:3.17

RUN apk add ca-certificates \
    e2fsprogs \
    eudev \
    findmnt \
    xfsprogs \
    blkid \
    e2fsprogs-extra \
    util-linux \
    parted

ADD upcloud-csi-plugin /bin/

ENTRYPOINT ["/bin/upcloud-csi-plugin"]
