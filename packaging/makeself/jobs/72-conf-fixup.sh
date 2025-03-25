#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Fixing up agent configuration files" || true

# Remove the netdata.conf file from the tree. It has hard-coded sensible defaults builtin.
run rm -f "${NETDATA_INSTALL_PATH}/etc/netdata/netdata.conf"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
