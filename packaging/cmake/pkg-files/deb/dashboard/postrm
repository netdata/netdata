#!/bin/sh

set -e

case "$1" in
  remove) ;;

  purge)
    if dpkg-statoverride --list | grep -qw /var/lib/netdata/www; then
      dpkg-statoverride --remove /var/lib/netdata/www
    fi

    if dpkg-statoverride --list | grep -qw /usr/share/netdata/www; then
      dpkg-statoverride --remove /usr/share/netdata/www
    fi
    ;;
esac
