FROM alpine:3.14

LABEL maintainers="Lightbits Labs" \
      description="LB CSI Plugin" \
      version.lb-csi.rel="UNKNOWN" \
      version.lb-csi.git="UNKNOWN" \
      version.lb-csi.build.host="UNKNOWN" \
      version.lb-csi.build.time="UNKNOWN" \
      version.lb-csi.build.id="UNKNOWN" \
      dummy_label="check a new label is added to docker image"
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

RUN apk add --no-cache \
    e2fsprogs e2fsprogs-extra \
    xfsprogs xfsprogs-extra \
    blkid \
    $EXTRA_PACKAGES

COPY licenses /licenses

COPY lb-csi-plugin /

ENTRYPOINT ["/lb-csi-plugin"]
