#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS:-false}" = "true" ] && echo "::group::Creating package path inventory" || true

run bash "${NETDATA_MAKESELF_PATH}/create-package-manifest.sh" "${NETDATA_INSTALL_PATH}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS:-false}" = "true" ] && echo "::endgroup::" || true
