#!/bin/bash

set -e

host="packages.netdata.cloud"
user="netdatabot"

prefix="/var/www/html/releases"
staging="${TMPDIR:-/tmp}/staging-new-releases"

mkdir -p "${staging}"

for source_dir in "${staging}"/*; do
    if [ -d "${source_dir}" ]; then
        base_name=$(basename "${source_dir}")
        scp -r "${source_dir}"/* "${user}@${host}:${prefix}/${base_name}"
    fi
done
