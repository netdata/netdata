#!/bin/sh

set -e

user="netdatabot"

host="${1}"
distro="${2}"
arch="${3}"
format="${4}"
repo="${5}"
pkg_src="${6:-./artifacts}"

staging="${TMPDIR:-/tmp}/package-staging"
prefix="/home/netdatabot/incoming/${repo}/"

packages="$(find "${pkg_src}" -name "*.${format}")"

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
