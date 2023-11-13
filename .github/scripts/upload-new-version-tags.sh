#!/bin/bash

set -e

paths_array=$1
vers_array=$2

IFS=' ' read -r -a paths <<< "$paths_array"
IFS=' ' read -r -a versions <<< "$vers_array"

echo "${paths[@]}"
echo "${versions[@]}"

host="packages.netdata.cloud"
user="netdatabot"

prefix="/var/www/html/releases"
staging="${TMPDIR:-/tmp}/staging-new-releases"

mkdir -p "${staging}"

# rsync -av "${staging}" "${user}@${host}:${prefix}"