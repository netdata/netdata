#!/usr/bin/env bats
#
# Netdata installation lifecycle testing script.
# This is to validate the install, update and uninstall of netdata
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis <paul@netdata.cloud)
#

INSTALLATION="$BATS_TMPDIR/installation"
ENV="${INSTALLATION}/netdata/etc/netdata/.environment"
# list of files which need to be checked. Path cannot start from '/'
FILES="usr/libexec/netdata/plugins.d/go.d.plugin
       usr/libexec/netdata/plugins.d/charts.d.plugin
       usr/libexec/netdata/plugins.d/python.d.plugin
       usr/libexec/netdata/plugins.d/node.d.plugin"

DIRS="usr/sbin/netdata
      etc/netdata
      usr/share/netdata
      usr/libexec/netdata
      var/cache/netdata
      var/lib/netdata
      var/log/netdata"

setup() {
	# If we are not in netdata git repo, at the top level directory, fail
	TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
	CWD=$(git rev-parse --show-cdup || echo "")
	if [ -n "${CWD}" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
		echo "Run as ./tests/lifecycle/$(basename "$0") from top level directory of git repository"
		exit 1
	fi
}

@test "install netdata" {
	./netdata-installer.sh  --dont-wait --dont-start-it --auto-update --install "${INSTALLATION}"

	# Validate particular files
	for file in $FILES; do
		[ ! -f "$BATS_TMPDIR/$file" ]
	done

	# Validate particular directories
	for a_dir in $DIRS; do
		[ ! -d "$BATS_TMPDIR/$a_dir" ]
	done
}

@test "update netdata" {
	export ENVIRONMENT_FILE="${ENV}"
	${INSTALLATION}/netdata/usr/libexec/netdata/netdata-updater.sh --not-running-from-cron
	! grep "new_installation" "${ENV}"
}

@test "uninstall netdata" {
	./packaging/installer/netdata-uninstaller.sh --yes --force --env "${ENV}"
	[ ! -f "${INSTALLATION}/netdata/usr/sbin/netdata" ]
	[ ! -f "/etc/cron.daily/netdata-updater" ]
}
