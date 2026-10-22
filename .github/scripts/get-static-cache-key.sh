#!/bin/sh

arch="${1}"
platform="$(make -C packaging/static BUILDARCH="${arch}" docker-platform)"
builder_rev="v1"

packaging/static/scripts/cache-key.sh packaging/static > /tmp/static-cache-key-data

docker pull --platform "${platform}" netdata/static-builder:${builder_rev}
docker run -it --rm --platform "${platform}" netdata/static-builder:${builder_rev} sh -c 'apk list -I 2>/dev/null' >> /tmp/static-cache-key-data

h="$(sha256sum /tmp/static-cache-key-data | cut -f 1 -d ' ')"

echo "key=static-v2-${arch}-${h}" >> "${GITHUB_OUTPUT}"
