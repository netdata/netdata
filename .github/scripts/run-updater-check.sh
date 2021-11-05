#!/bin/sh

echo ">>> Installing Netdata..."
/netdata/packaging/installer/kickstart.sh --dont-wait --disable-telemetry || exit 1
echo ">>> Updating Netdata..."
/netdata/packaging/installer/netdata-updater.sh --not-running-from-cron --no-updater-self-update || exit 1
echo ">>> Checking if update was successful..."
/netdata/.github/scripts/check-updater.sh || exit 1
