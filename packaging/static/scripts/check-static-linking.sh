#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later

prefix="${1}"
check_path="${prefix}/bin/srv/netdata"

readelf_output="$(readelf -l "${check_path}")" || exit 1
if printf "%s\n" "${readelf_output}" | grep 'INTERP'; then
  printf >&2 "Ooops. %s is not a statically linked binary!\n" "${check_path}"
  ldd "${check_path}"
  exit 1
fi
