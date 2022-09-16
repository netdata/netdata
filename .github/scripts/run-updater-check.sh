#!/bin/sh

echo ">>> Installing CI support packages..."
/netdata/.github/scripts/ci-support-pkgs.sh
echo ">>> Installing Netdata..."
/netdata/packaging/installer/kickstart.sh --dont-wait --build-only --disable-telemetry || exit 1
echo "::group::Environment File Contents"
cat /etc/netdata/.environment
echo "::endgroup::"
echo ">>> Updating Netdata..."
export NETDATA_NIGHTLIES_BASEURL="http://localhost:8080/artifacts/" # Pull the tarball from the local web server.
/netdata/packaging/installer/netdata-updater.sh --not-running-from-cron --no-updater-self-update || exit 1
echo ">>> Checking if update was successful..."
/netdata/.github/scripts/check-updater.sh || exit 1
