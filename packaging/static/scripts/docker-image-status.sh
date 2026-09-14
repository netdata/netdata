#!/bin/sh

set -e

docker="${1}"
image="${2}"
platform="${3}"

if ${docker} inspect "${image}" > /dev/null 2>&1; then
    if ${docker} image inspect "${image}" | grep -q 'Variant'; then
        img_platform="$(${docker} image inspect "${image}" --format '{{.Os}}/{{.Architecture}}/{{.Variant}}')"
    else
        img_platform="$(${docker} image inspect "${image}" --format '{{.Os}}/{{.Architecture}}')"
    fi

    if [ "${img_platform}" != "${platform}" ]; then
        echo "MISMATCH"
    else
        echo "OK"
    fi
else
    echo "MISSING"
fi
