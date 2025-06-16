#!/bin/sh

SRC_DIR="${1}"
PATCH_DIR="${2}"

cd "${SRC_DIR}" || exit 1

for patch_file in "${PATCH_DIR}"/*; do
    patch -p1 --forward -i "${patch_file}"
    ret="${?}"

    # We want to ignore patches that are already applied instead of
    # failing the patching process, and patch returns 1 for a patch that
    # is already applied, and higher numbers for other errors.
    if [ "${ret}" -gt 1 ] ; then
        exit 1
    fi
done
