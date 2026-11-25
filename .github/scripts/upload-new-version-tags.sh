#!/bin/bash

set -e

user="netdatabot"

host="${1}"

prefix="/var/www/html/releases"
staging="${TMPDIR:-/tmp}/staging-new-releases"

mkdir -p "${staging}"

for source_dir in "${staging}"/*; do
    if [ -d "${source_dir}" ]; then
        base_name=$(basename "${source_dir}")
        scp -r "${source_dir}"/* "${user}@${host}:${prefix}/${base_name}"
    fi
done
