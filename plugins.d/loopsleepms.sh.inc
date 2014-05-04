#!/bin/sh

# this function is used to sleep a fraction of a second
# it calculates the difference between every time is called
# and tries to align the sleep time to give you exactly the
# loop you need.

LOOPSLEEPMS_HIGHRES=2
LOOPSLEEPMS_LASTRUN=0
LOOPSLEEPMS_LASTSLEEP=0
LOOPSLEEPMS_LASTWORK=0
loopsleepms() {
	tellwork=0
	if [ "$1" = "tellwork" ]
	then
		local tellwork=1
		shift
	fi

	# the time in seconds to wait, as the first argument
	local t=$1

	# check if high resolution timer is supported
	if [ $LOOPSLEEPMS_HIGHRES -eq 2 ]
	then
		LOOPSLEEPMS_HIGHRES=1
		test `date +%N` = "%N" && LOOPSLEEPMS_HIGHRES=0
	fi

	# if high resolution is not supported
	# just sleep the time requested, in seconds
	if [ $LOOPSLEEPMS_HIGHRES -eq 0 ]
	then
		sleep $t
		return
	fi

	# get the current time
	local d=`date +'%s.%N'`
	local s=`echo $d | cut -d '.' -f 1`
	local m=`echo $d | cut -d '.' -f 2 | cut -b 1-3`
	local now="$s$m" # milliseconds since epoch (1-1-1970)

	# calculate required sleep in ms
	t=$((t * 1000))

	# this is our first run
	# just wait the requested time
	test $LOOPSLEEPMS_LASTRUN -eq 0 && LOOPSLEEPMS_LASTRUN=$now

	# calculate ms since last run
	LOOPSLEEPMS_LASTWORK=$((now - LOOPSLEEPMS_LASTRUN - LOOPSLEEPMS_LASTSLEEP))
	# echo "# last loop's work took $LOOPSLEEPMS_LASTWORK ms"

	test $tellwork -eq 1 && echo >&2 " >>> PERFORMANCE >>> WORK TOOK $LOOPSLEEPMS_LASTWORK ms ( $((LOOPSLEEPMS_LASTWORK * 100 / 1000)).$((LOOPSLEEPMS_LASTWORK % 10))% cpu )"

	# calculate ms to sleep
	local mstosleep=$((t - LOOPSLEEPMS_LASTWORK))
	# echo "# mstosleep is $mstosleep ms"
	
	# if we are too slow, sleep some time
	test $mstosleep -lt 100 && mstosleep=100

	local s=$((mstosleep / 1000))
	local m=$((mstosleep - (s * 1000)))

	# echo "# sleeping $s.$m"
	# echo
	sleep $s.$m

	# keep the values we need
	# for our next run
	LOOPSLEEPMS_LASTRUN=$now
	LOOPSLEEPMS_LASTSLEEP=$mstosleep
}
