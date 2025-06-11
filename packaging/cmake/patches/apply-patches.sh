#!/bin/sh

SRC_DIR="${1}"
PATCH_DIR="${2}"

set -e

cd "${SRC_DIR}"

for patch_file in "${PATCH_DIR}"/*; do
    patch -p1 -i "${patch_file}"
done
