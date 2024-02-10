#!/bin/sh

arch="${1}"
platform="$(packaging/makeself/uname2platform.sh "${arch}")"
builder_rev="v1"

docker pull --platform "${platform}" netdata/static-builder:${builder_rev}

# shellcheck disable=SC2046
cat $(find packaging/makeself/jobs -type f ! -regex '.*\(netdata\|-makeself\).*') > /tmp/static-cache-key-data
cat packaging/makeself/bundled-packages.version >> /tmp/static-cache-key-data

docker run -it --rm --platform "${platform}" netdata/static-builder:${builder_rev} sh -c 'apk list -I 2>/dev/null' >> /tmp/static-cache-key-data

h="$(sha256sum /tmp/static-cache-key-data | cut -f 1 -d ' ')"

echo "key=static-${arch}-${h}" >> "${GITHUB_OUTPUT}"
