# SPDX-License-Identifier: GPL-3.0-or-later

# This image contains preinstalled dependencies
# hadolint ignore=DL3007
FROM netdata/builder:v3 AS builder

# One of 'nightly' or 'stable'
ARG RELEASE_CHANNEL=nightly

ARG CFLAGS

ENV CFLAGS=$CFLAGS

ARG EXTRA_INSTALL_OPTS

ENV EXTRA_INSTALL_OPTS=$EXTRA_INSTALL_OPTS

ARG DEBUG_BUILD

ENV DEBUG_BUILD=$DEBUG_BUILD

ARG BUILD_ARCH

ENV BUILD_ARCH=$BUILD_ARCH

# Copy source
COPY --exclude='**/.git' \
     --exclude='**/Dockerfile' \
     . /opt/netdata.git
WORKDIR /opt/netdata.git

# Install from source
RUN chmod +x netdata-installer.sh && \
   cp -rp /deps/* /usr/local/ && \
   BUILD_ARCH="${BUILD_ARCH:-"$(uname -m)"}" && \
   /bin/echo -e "INSTALL_TYPE='oci'\nPREBUILT_ARCH='${BUILD_ARCH}'" > ./system/.install-type && \
   CFLAGS="$(packaging/docker/gen-cflags.sh)" LDFLAGS="-Wl,--gc-sections" ./netdata-installer.sh --dont-wait --dont-start-it \
   --use-system-protobuf \
   --disable-ebpf \
   --enable-plugin-otel \
   --enable-plugin-otel-signal-viewer \
   --internal-systemd-journal \
   ${EXTRA_INSTALL_OPTS} \
   --install-no-prefix / \
   "$([ "$RELEASE_CHANNEL" = stable ] && echo --stable-channel)"

# files to one directory
RUN mkdir -p /app/usr/sbin/ \
             /app/usr/share \
             /app/usr/libexec \
             /app/usr/local \
             /app/usr/lib \
             /app/var/cache \
             /app/var/lib \
             /app/etc && \
    mv /usr/share/netdata   /app/usr/share/ && \
    mv /usr/libexec/netdata /app/usr/libexec/ && \
    mv /usr/lib/netdata     /app/usr/lib/ && \
    mv /var/cache/netdata   /app/var/cache/ && \
    mv /var/lib/netdata     /app/var/lib/ && \
    mv /etc/netdata         /app/etc/ && \
    mv /usr/sbin/netdata    /app/usr/sbin/ && \
    mv /usr/sbin/netdatacli    /app/usr/sbin/ && \
    mv /usr/sbin/nd-run    /app/usr/sbin/ && \
    mv /usr/sbin/systemd-cat-native /app/usr/sbin/ && \
    mv packaging/docker/run.sh        /app/usr/sbin/ && \
    mv packaging/docker/health.sh     /app/usr/sbin/ && \
    mkdir -p /deps/etc && \
    cp -rp /deps/etc /app/usr/local/etc && \
    chmod -R o+rX /app && \
    chmod +x /app/usr/sbin/run.sh && \
    # Apply the permissions per https://learn.netdata.cloud/docs/collecting-metrics#file-permissions-and-ownership
    # TODO: "5" is used for the "others" bit, not consistent with the documentation
    chmod 0755 /app/usr/libexec/netdata/plugins.d/*.plugin && \
    for name in cgroup-network \
                local-listeners \
                apps.plugin \
                debugfs.plugin \
                freeipmi.plugin \
                go.d.plugin \
                perf.plugin \
                ndsudo \
                slabinfo.plugin \
                network-viewer.plugin \
                otel-plugin \
                otel-signal-viewer-plugin \
                systemd-journal.plugin; do \
        [ -f "/app/usr/libexec/netdata/plugins.d/$name" ] && chmod 4755 "/app/usr/libexec/netdata/plugins.d/$name"; \
    done && \
    # Group write permissions due to: https://github.com/netdata/netdata/pull/6543
    find /app/var/lib/netdata /app/var/cache/netdata -type d -exec chmod 0770 {} \; && \
    find /app/var/lib/netdata /app/var/cache/netdata -type f -exec chmod 0660 {} \; && \
    chmod 0700 /app/var/lib/netdata/cloud.d

#####################################################################
# This image contains preinstalled dependencies
# hadolint ignore=DL3007
FROM netdata/base:v3 AS base

ARG BUILD_DATE
ARG BUILD_VERSION
LABEL org.opencontainers.image.authors="Netdatabot <bot@netdata.cloud>"
LABEL org.opencontainers.image.url="https://netdata.cloud"
LABEL org.opencontainers.image.documentation="https://learn.netdata.cloud"
LABEL org.opencontainers.image.source="https://github.com/netdata/netdata"
LABEL org.opencontainers.image.title="Netdata Agent"
LABEL org.opencontainers.image.description="Official Netdata Agent Docker Image"
LABEL org.opencontainers.image.vendor="Netdata Inc."
LABEL org.opencontainers.image.created=${BUILD_DATE}
LABEL org.opencontainers.image.version=${BUILD_VERSION}

ARG OFFICIAL_IMAGE=false
ENV NETDATA_OFFICIAL_IMAGE=$OFFICIAL_IMAGE

ONBUILD ENV NETDATA_OFFICIAL_IMAGE=false

ARG NETDATA_UID=201
ARG NETDATA_GID=201
ENV DOCKER_GRP=netdata
ENV DOCKER_USR=netdata
# If DISABLE_TELEMETRY is set, it will disable anonymous stats collection and reporting
#ENV DISABLE_TELEMETRY=1
ENV NETDATA_LISTENER_PORT=19999
EXPOSE $NETDATA_LISTENER_PORT

ENV NETDATA_EXTRA_DEB_PACKAGES=""

RUN mkdir -p /opt/src /var/log/netdata && \
    ln -sf /dev/stdout /var/log/netdata/access.log && \
    ln -sf /dev/stdout /var/log/netdata/aclk.log && \
    ln -sf /dev/stdout /var/log/netdata/debug.log && \
    ln -sf /dev/stderr /var/log/netdata/error.log && \
    ln -sf /dev/stderr /var/log/netdata/daemon.log && \
    ln -sf /dev/stdout /var/log/netdata/collector.log && \
    ln -sf /dev/stdout /var/log/netdata/health.log && \
    chown -R ${NETDATA_UID}:0 /var/log/netdata

# Using chmod/chown command with a "RUN" instruction to set the permission/ownership
# would bloat the image significantly, so we set the permission bits in the builder stage
# and then copy the files with the correct ownership.
# Own everything by root group due to https://github.com/netdata/netdata/pull/6543
COPY --from=builder --chown=0:0 --parents \
    /app/./usr/sbin/ \
    /app/./usr/share/netdata \
    /app/./usr/libexec/netdata \
    /app/./usr/local/etc \
    /app/./etc/netdata \
    /
COPY --from=builder --chown=${NETDATA_UID}:0 --parents \
    /app/./usr/lib/netdata \
    /app/./var/cache/netdata \
    /app/./var/lib/netdata \
    /

# Create netdata user
# hadolint ignore=DL3013
RUN addgroup --gid ${NETDATA_GID} --system "${DOCKER_GRP}" && \
    adduser --system --no-create-home --shell /usr/sbin/nologin --uid ${NETDATA_UID} --home /etc/netdata --group "${DOCKER_USR}" && \
    chown -R ${NETDATA_UID}:${NETDATA_GID} /var/lib/netdata/cloud.d && \
    for name in cgroup-network \
                local-listeners \
                apps.plugin \
                debugfs.plugin \
                freeipmi.plugin \
                go.d.plugin \
                perf.plugin \
                ndsudo \
                slabinfo.plugin \
                network-viewer.plugin \
                otel-plugin \
                otel-signal-viewer-plugin \
                systemd-journal.plugin; do \
        [ -f "/usr/libexec/netdata/plugins.d/$name" ] && test $(stat -c %a "/usr/libexec/netdata/plugins.d/$name") = "4755"; \
    done && \
    cp -va /etc/netdata /etc/netdata.stock

ENTRYPOINT ["/usr/sbin/run.sh"]

HEALTHCHECK --interval=60s --timeout=10s --retries=3 CMD /usr/sbin/health.sh
