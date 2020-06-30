#!/bin/sh

SRCDIR="${1}"
WEBDIR="${2}"

DASHBOARD_TARBALL="dashboard.tar.gz"
DASHBOARD_VERSION="$(cat "${SRCDIR}/packaging/dashboard.version")"

mkdir -p "${SRCDIR}/tmp/dashboard"
curl -sSL --connect-timeout 10 --retry 3 "https://github.com/netdata/dashboard/releases/download/${DASHBOARD_VERSION}/${DASHBOARD_TARBALL}" > "${DASHBOARD_TARBALL}" || exit 1
sha256sum -c "${SRCDIR}/packaging/dashboard.checksums" || exit 1
tar -xzf "${DASHBOARD_TARBALL}" -C "${SRCDIR}/tmp/dashboard" || exit 1
# shellcheck disable=SC2046
cp -a $(find "${SRCDIR}/tmp/dashboard/build" -mindepth 1 -maxdepth 1) "${WEBDIR}"
