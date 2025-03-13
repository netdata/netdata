#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Setting correct install type information in install path" || true

# Properly mark the install type
cat > "${NETDATA_INSTALL_PATH}/etc/netdata/.install-type" <<-EOF
	INSTALL_TYPE='manual-static'
	PREBUILT_ARCH='${BUILDARCH}'
	EOF

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
