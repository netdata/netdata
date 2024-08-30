#!/bin/sh

set -e

case "$1" in
  configure|reconfigure)
    chown root:netdata /usr/libexec/netdata/plugins.d/freeipmi.plugin
    chmod -f 4750 /usr/libexec/netdata/plugins.d/freeipmi.plugin
    ;;
esac

exit 0
