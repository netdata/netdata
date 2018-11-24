# SPDX-License-Identifier: GPL-3.0-or-later
# author  : paulfantom

# Cross-arch building is achieved by specifying ARCH as a build parameter with `--build-arg` option.
# It is automated in `build.sh` script
ARG ARCH=amd64-v3.8
FROM multiarch/alpine:${ARCH} as builder

# Install prerequisites
RUN apk --no-cache add alpine-sdk \
                       autoconf \
                       automake \
                       bash \
                       build-base \
                       curl \
                       jq \
                       libmnl-dev \
                       libuuid \
                       lm_sensors \
                       netcat-openbsd \
                       nodejs \
                       pkgconfig \
                       py-mysqldb \
                       py-psycopg2 \
                       py-yaml \
                       python \
                       util-linux-dev \
                       zlib-dev

# Copy source
COPY . /opt/netdata.git
WORKDIR /opt/netdata.git

# Install from source
RUN chmod +x netdata-installer.sh && \
    sync && sleep 1 && \
    ./netdata-installer.sh --dont-wait --dont-start-it

# files to one directory
RUN mkdir -p /app/usr/sbin/ \
             /app/usr/share \
             /app/usr/libexec \
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
    mv docker/run.sh        /app/usr/sbin/ && \
    chmod +x /app/usr/sbin/run.sh

#####################################################################
ARG ARCH
FROM multiarch/alpine:${ARCH}

# Configure system
ARG NETDATA_UID=201
ARG NETDATA_GID=201

# Reinstall some prerequisites
RUN apk --no-cache add curl \
                       fping \
                       jq \
                       libuuid \
                       lm_sensors \
                       netcat-openbsd \
                       nodejs \
                       py-mysqldb \
                       py-psycopg2 \
                       py-yaml \
                       python

# fping from alpine apk is on a different location. Moving it.
RUN mv /usr/sbin/fping /usr/local/bin/fping && \
    chmod 4755 /usr/local/bin/fping && \
    mkdir -p /var/log/netdata && \
    # Add netdata user
    addgroup -g ${NETDATA_GID} -S netdata && \
    adduser -S -H -s /usr/sbin/nologin -u ${NETDATA_GID} -h /etc/netdata -G netdata netdata

# Copy files over
COPY --from=builder /app /

RUN \
    # Apply permissions as described in
    # https://github.com/netdata/netdata/tree/master/doc/netdata-security.md#netdata-directories
    chown -R root:netdata /etc/netdata && \
    chown -R netdata:netdata /var/cache/netdata /var/lib/netdata /usr/share/netdata && \
    chown -R root:netdata /usr/lib/netdata && \
    chown -R root:netdata /usr/libexec/netdata/plugins.d/apps.plugin /usr/libexec/netdata/plugins.d/cgroup-network && \
    chmod 4750 /usr/libexec/netdata/plugins.d/cgroup-network /usr/libexec/netdata/plugins.d/apps.plugin && \
    chmod 0750 /var/lib/netdata /var/cache/netdata && \
    # Link log files to stdout
    ln -sf /dev/stdout /var/log/netdata/access.log && \
    ln -sf /dev/stdout /var/log/netdata/debug.log && \
    ln -sf /dev/stderr /var/log/netdata/error.log

ENV NETDATA_PORT 19999
EXPOSE $NETDATA_PORT

ENTRYPOINT ["/usr/sbin/run.sh"]
