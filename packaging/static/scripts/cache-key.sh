#!/bin/sh

set -e

dir="${1}"
tmpfile="$(mktemp)"

cksum "${dir}/Makefile" > "${tmpfile}" 2>/dev/null
cksum "${dir}"/scripts/* >> "${tmpfile}" 2>/dev/null

md5sum "${tmpfile}" | awk '{ print $1 }'
rm -f "${tmpfile}" 2>/dev/null
