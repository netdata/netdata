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

rrddim_collection_test() {
  echo "Running RRD dimension collection component test"

  local build_dir="${NETDATA_BUILD_DIR:-./build}"
  local config_h="${build_dir}/config.h"

  if [[ ! -r "${config_h}" ]]; then
    echo >&2 "Cannot read CMake configuration: ${config_h}"
    return 1
  fi

  if ! grep -Fqx '#define OS_LINUX' "${config_h}"; then
    echo "Skipping RRD dimension collection component test (requires Linux)"
    return 0
  fi

  cmake --build "${build_dir}" --target rrddim-collection-test || return 1
  "${build_dir}"/rrddim-collection-test
}

spawn_server_unit_tests() {
  echo "Running spawn-server unit tests"

  local run_dir
  run_dir="$(mktemp -d "${TMPDIR:-/tmp}/netdata-spawn-tester.XXXXXX")" || return 1

  NETDATA_RUN_DIR="${run_dir}" \
  ASAN_OPTIONS=detect_leaks=0 \
  "${NETDATA_BUILD_DIR:-./build}/spawn-tester" test
  local ret=$?

  rm -rf -- "${run_dir}"
  return "${ret}"
}

install_netdata || exit 1

c_unit_tests || exit 1

rrddim_collection_test || exit 1

spawn_server_unit_tests || exit 1
