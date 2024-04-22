#!/bin/sh

set -e

case "$1" in
  configure|reconfigure)
    chown root:netdata /usr/libexec/netdata/plugins.d/network-viewer.plugin
    chmod 0750 /usr/libexec/netdata/plugins.d/network-viewer.plugin
    if ! setcap "cap_dac_read_search,cap_sys_admin,cap_sys_ptrace=eip" /usr/libexec/netdata/plugins.d/network-viewer.plugin; then
        chmod -f 4750 /usr/libexec/netdata/plugins.d/network-viewer.plugin
    fi
    ;;
esac

exit 0
