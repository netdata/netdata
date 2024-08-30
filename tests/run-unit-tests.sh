#!/usr/bin/env bash
#
# Copyright: 2020-2024 (c) Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Author: Austin S. Hemmelgarn <austin@netdata.cloud>
#
# shellcheck disable=SC2230

install_netdata() {
  echo "Installing Netdata"

  NETDATA_CMAKE_OPTIONS="-DCMAKE_BUILD_TYPE=Debug -DENABLE_ADDRESS_SANITIZER=On" \
  fakeroot ./netdata-installer.sh \
    --install-prefix "$HOME" \
    --dont-wait \
    --dont-start-it \
    --disable-lto
}

c_unit_tests() {
  echo "Running C code unit tests"

  ASAN_OPTIONS=detect_leaks=0 \
  "$HOME"/netdata/usr/sbin/netdata -W unittest
}

install_netdata || exit 1

c_unit_tests || exit 1
