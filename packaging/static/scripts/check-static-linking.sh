#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later

prefix="${1}"

if readelf -l "${prefix}/bin/srv/netdata" | grep 'INTERP'; then
  printf >&2 "Ooops. %s is not a statically linked binary!\n" "${prefix}/bin/netdata"
  ldd "${prefix}/bin/srv/netdata"
  exit 1
fi
