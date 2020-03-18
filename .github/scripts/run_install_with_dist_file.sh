#!/usr/bin/env bash
#
# This script is evaluating netdata installation with the source from make dist
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis <paul@netdata.cloud)

set -e

if [ $# -ne 1 ]; then
  printf >&2 "Usage: %s <dist_file>\n" "$(basename "$0")"
  exit 1
fi

distfile="${1}"
shift

printf >&2 "Opening dist archive %s ... " "${distfile}"
tar -xovf "${distfile}"
distdir="$(echo "${distfile}" | cut -d. -f1,2,3)"
if [ ! -d "${distdir}" ]; then
  printf >&2 "ERROR: %s is not a directory" "${distdir}"
  exit 2
fi

printf >&2 "Entering %s and starting docker run ..." "${distdir}"

pushd "${distdir}" || exit 1
docker run \
  -v "${PWD}:/netdata" \
  -w /netdata \
  "netdata/os-test:centos7" \
  /bin/bash -c "./netdata-installer.sh --dont-wait --install /tmp && echo \"Validating netdata instance is running\" && wget -O - 'http://127.0.0.1:19999/api/v1/info' | grep version"
popd || exit 1

echo "All Done!"
