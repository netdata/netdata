#!/bin/sh

set -e

case "$1" in
  configure|reconfigure)
    grep /usr/libexec/netdata /var/lib/dpkg/info/netdata-plugin-pythond.list | xargs -n 30 chown root:netdata
    ;;
esac

exit 0
