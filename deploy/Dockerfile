# update alpine to get cryptsetup 2.4.x
FROM alpine:3.19

LABEL org.opencontainers.image.title="Lightbits CSI Plugin" \
    org.opencontainers.image.description="CSI plugin for Lightbits Cluster" \
    org.opencontainers.image.authors="Lightbits Labs <support@lightbitslabs.com>" \
    org.opencontainers.image.documentation="https://www.lightbitslabs.com/support/" \
    org.opencontainers.image.version="UNKNOWN" \
    org.opencontainers.image.revision="UNKNOWN" \
    org.opencontainers.image.created="UNKNOWN"


# official builds will also carry the following labels:
#     version.lb-csi.hash
# while custom builds might also have labels:
#     build.host
#     build.time

ENV CSI_ENDPOINT=unix:///csi/csi.sock   \
    LB_CSI_NODE_ID=             \
    LB_CSI_DEFAULT_FS=ext4      \
    LB_CSI_LOG_LEVEL=info       \
    LB_CSI_LOG_ROLE=node        \
    LB_CSI_LOG_TIME=true        \
    LB_CSI_LOG_FMT=text

RUN apk add --no-cache --repository=http://dl-cdn.alpinelinux.org/alpine/v3.14/main \
    cryptsetup \
    e2fsprogs e2fsprogs-extra \
    xfsprogs=5.12.0-r0 \
    xfsprogs-extra=5.12.0-r0 \
    lsblk \
    blkid \
    kmod \
    $EXTRA_PACKAGES

COPY licenses /licenses
COPY lb-csi-plugin /

ENTRYPOINT ["/lb-csi-plugin"]
