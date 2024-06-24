#!/bin/sh

set -e

path="${1}"
group="${2}"

[ "$(id -u)" -eq 0 ] || exit 0

chown root:"${group}" "${path}"
chmod 4750 "${path}"
