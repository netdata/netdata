#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

cd "${NETDATA_SOURCE_PATH}" || exit 1

if [ ${NETDATA_BUILD_WITH_DEBUG} -eq 0 ]; then
  export CFLAGS="-static -O3"
else
  export CFLAGS="-static -O1 -ggdb -Wall -Wextra -Wformat-signedness -fstack-protector-all -D_FORTIFY_SOURCE=2 -DNETDATA_INTERNAL_CHECKS=1"
#    export CFLAGS="-static -O1 -ggdb -Wall -Wextra -Wformat-signedness"
fi

# We export this to 'yes', installer sets this to .environment.
# The updater consumes this one, so that it can tell whether it should update a static install or a non-static one
export IS_NETDATA_STATIC_BINARY="yes"

run ./netdata-installer.sh --install "${NETDATA_INSTALL_PARENT}" \
  --dont-wait \
  --dont-start-it \
  "${NULL}"

# Remove the netdata.conf file from the tree. It has hard-coded sensible defaults builtin.
rm -f "${NETDATA_INSTALL_PARENT}/etc/netdata/netdata.conf"

# Ensure the netdata binary is in fact statically linked
if run readelf -l "${NETDATA_INSTALL_PATH}"/bin/netdata | grep 'INTERP'; then
  printf >&2 "Ooops. %s is not a statically linked binary!\n" "${NETDATA_INSTALL_PATH}"/bin/netdata
  ldd "${NETDATA_INSTALL_PATH}"/bin/netdata
  exit 1
fi

if [ ${NETDATA_BUILD_WITH_DEBUG} -eq 0 ]; then
  run strip "${NETDATA_INSTALL_PATH}"/bin/netdata
  run strip "${NETDATA_INSTALL_PATH}"/usr/libexec/netdata/plugins.d/apps.plugin
  run strip "${NETDATA_INSTALL_PATH}"/usr/libexec/netdata/plugins.d/cgroup-network
fi
