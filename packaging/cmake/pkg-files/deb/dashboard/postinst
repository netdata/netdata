#!/bin/sh

set -e

case "$1" in
  configure|reconfigure)
    if ! dpkg-statoverride --list /usr/share/netdata/www > /dev/null 2>&1; then
      dpkg-statoverride --update --add root netdata 0755 /usr/share/netdata/www
    fi
    ;;
esac
