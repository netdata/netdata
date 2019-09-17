#!/usr/bin/env bats
#
# This script is responsible for validating
# updater capabilities after a change
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
		echo "Run as ./tests/$(basename "$0") from top level directory of git repository"
		exit 1
	fi
}

@test "install stable netdata using kickstart" {
	kickstart_file="/tmp/kickstart.$$"
	curl -Ss -o ${kickstart_file} https://my-netdata.io/kickstart.sh
	chmod +x ${kickstart_file}
	${kickstart_file} --dont-wait --dont-start-it --auto-update --install ${INSTALLATION}

	# Validate particular files
	for file in $FILES; do
		[ ! -f "$BATS_TMPDIR/$file" ]
	done

	# Validate particular directories
	for a_dir in $DIRS; do
		[ ! -d "$BATS_TMPDIR/$a_dir" ]
	done

	# Cleanup
	rm -rf ${kickstart_file}
}

@test "update netdata using the new updater" {
	export ENVIRONMENT_FILE="${ENV}"
	# Run the updater, with the override so that it uses the local repo we have at hand
	# Try to run the installed, if any, otherwise just run the one from the repo
	export NETDATA_LOCAL_TARBAL_OVERRIDE="${PWD}"
	/etc/cron.daily/netdata-updater || ./packaging/installer/netdata-updater.sh
	! grep "new_installation" "${ENV}"
}

@test "uninstall netdata using latest uninstaller" {
	./packaging/installer/netdata-uninstaller.sh --yes --force --env "${ENV}"
	[ ! -f "${INSTALLATION}/netdata/usr/sbin/netdata" ]
	[ ! -f "/etc/cron.daily/netdata-updater" ]
}
