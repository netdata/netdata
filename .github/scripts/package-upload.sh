#!/bin/sh

set -e

host="packages.netdata.cloud"
user="netdatabot"

distro="${1}"
arch="${2}"
format="${3}"
repo="${4}"

staging="${TMPDIR:-/tmp}/package-staging"
prefix="/home/netdatabot/incoming/${repo}/"

packages="$(find artifacts -name "*.${format}")"

mkdir -p "${staging}"

case "${format}" in
    deb)
        src="${staging}/${distro}"
        mkdir -p "${src}"

        for pkg in ${packages}; do
            cp "${pkg}" "${src}"
        done
        ;;
    rpm)
        src="${staging}/${distro}/${arch}/"
        mkdir -p "${src}"

        for pkg in ${packages}; do
            cp "${pkg}" "${src}"
        done
        ;;
    *)
        echo "Unrecognized package format ${format}."
        exit 1
        ;;
esac

rsync -vrptO "${staging}/" "${user}@${host}:${prefix}"
