#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Preparing installation paths for archive creation" || true

run cd "${NETDATA_SOURCE_PATH}" || exit 1

# -----------------------------------------------------------------------------
# copy the files needed by makeself installation

run mkdir -p "${NETDATA_INSTALL_PATH}/system"

run cp \
  packaging/makeself/post-installer.sh \
  packaging/makeself/install-or-update.sh \
  packaging/installer/functions.sh \
  "${NETDATA_INSTALL_PATH}/system/"

# -----------------------------------------------------------------------------
# create a wrapper to start our netdata with a modified path

run mkdir -p "${NETDATA_INSTALL_PATH}/bin/srv"

run mv "${NETDATA_INSTALL_PATH}/bin/netdata" \
  "${NETDATA_INSTALL_PATH}/bin/srv/netdata" || exit 1

cat > "${NETDATA_INSTALL_PATH}/bin/netdata" << EOF
#!${NETDATA_INSTALL_PATH}/bin/bash
export NETDATA_BASH_LOADABLES="DISABLE"
export PATH="${NETDATA_INSTALL_PATH}/bin:\${PATH}"
exec "${NETDATA_INSTALL_PATH}/bin/srv/netdata" "\${@}"
EOF
run chmod 755 "${NETDATA_INSTALL_PATH}/bin/netdata"

# -----------------------------------------------------------------------------
# the claiming script must be in the same directory as the netdata binary for web-based claiming to work

run ln -s "${NETDATA_INSTALL_PATH}/bin/netdata-claim.sh" \
  "${NETDATA_INSTALL_PATH}/bin/srv/netdata-claim.sh" || exit 1

# -----------------------------------------------------------------------------
# remove the links to allow untaring the archive

run rm "${NETDATA_INSTALL_PATH}/sbin" \
  "${NETDATA_INSTALL_PATH}/usr/bin" \
  "${NETDATA_INSTALL_PATH}/usr/sbin" \
  "${NETDATA_INSTALL_PATH}/usr/local"

# -----------------------------------------------------------------------------
# ensure required directories actually exist

for dir in var/lib/netdata var/cache/netdata var/log/netdata ; do
    run mkdir -p "${NETDATA_INSTALL_PATH}/${dir}"
    run touch "${NETDATA_INSTALL_PATH}/${dir}/.keep"
done

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
