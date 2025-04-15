#!/usr/bin/env sh

set -e

repo="${1}"
tag="${2}"
count="${3}"

retag_image() {
    new_tag="${1}"
    old_tag="${2}"

    if docker manifest inspect "${old_tag}" > /dev/null; then
        docker buildx imagetools create --tag "${new_tag}" "${old_tag}"
    fi
}

for i in $(seq "${count}" -1 1); do
    retag_image "${repo}:${tag}-${i}" "${repo}:${tag}-$((i - 1))"
done

retag_image "${repo}:${tag}-0" "${repo}:${tag}"
