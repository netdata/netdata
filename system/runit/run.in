#!/bin/sh

piddir="@localstatedir_POST@/run/netdata/netdata.pid"
pidfile="${piddir}/netdata.pid"

command="@sbindir_POST@/netdata"
command_args="-P ${pidfile} -D"

[ ! -d "${piddir}" ] && mkdir -p "${piddir}"
chown -R @netdata_user_POST@ "${piddir}"

exec ${command} ${command_args}
