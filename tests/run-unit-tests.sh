#!/usr/bin/env bash
#
# Unit-testing script
#
# This script does the following:
#   1. Check whether any files were modified that would necessitate unit testing (using the `TRAVIS_COMMIT_RANGE` environment variable).
#   2. If there are no changed files that require unit testing, exit successfully.
#   3. Otherwise, run all the unit tests.
#
# We do things this way because our unit testing takes a rather long
# time (average 18-19 minutes as of the original creation of this script),
# so skipping it when we don't actually need it can significantly speed
# up the CI process.
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
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
    --disable-lto \
    --enable-logsmanagement-tests
}

c_unit_tests() {
  echo "Running C code unit tests"

  ASAN_OPTIONS=detect_leaks=0 \
  "$HOME"/netdata/usr/sbin/netdata -W unittest
}

install_netdata || exit 1

c_unit_tests || exit 1
