#!/bin/sh
# Start/stop the netdata daemon.
#
### BEGIN INIT INFO
# Provides:          netdata
# Required-Start:    $remote_fs
# Required-Stop:     $remote_fs
# Should-Start:      $network
# Should-Stop:       $network
# Default-Start:     2 3 4 5
# Default-Stop:
# Short-Description: Real-time charts for system monitoring
# Description:       Netdata is a daemon that collects data in realtime (per second)
#                    and presents a web site to view and analyze them. The presentation
#                    is also real-time and full of interactive charts that precisely
#                    render all collected values.
### END INIT INFO

PATH=/bin:/usr/bin:/sbin:/usr/sbin
DESC="netdata daemon"
NAME=netdata
DAEMON=/usr/sbin/netdata
PIDFILE=/var/run/netdata/netdata.pid
SCRIPTNAME=/etc/init.d/"$NAME"

test -f $DAEMON || exit 0

. /lib/lsb/init-functions

[ -r /etc/default/netdata ] && . /etc/default/netdata

case "$1" in
start)	log_daemon_msg "Starting real-time system monitoring" "netdata"
        start_daemon -p $PIDFILE $DAEMON -P $PIDFILE $EXTRA_OPTS
        log_end_msg $?
	;;
stop)	log_daemon_msg "Stopping real-time system monitoring" "netdata"
        killproc -p $PIDFILE $DAEMON
        RETVAL=$?
        [ $RETVAL -eq 0 ] && [ -e "$PIDFILE" ] && rm -f $PIDFILE
        log_end_msg $RETVAL
	# wait for plugins to exit
	sleep 1
        ;;
restart|force-reload) log_daemon_msg "Restarting real-time system monitoring" "netdata"
        $0 stop
        $0 start
        ;;
status)
        status_of_proc -p $PIDFILE $DAEMON $NAME && exit 0 || exit $?
        ;;
*)	log_action_msg "Usage: $SCRIPTNAME {start|stop|status|restart|force-reload}"
        exit 2
        ;;
esac
exit 0
