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
distdir="$(echo "${distfile}" | rev | cut -d. -f3- | rev)"
cp -a packaging/installer/install-required-packages.sh "${distdir}/install-required-packages.sh"
if [ ! -d "${distdir}" ]; then
  printf >&2 "ERROR: %s is not a directory" "${distdir}"
  exit 2
fi

printf >&2 "Entering %s and starting docker run ..." "${distdir}"

pushd "${distdir}" || exit 1
docker run \
  -e DISABLE_TELEMETRY=1 \
  -v "${PWD}:/netdata" \
  -w /netdata \
  "ubuntu:latest" \
  /bin/bash -c "./install-required-packages.sh --dont-wait --non-interactive netdata && apt install wget && ./netdata-installer.sh --dont-wait --require-cloud --disable-telemetry --install-prefix /tmp --one-time-build && echo \"Validating netdata instance is running\" && wget -O - 'http://127.0.0.1:19999/api/v1/info' | grep version"
popd || exit 1

echo "All Done!"
