#!/bin/sh

set -e

case "$1" in
  configure|reconfigure)
    chown root:netdata /usr/libexec/netdata/plugins.d/perf.plugin
    chmod 0750 /usr/libexec/netdata/plugins.d/perf.plugin

    if ! setcap cap_perfmon+ep /usr/libexec/netdata/plugins.d/perf.plugin 2>/dev/null; then
        if ! setcap cap_sys_admin+ep /usr/libexec/netdata/plugins.d/perf.plugin 2>/dev/null; then
            chmod -f 4750 /usr/libexec/netdata/plugins.d/perf.plugin
        fi
    fi
    ;;
esac

exit 0
