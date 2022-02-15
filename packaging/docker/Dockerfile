# SPDX-License-Identifier: GPL-3.0-or-later
# author  : paulfantom

# This image contains preinstalled dependencies
# hadolint ignore=DL3007
FROM netdata/builder:latest as builder

# One of 'nightly' or 'stable'
ARG RELEASE_CHANNEL=nightly

ENV JUDY_VER 1.0.5

ARG CFLAGS

ENV CFLAGS=$CFLAGS

ARG EXTRA_INSTALL_OPTS

ENV EXTRA_INSTALL_OPTS=$EXTRA_INSTALL_OPTS

# Copy source
COPY . /opt/netdata.git
WORKDIR /opt/netdata.git

# Install from source
RUN chmod +x netdata-installer.sh && \
   cp -rp /deps/* /usr/local/ && \
   /bin/echo -e "INSTALL_TYPE='oci'\nPREBUILT_ARCH='$(uname -m)'" > ./system/.install-type && \
   ./netdata-installer.sh --dont-wait --dont-start-it --use-system-protobuf ${EXTRA_INSTALL_OPTS} \
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
    mv /usr/sbin/netdata-claim.sh    /app/usr/sbin/ && \
    mv /usr/sbin/netdatacli    /app/usr/sbin/ && \
    mv packaging/docker/run.sh        /app/usr/sbin/ && \
    mv packaging/docker/health.sh     /app/usr/sbin/ && \
    cp -rp /deps/* /app/usr/local/ && \
    chmod +x /app/usr/sbin/run.sh

#####################################################################
# This image contains preinstalled dependencies
# hadolint ignore=DL3007
FROM netdata/base:latest as base

ARG OFFICIAL_IMAGE=false
ENV NETDATA_OFFICIAL_IMAGE=$OFFICIAL_IMAGE

# Configure system
ARG NETDATA_UID=201
ARG NETDATA_GID=201
ENV DOCKER_GRP netdata
ENV DOCKER_USR netdata
# If DISABLE_TELEMETRY is set, it will disable anonymous stats collection and reporting
#ENV DISABLE_TELEMETRY=1

# Copy files over
RUN mkdir -p /opt/src /var/log/netdata && \
    # Link log files to stdout
    ln -sf /dev/stdout /var/log/netdata/access.log && \
    ln -sf /dev/stdout /var/log/netdata/debug.log && \
    ln -sf /dev/stderr /var/log/netdata/error.log && \
    # fping from alpine apk is on a different location. Moving it.
    ln -snf /usr/sbin/fping /usr/local/bin/fping && \
    chmod 4755 /usr/local/bin/fping && \
    # Add netdata user
    addgroup -g ${NETDATA_GID} -S "${DOCKER_GRP}" && \
    adduser -S -H -s /usr/sbin/nologin -u ${NETDATA_GID} -h /etc/netdata -G "${DOCKER_GRP}" "${DOCKER_USR}"

# Long-term this should leverage BuildKit’s mount option.
COPY --from=builder /wheels /wheels
COPY --from=builder /app /

# Apply the permissions as described in
# https://docs.netdata.cloud/docs/netdata-security/#netdata-directories, but own everything by root group due to https://github.com/netdata/netdata/pull/6543
# hadolint ignore=DL3013
RUN chown -R root:root \
        /etc/netdata \
        /usr/share/netdata \
        /usr/libexec/netdata && \
    chown -R netdata:root \
        /usr/lib/netdata \
        /var/cache/netdata \
        /var/lib/netdata \
        /var/log/netdata && \
    chown -R netdata:netdata /var/lib/netdata/cloud.d && \
    chmod 0700 /var/lib/netdata/cloud.d && \
    chmod 0755 /usr/libexec/netdata/plugins.d/*.plugin && \
    chmod 4755 \
        /usr/libexec/netdata/plugins.d/cgroup-network \
        /usr/libexec/netdata/plugins.d/apps.plugin && \
    if [ -f /usr/libexec/netdata/plugins.d/freeipmi.plugin ]; then \
        chmod 4755 /usr/libexec/netdata/plugins.d/freeipmi.plugin; \
    fi && \
    # Group write permissions due to: https://github.com/netdata/netdata/pull/6543
    find /var/lib/netdata /var/cache/netdata -type d -exec chmod 0770 {} \; && \
    find /var/lib/netdata /var/cache/netdata -type f -exec chmod 0660 {} \; && \
    pip --no-cache-dir install /wheels/* && \
    rm -rf /wheels

ENV NETDATA_LISTENER_PORT 19999
EXPOSE $NETDATA_LISTENER_PORT

ENTRYPOINT ["/usr/sbin/run.sh"]

HEALTHCHECK --interval=60s --timeout=10s --retries=3 CMD /usr/sbin/health.sh

ONBUILD ENV NETDATA_OFFICIAL_IMAGE=false
