#!/usr/bin/env bats

INSTALLATION="$BATS_TMPDIR/installation"
ENV="${INSTALLATION}/netdata/etc/netdata/.environment"
# list of files which need to be checked. Path cannot start from '/'
FILES="usr/libexec/netdata/plugins.d/go.d.plugin
       usr/libexec/netdata/plugins.d/charts.d.plugin
       usr/libexec/netdata/plugins.d/python.d.plugin
       usr/libexec/netdata/plugins.d/node.d.plugin"

setup() {
	if [ ! -f .gitignore ];	then
		echo "Run as ./tests/lifecycle/$(basename "$0") from top level directory of git repository"
		exit 1
	fi
}

@test "install netdata" {
	./netdata-installer.sh  --dont-wait --dont-start-it --auto-update --install "${INSTALLATION}"
	for file in $FILES; do
		[ ! -f "$BATS_TMPDIR/$file" ]
	done
}

@test "update netdata" {
	export ENVIRONMENT_FILE="${ENV}"
	/etc/cron.daily/netdata-updater
	! grep "new_installation" "${ENV}"
}

@test "uninstall netdata" {
	./packaging/installer/netdata-uninstaller.sh --yes --force --env "${ENV}"
	[ ! -f "${INSTALLATION}/netdata/usr/sbin/netdata" ]
	[ ! -f "/etc/cron.daily/netdata-updater" ]
}
