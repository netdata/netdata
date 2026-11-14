#!/bin/sh

set -e

build_dir="${1}"
prefix="${2}"
install_dir="${3}"

cmake --install "${build_dir}"

cp -a --reflink=auto "${prefix}/etc" "${install_dir}"
cp -a --reflink=auto "${prefix}/var" "${install_dir}"
[ -d "${prefix}/usr/sbin" ] && cp -a --reflink=auto "${prefix}"/usr/sbin/* "${install_dir}/bin"
[ -d "${prefix}/usr/bin" ] && cp -a --reflink=auto "${prefix}"/usr/bin/* "${install_dir}/bin"

for dir in "${prefix}"/usr/* ; do
    target="${install_dir}/usr/$(basename "${dir}")"
    case "${target}" in
        */bin|*/sbin) continue ;;
        *)
            [ -e "${target}" ] || mkdir -p "${target}"
            cp -a --reflink=auto "${dir}"/* "${target}/"
            ;;
    esac
done
