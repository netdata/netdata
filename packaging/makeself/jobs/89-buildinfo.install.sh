#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1


# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Netdata buildinfo" || true

run "${NETDATA_INSTALL_PATH}/bin/netdata" -W buildinfo

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
