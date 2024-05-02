#!/bin/sh

set -e

case "$1" in
  configure|reconfigure)
    chown root:netdata /usr/libexec/netdata/plugins.d/apps.plugin
    chmod 0750 /usr/libexec/netdata/plugins.d/apps.plugin
    if ! setcap "cap_dac_read_search=eip cap_sys_ptrace=eip" /usr/libexec/netdata/plugins.d/apps.plugin; then
        chmod -f 4750 /usr/libexec/netdata/plugins.d/apps.plugin
    fi
    ;;
esac

exit 0
