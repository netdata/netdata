#!/bin/bash

# this function is used to sleep a fraction of a second
# it calculates the difference between every time is called
# and tries to align the sleep time to give you exactly the
# loop you need.

LOOPSLEEP_DATE="$(which date)"
if [ -z "$LOOPSLEEP_DATE" ]
	then
	echo >&2 "$0: ERROR: Cannot find the command 'date' in the system path."
	exit 1
fi

LOOPSLEEPMS_LASTRUN=0
LOOPSLEEPMS_LASTSLEEP=0
LOOPSLEEPMS_LASTWORK=0

LOOPSLEEPMS_HIGHRES=1
test "$($LOOPSLEEP_DATE +%N)" = "%N" && LOOPSLEEPMS_HIGHRES=0

now_ms=
current_time_ms() {
	# if high resolution is not supported
	# just sleep the time requested, in seconds
	if [ $LOOPSLEEPMS_HIGHRES -eq 0 ]
	then
		now_ms="$($LOOPSLEEP_DATE +'%s')000"
	else
		now_ms="$(( $( $LOOPSLEEP_DATE +'%s * 1000 + %-N / 1000000' ) ))"
	fi
}

loopsleepms() {
	local tellwork=0 t="$1" div s m now mstosleep

	if [ "$t" = "tellwork" ]
	then
		tellwork=1
		shift
		t="$1"
	fi
	div="${2-100}"

	# $t = the time in seconds to wait

	# if high resolution is not supported
	# just sleep the time requested, in seconds
	if [ $LOOPSLEEPMS_HIGHRES -eq 0 ]
	then
		sleep $t
		return
	fi

	# get the current time, in ms
	# milliseconds since epoch (1-1-1970)
	now="$(( $( $LOOPSLEEP_DATE +'%s * 1000 + %-N / 1000000' ) ))"

	# calculate required sleep in ms
	t=$((t * 1000 * div / 100))

	# this is our first run
	# just wait the requested time
	test $LOOPSLEEPMS_LASTRUN -eq 0 && LOOPSLEEPMS_LASTRUN=$now

	# calculate ms since last run
	LOOPSLEEPMS_LASTWORK=$((now - LOOPSLEEPMS_LASTRUN - LOOPSLEEPMS_LASTSLEEP))
	# echo "# last loop's work took $LOOPSLEEPMS_LASTWORK ms"

	# calculate ms to sleep
	mstosleep=$(( t - LOOPSLEEPMS_LASTWORK ))
	# echo "# mstosleep is $mstosleep ms"

	# if we are too slow, sleep some time
	test $mstosleep -lt 200 && mstosleep=200

	s=$((mstosleep / 1000))
	m=$((mstosleep - (s * 1000)))

	test $tellwork -eq 1 && echo >&2 " >>> PERFORMANCE >>> WORK TOOK $LOOPSLEEPMS_LASTWORK ms ( $((LOOPSLEEPMS_LASTWORK * 100 / 1000)).$((LOOPSLEEPMS_LASTWORK % 10))% cpu ) >>> SLEEPING $mstosleep ms"

	# echo "# sleeping $s.$m"
	# echo
	sleep $s.$m

	# keep the values we need
	# for our next run
	LOOPSLEEPMS_LASTRUN=$now
	LOOPSLEEPMS_LASTSLEEP=$mstosleep
}
