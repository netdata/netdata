#!/bin/bash

export PATH="${PATH}:/sbin:/usr/sbin:/usr/local/sbin"

# default time function
now_ms=
current_time_ms() {
	now_ms="$(date +'%s')000"
}

# default sleep function
LOOPSLEEPMS_LASTWORK=0
loopsleepms() {
	[ "$1" = "tellwork" ] && shift
	sleep $1
}

# if found and included, this file overwrites loopsleepms()
# with a high resolution timer function for precise looping.
. "$NETDATA_PLUGINS_DIR/loopsleepms.sh.inc"

# check if we have a valid number for interval
t=$1
sleep_time=$((t))
[ $((sleep_time)) -lt 1 ] && $NETDATA_UPDATE_EVERY
[ $((sleep_time)) -lt 1 ] && sleep_time=1

tc_cmd="$(which tc)"
if [ -z "$tc_cmd" ]
	then
	echo >&2 "tc: Cannot find a 'tc' command in this system."
	exit 1
fi

devices=
fix_names=

setclassname() {
	echo "SETCLASSNAME $3 $2"
}

show_tc() {
	local x="$1"

	echo "BEGIN $x"
	$tc_cmd -s class show dev $x

	# check FireQOS names for classes
	if [ ! -z "$fix_names" -a -f /var/run/fireqos/ifaces/$x ]
	then
		name="$(cat /var/run/fireqos/ifaces/$x)"
		echo "SETDEVICENAME $name"

		interface_classes=
		interface_classes_monitor=
		. /var/run/fireqos/$name.conf
		for n in $interface_classes_monitor
		do
			setclassname $(echo $n | tr '|' ' ')
		done
		echo "SETDEVICEGROUP $interface_dev"
	fi
	echo "END $x"
}

all_devices() {
	cat /proc/net/dev | grep ":" | cut -d ':' -f 1 | while read dev
	do
		l=$($tc_cmd class show dev $dev | wc -l)
		[ $l -ne 0 ] && echo $dev
	done
}

# update devices and class names
# once every 2 minutes
names_every=$((120 / sleep_time))

# exit this script every hour
# it will be restarted automatically
exit_after=$((3600 / sleep_time))

c=0
gc=0
while [ 1 ]
do
	fix_names=
	c=$((c + 1))
	gc=$((gc + 1))

	if [ $c -le 1 -o $c -ge $names_every ]
	then
		c=1
		fix_names="YES"
		devices="$( all_devices )"
	fi

	for d in $devices
	do
		show_tc $d
	done

	echo "WORKTIME $LOOPSLEEPMS_LASTWORK"

	loopsleepms $sleep_time

	[ $gc -gt $exit_after ] && exit 0
done
