FROM centos:7.5.1804

LABEL maintainers="Lightbits Labs" \
      description="LB CSI Plugin" \
      version.lb-csi.rel="UNKNOWN" \
      version.lb-csi.git="UNKNOWN" \
      version.lb-csi.build.host="UNKNOWN" \
      version.lb-csi.build.time="UNKNOWN" \
      version.lb-csi.build.id="UNKNOWN"
# official builds will also carry the following labels:
#     version.lb-csi.hash
# while custom builds might also have labels:
#     build.host
#     build.time

# at least as of v18.06.1, Docker appears to interpret empty second source
# arg to COPY as "*" (shurely shome mishtake)... hence this silliness:
COPY lb-csi-plugin /

RUN yum install -y e2fsprogs xfsprogs $EXTRA_PACKAGES && \
    yum clean all || true

ENV CSI_ENDPOINT=unix:///csi/csi.sock   \
    LB_CSI_NODE_ID=             \
    LB_CSI_DEFAULT_FS=ext4      \
    LB_CSI_LOG_LEVEL=info       \
    LB_CSI_LOG_ROLE=node        \
    LB_CSI_LOG_TIME=true        \
    LB_CSI_LOG_FMT=text

ENTRYPOINT ["/lb-csi-plugin"]
