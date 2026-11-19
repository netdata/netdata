#!/bin/sh
# shellcheck shell=bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Makeself extracts into temporary staging before this owner touches /opt/netdata.
# Use the supplied static Bash for the existing installer helper functions.
if [ "${NETDATA_DEM_STATIC_OWNER_BASH:-0}" != 1 ]; then
    NETDATA_DEM_STATIC_OWNER_BASH=1
    export NETDATA_DEM_STATIC_OWNER_BASH
    exec ./bin/bash "$0" "$@"
fi
DP_STAGE=$(pwd)
chmod 755 "$DP_STAGE" || exit $?
if [ -f "$DP_STAGE/usr/libexec/netdata/plugins.d/dem.plugin" ]; then
    chmod 755 "$DP_STAGE/usr/libexec/netdata/plugins.d/dem.plugin" || exit $?
fi
# shellcheck source=packaging/installer/functions.sh
. "$DP_STAGE/system/functions.sh"
# shellcheck source=packaging/installer/netdata-dem-pair.sh
. "$DP_STAGE/system/netdata-dem-pair.sh"
NETDATA_PREFIX=/opt/netdata
DP_DONT_START=0
for static_option in "$@"; do
    [ "$static_option" != --dont-start-it ] || DP_DONT_START=1
done
dem_pair_plan "$DP_STAGE/usr/libexec/netdata/plugins.d/dem.plugin" \
    /opt/netdata/bin/srv/netdata /opt/netdata static "$DP_STAGE/bin/srv/netdata" "$DP_STAGE/bin/netdatacli" || exit $?
if [ "$DP_ENABLED" = 1 ]; then
    trap 'dem_pair_exit "$?"' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM HUP
    dem_pair_begin || exit $?
fi
dashboard_upgrade=0
if [ -x /opt/netdata/bin/netdata ]; then dashboard_upgrade=1; fi
# The ordinary static installer owns payload replacement on both routes.
dem_pair_run mkdir -p /opt/netdata || exit $?
(
    set -o pipefail
    cd "$DP_STAGE" || exit $?
    # GNU tar needs an explicit option; BusyBox already follows directory links.
    set --
    case "$(tar --help 2>&1)" in
        *--keep-directory-symlink*) set -- --keep-directory-symlink ;;
    esac
    dem_pair_run tar -cf - --exclude=./etc/netdata --exclude=./var/lib/netdata \
        --exclude=./var/cache/netdata --exclude=./var/log/netdata . |
        dem_pair_run tar -xpf - "$@" -C /opt/netdata
) || exit $?
dashboard_cleanup_status=0
if [ "$dashboard_upgrade" = 1 ]; then
    run sh "$DP_STAGE/system/cleanup-dashboard.sh" "$DP_STAGE/usr/share/netdata/web/v3" \
        /opt/netdata usr/share/netdata/web || dashboard_cleanup_status=$?
fi
if [ ! -d /opt/netdata/etc/netdata ]; then
    dem_pair_run mkdir -p /opt/netdata/etc || exit $?
    dem_pair_run cp -a "$DP_STAGE/etc/netdata" /opt/netdata/etc/netdata || exit $?
fi
# Configuration contents and existing directory permissions belong to the
# administrator; packaged directories and the editing tool still need updating.
(
    set -o pipefail
    cd "$DP_STAGE/etc/netdata" || exit $?
    find . -type d -print0 | while IFS= read -r -d '' static_directory; do
        dem_pair_run mkdir -p "/opt/netdata/etc/netdata/$static_directory" || exit $?
    done
) || exit $?
dem_pair_run install -m 755 "$DP_STAGE/etc/netdata/edit-config" /opt/netdata/etc/netdata/edit-config || exit $?
for static_metadata in .environment .install-type; do
    if [ ! -e "/opt/netdata/etc/netdata/$static_metadata" ] &&
       [ ! -L "/opt/netdata/etc/netdata/$static_metadata" ]; then
        dem_pair_run cp -p "$DP_STAGE/etc/netdata/$static_metadata" "/opt/netdata/etc/netdata/$static_metadata" || exit $?
    fi
done
cd /opt/netdata || exit $?
if [ "$DP_ENABLED" = 1 ]; then
    # Postinstall owns account/service setup; activation owns the deferred start.
    dem_pair_run ./bin/bash system/install-or-update.sh "$@" --dont-start-it || exit $?
    dem_pair_finish || exit $?
else
    dem_pair_run ./bin/bash system/install-or-update.sh "$@" || exit $?
fi
dem_pair_cleanup_stage || exit $?
trap - EXIT HUP INT TERM
exit "$dashboard_cleanup_status"
