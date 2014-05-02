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

# create the chart with 3 dimensions
cat <<EOF
CHART example.random '' "Random Numbers Example Chart" "random numbers" random random stacked 500 $update_every
DIMENSION number1 '' absolute 1 1
DIMENSION number2 '' absolute 1 1
DIMENSION number3 '' absolute 1 1 hidden
EOF

# You can create more charts if you like.
# Just add more chart definitions.

# work forever
while [ 1 ]
do
	# do all the work to collect / calculate the values
	# for each dimension

	value1=$RANDOM
	value2=$RANDOM
	value3=$RANDOM


	# write the result of the work.
	cat <<VALUESEOF
BEGIN example.random
SET number1 = $value1
SET number2 = $value2
SET number3 = $value3
END
VALUESEOF

	# if you have more charts, add BEGIN->END statements here

	# wait the time you are required to
	loopsleepms $update_every
done
