#!/bin/bash

set -e

host="packages.netdata.cloud"
user="netdatabot"

prefix="/var/www/html/releases"
staging="${TMPDIR:-/tmp}/staging-new-releases"

mkdir -p "${staging}"

# rsync -av "${staging}" "${user}@${host}:${prefix}"