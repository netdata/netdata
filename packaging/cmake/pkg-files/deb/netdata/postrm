#!/bin/sh

set -e

case "$1" in
  remove) ;;

  purge)
    if dpkg-statoverride --list | grep -qw /var/cache/netdata; then
      dpkg-statoverride --remove /var/cache/netdata
    fi

    if dpkg-statoverride --list | grep -qw /var/lib/netdata/registry; then
      dpkg-statoverride --remove /var/lib/netdata/registry
    fi

    if dpkg-statoverride --list | grep -qw /var/lib/netdata; then
      dpkg-statoverride --remove /var/lib/netdata
    fi

    if dpkg-statoverride --list | grep -qw /var/run/netdata; then
      dpkg-statoverride --remove /var/run/netdata
    fi

    if dpkg-statoverride --list | grep -qw /var/log/netdata; then
      dpkg-statoverride --remove /var/log/netdata
    fi
    ;;

  *) ;;

esac

if [ "$1" = "remove" ]; then
	if [ -x "/usr/bin/deb-systemd-helper" ]; then
		deb-systemd-helper mask 'netdata.service' >/dev/null || true
	fi
fi

if [ "$1" = "purge" ]; then
	if [ -x "/usr/bin/deb-systemd-helper" ]; then
		deb-systemd-helper purge 'netdata.service' >/dev/null || true
		deb-systemd-helper unmask 'netdata.service' >/dev/null || true
	fi
fi

exit 0
