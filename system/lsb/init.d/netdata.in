#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Netdata LSB start script

### BEGIN INIT INFO
# Provides:          netdata
# Required-Start:    $local_fs $remote_fs $network $named $time
# Required-Stop:     $local_fs $remote_fs $network $named $time
# Should-Start:      $local_fs $network $named $remote_fs $time $all
# Should-Stop:       $local_fs $network $named $remote_fs $time $all
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: Start and stop netdata
# Description:       Controls the main netdata process and all its plugins.
### END INIT INFO
#
set -e
set -u
${DEBIAN_SCRIPT_DEBUG:+ set -v -x}

DAEMON="netdata"
DAEMON_PATH=@sbindir_POST@
PIDFILE_PATH=@localstatedir_POST@/run/netdata
PIDFILE=$PIDFILE_PATH/$DAEMON.pid
DAEMONOPTS="-P $PIDFILE"

test -x $DAEMON_PATH/$DAEMON || exit 0

. /lib/lsb/init-functions

# Safeguard (relative paths, core dumps..)
cd /
umask 022

service_start() {
	if [ ! -d $PIDFILE_PATH ]; then
		mkdir -p $PIDFILE_PATH
	fi
	
	chown @netdata_user_POST@:@netdata_user_POST@ $PIDFILE_PATH
	
	log_success_msg "Starting netdata"
	start_daemon -p $PIDFILE $DAEMON_PATH/$DAEMON $DAEMONOPTS
	RETVAL=$?
	case "${RETVAL}" in
		0) log_success_msg "Started netdata" ;;
		*) log_error_msg "Failed to start netdata" ;;
	esac
	return $RETVAL
}

service_stop() {
	log_success_msg "Stopping netdata"
	killproc -p ${PIDFILE} $DAEMON_PATH/$DAEMON
	RETVAL=$?
	case "${RETVAL}" in
		0) log_success_msg "Stopped netdata" ;;
		*) log_error_msg "Failed to stop netdata" ;;
	esac

	if [ $RETVAL -eq 0 ]; then
		rm -f ${PIDFILE}
	fi
	return $RETVAL
}

condrestart() {
	if ! service_status > /dev/null; then
		RETVAL=$1
		return
	fi

	service_stop
	service_start
}

service_status() {
	pidofproc -p $PIDFILE $DAEMON_PATH/$DAEMON
}


#
# main()
#

case "${1:-''}" in
	'start')
		service_start
		;;

	'stop')
		service_stop
		;;

	'restart')
		service_stop
		service_start
		;;

	'try-restart')
		condrestart 0
		;;

	'force-reload')
		condrestart 7
		;;

	'status')
		service_status && exit 0 || exit $?
		;;
	*)
		echo "Usage: $0 {start|stop|restart|try-restart|force-reload|status}"
		exit 1
esac
