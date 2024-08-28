#!/bin/sh

set -e

case "$1" in
  configure|reconfigure)
    if ! dpkg-statoverride --list /var/lib/netdata > /dev/null 2>&1; then
      dpkg-statoverride --update --add netdata netdata 0755 /var/lib/netdata
    fi

    if ! dpkg-statoverride --list /var/cache/netdata > /dev/null 2>&1; then
      dpkg-statoverride --update --add netdata netdata 0755 /var/cache/netdata
    fi

    if ! dpkg-statoverride --list /var/run/netdata > /dev/null 2>&1; then
      dpkg-statoverride --update --add netdata netdata 0755 /var/run/netdata
    fi

    if ! dpkg-statoverride --list /var/log/netdata > /dev/null 2>&1; then
      dpkg-statoverride --update --add netdata adm 02750 /var/log/netdata
    fi

    dpkg-statoverride --force --update --add root netdata 0775 /var/lib/netdata/registry > /dev/null 2>&1

    grep /usr/libexec/netdata /var/lib/dpkg/info/netdata.list | xargs -n 30 chown root:netdata

    for f in ndsudo cgroup-network local-listeners ioping.plugin; do
        chmod 4750 "/usr/libexec/netdata/plugins.d/${f}" || true
    done

    ;;
esac

if [ "$1" = "configure" ] || [ "$1" = "abort-upgrade" ] || [ "$1" = "abort-deconfigure" ] || [ "$1" = "abort-remove" ] ; then
    deb-systemd-helper unmask 'netdata.service' >/dev/null || true

    if deb-systemd-helper --quiet was-enabled 'netdata.service'; then
        deb-systemd-helper enable 'netdata.service' >/dev/null || true
    else
        deb-systemd-helper update-state 'netdata.service' >/dev/null || true
    fi

    if [ -z "${DPKG_ROOT:-}" ] && [ -d /run/systemd/system ]; then
        systemctl --system daemon-reload >/dev/null || true
        deb-systemd-invoke restart 'netdata.service' >/dev/null || true
    fi
fi

exit 0
