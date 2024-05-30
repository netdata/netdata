#!/bin/sh

set -e

case "$1" in
  configure|reconfigure)
    grep /usr/libexec/netdata /var/lib/dpkg/info/netdata-plugin-ebpf.list | xargs -n 30 chown root:netdata
    chmod -f 4750 /usr/libexec/netdata/plugins.d/ebpf.plugin
    ;;
esac

exit 0
