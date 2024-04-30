#!/bin/sh

set -e

case "$1" in
  configure|reconfigure)
    chown root:netdata /usr/libexec/netdata/plugins.d/debugfs.plugin
    chmod 0750 /usr/libexec/netdata/plugins.d/debugfs.plugin
    if ! setcap "cap_dac_read_search=eip" /usr/libexec/netdata/plugins.d/debugfs.plugin; then
        chmod -f 4750 /usr/libexec/netdata/plugins.d/debugfs.plugin
    fi
    ;;
esac

exit 0
