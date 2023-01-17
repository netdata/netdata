#!/bin/sh

echo ">>> Installing CI support packages..."
/netdata/.github/scripts/ci-support-pkgs.sh
echo ">>> Installing Netdata..."
/netdata/packaging/installer/kickstart.sh --dont-wait --build-only --disable-telemetry --disable-cloud --local-build-options --disable-ml --local-build-options --disable-ebpf --local-build-options --disable-go --local-build-options --use-system-protobuf || exit 1
echo "::group::>>> Pre-Update Environment File Contents"
cat /etc/netdata/.environment
echo "::endgroup::"
echo "::group::>>> Pre-Update Netdata Build Info"
netdata -W buildinfo
echo "::endgroup::"
echo ">>> Updating Netdata..."
cp /dev/null /var/log/netdata/error.log 2>/dev/null
export NETDATA_BASE_URL="http://localhost:8080/artifacts/" # Pull the tarball from the local web server.
timeout 900 /netdata/packaging/installer/netdata-updater.sh --not-running-from-cron --no-updater-self-update

case "$?" in
124)
    echo "!!! Updater timed out."
    echo "::group::>>> Netdata error.log"
    cat /var/log/netdata/error.log
    echo "::endgroup::"
    exit 1
    ;;
0) ;;
*)
    echo "!!! Updater failed."
    exit 1
    ;;
esac
echo "::group::>>> Post-Update Environment File Contents"
cat /etc/netdata/.environment
echo "::endgroup::"
echo "::group::>>> Post-Update Netdata Build Info"
netdata -W buildinfo
echo "::endgroup::"
echo ">>> Checking if update was successful..."
/netdata/.github/scripts/check-updater.sh || exit 1
