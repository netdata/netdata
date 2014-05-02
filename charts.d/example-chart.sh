#!/bin/sh

# report our PID back to netdata
# this is required for netdata to kill this process when it exits
echo "MYPID $$"

# default sleep function
loopsleepms() {
	sleep $1
}
# if found and included, this file overwrites loopsleepms()
# with a high resolution timer function for precise looping.
. "`dirname $0`/loopsleepms.sh.inc"

# netdata passes the requested update frequency as the first argument
update_every=$1
update_every=$(( update_every + 1 - 1))	# makes sure it is a number
test $update_every -eq 0 && update_every=1 # if it is zero, make it 1

# internal default values
enabled="no"

# check if there is a config for us - if there is, bring it in
if [ -f "$0.conf" ]
then
	. "$0.conf"
fi

# check if it is enabled
if [ ! "$enabled" = "yes" ]
then
	echo "DISABLE"
	exit 1
fi

# create the chart with 3 dimensions
cat <<EOF
CHART example.load '' "System Load Average" "load" load load line 500 $update_every
DIMENSION load1 '1 min' absolute 1 100
DIMENSION load5 '5 mins' absolute 1 100
DIMENSION load15 '15 mins' absolute 1 100

CHART example.random '' "Random Numbers Stacked Chart" "% of random numbers" random random stacked 5000 $update_every
DIMENSION random1 '' percentage-of-absolute-row 1 1
DIMENSION random2 '' percentage-of-absolute-row 1 1
DIMENSION random3 '' percentage-of-absolute-row 1 1
EOF

# You can create more charts if you like.
# Just add more chart definitions.

# work forever
while [ 1 ]
do
	# do all the work to collect / calculate the values
	# for each dimension

	# here we parse the system average load
	# it is decimal (with 2 decimal digits), so we remove the dot and
	# at the definition we have divisor = 100, to have the graph show the right value
	loadavg="`cat /proc/loadavg | sed -e "s/\.//g"`"
	load1=`echo $loadavg | cut -d ' ' -f 1`
	load5=`echo $loadavg | cut -d ' ' -f 2`
	load15=`echo $loadavg | cut -d ' ' -f 3`

	value1=$RANDOM
	value2=$RANDOM
	value3=$RANDOM

	# write the result of the work.
	cat <<VALUESEOF
BEGIN example.load
SET load1 = $load1
SET load5 = $load5
SET load15 = $load15
END

BEGIN example.random
SET random1 = $value1
SET random2 = $value2
SET random3 = $value3
END
VALUESEOF

	# if you have more charts, add BEGIN->END statements here

	# wait the time you are required to
	loopsleepms $update_every
done
