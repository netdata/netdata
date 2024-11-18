#!/bin/sh

echo ">>> Installing CI support packages..."
sh -x .github/scripts/ci-support-pkgs.sh
mkdir -p /etc/cron.daily # Needed to make auto-update checking work correctly on some platforms.
echo ">>> Installing Netdata..."
sh -x packaging/installer/kickstart.sh --dont-wait --build-only --dont-start-it --disable-telemetry "${EXTRA_INSTALL_FLAGS:+--local-build-options "${EXTRA_INSTALL_FLAGS}"}" || exit 1
echo "::group::>>> Pre-Update Environment File Contents"
cat /etc/netdata/.environment
echo "::endgroup::"
echo "::group::>>> Pre-Update Netdata Build Info"
netdata -W buildinfo
echo "::endgroup::"
echo ">>> Updating Netdata..."
export NETDATA_BASE_URL="http://localhost:8080" # Pull the tarball from the local web server.
echo 'NETDATA_ACCEPT_MAJOR_VERSIONS="1 9999"' > /etc/netdata/netdata-updater.conf
timeout 3600 sh -x packaging/installer/netdata-updater.sh --not-running-from-cron --no-updater-self-update

case "$?" in
    124) echo "!!! Updater timed out." ; exit 1 ;;
    0) ;;
    *) echo "!!! Updater failed." ; exit 1 ;;
esac
echo "::group::>>> Post-Update Environment File Contents"
cat /etc/netdata/.environment
echo "::endgroup::"
echo "::group::>>> Post-Update Netdata Build Info"
netdata -W buildinfo
echo "::endgroup::"
echo ">>> Checking if update was successful..."
sh -x .github/scripts/check-updater.sh || exit 1
