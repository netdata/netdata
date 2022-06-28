#!/bin/sh

set -e

host="packages.netdata.cloud"
user="netdatabot"

distro="${1}"
arch="${2}"
format="${3}"
repo="${4}"

staging="/tmp/package-staging/"
prefix="/var/www/html/repos/${repo}/"

packages="$(find artifacts -name "*.${format}")"

mkdir -p "${staging}"

case "${format}" in
    deb)
        src="${staging}/pool/"

        for pkg in ${packages}; do
            cp "${pkg}" "${src}"
        done
        ;;
    rpm)
        src="${staging}/${distro}/${arch}/"

        for pkg in ${packages}; do
            cp "${pkg}" "${src}"
        done
        ;;
    *)
        echo "Unrecognized package format ${format}."
        exit 1
        ;;
esac

rsync -vrlp --omit-dir-times --omit-link-times --inplace "${src}" "${user}@${host}:${prefix}"
