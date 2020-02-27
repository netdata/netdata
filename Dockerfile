# TODO: Create a netdata/package-builder:alpine9
#FROM netdata/package-buidler:alpine AS build
FROM alpine:3.9 AS build

# Install Dependencies
RUN apk add --no-cache -U alpine-sdk bash curl libuv-dev zlib-dev \
                          util-linux-dev libmnl-dev gcc make git autoconf \
                          automake pkgconfig python logrotate openssl-dev

# Pass optional ./netdata-installer.sh args with --build-arg INSTALLER_ARGS=...
ARG INSTALLER_ARGS=""

# Copy Sources
# Can also bind-mount sources with:
# $ docker run -v $PWD:/netdata

WORKDIR /netdata
COPY . .

# Build
RUN ./netdata-installer.sh --dont-wait --dont-start-it --disable-go "${INSTALLER_ARGS}"

FROM alpine:3.9 AS runtime

# Install runtime dependeices
RUN apk --no-cache -U add curl bash libuv zlib util-linux libmnl python

# Create netdata user/group
RUN addgroup -S netdata && \
	adduser -D -S -h /var/empty -s /bin/false -G netdata netdata

# Copy binary from build layer
COPY --from=build /usr/sbin/netdata /usr/sbin/netdata

# Copy configuration files from build layer
COPY --from=build /etc/netdata/ /etc/netdata/
COPY --from=build /usr/lib/netdata/ /usr/lib/netdata/

# Copy assets from build layer
COPY --from=build /usr/share/netdata/ /usr/share/netdata/

# Create some directories netdata needs
RUN mkdir -p \
	/etc/netdata \
	/var/log/netdata \
	/var/lib/netdata \
	/var/cache/netdata \
	/usr/lib/netdata/conf.d \
	/usr/libexec/netdata/plugins.d

# Fix permissions/ownerships
RUN chown -R netdata:netdata \
	/etc/netdata/ \
	/usr/lib/netdata/ \
	/usr/share/netdata/ \
	/var/log/netdata \
	/var/lib/netdata \
	/var/cache/netdata  \
	/usr/libexec/netdata

VOLUME /etc/netdata
VOLUME /var/lib/netdata
VOLUME /var/log/netdata

EXPOSE 19999/tcp

USER netdata

CMD ["/usr/sbin/netdata", "-D"]
