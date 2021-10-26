#!/bin/sh

/netdata/packaging/installer/kickstart.sh --dont-wait --disable-telemetry || exit 1
/netdata/packaging/installer/netdata-updater.sh --not-running-from-cron --no-updater-self-update || exit 1
/netdata/.github/scripts/check-updater.sh || exit 1
