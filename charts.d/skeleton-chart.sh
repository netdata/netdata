#!/bin/sh

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

# report the PID back to netdata
# this is required for netdata to kill this process when it exits
echo "MYPID $$"

# -----------------------------------------------------------------------------
# create a new chart
# > CHART family.chartname group[=type] homegroup[=family] charttype priority[=1000]
# charttype = line or area or stacked
# homegroup = any name or the word 'none' which hides the chart from the home web page
# 
# set the chart title
# > TITLE My Super Title
#
# set the units of measurement
# > UNITS my wonderfull unit
#
# you can overwrite the update frequency as you need
# > UPDATE EVERY $update_every
#
# create all the dimensions you need
# > DIMENSION CREATE dimensionname1 algorithm signed|unsigned byte|char|int|long|long long"
#
# algorithms:
#   absolute
#     the value is to drawn as-is
#
#   incremental
#     the value increases over time
#     the difference from the last value is drawn
#
#   percentage-of-absolute-row
#     the % is drawn of this value compared to the total of all dimensions
#
#   percentage-of-incremental-row
#     the % is drawn of this value compared to the differential total of
#     each dimension
#
# number sizes:
#   unsigned byte      = 1 byte  = 0 ...                        255
#   unsigned int       = 2 bytes = 0 ...                     65.535
#   unsigned long      = 4 bytes = 0 ...              4.294.967.295
#   unsigned long long = 8 bytes = 0 ... 18.446.744.073.709.551.615
#   
#   signed values are from - to + the half of the above
#

cat <<EOF
CHART example.random ExampleGroup ExampleCategory stacked 1
TITLE Random Numbers Example Chart
UNITS random numbers
UPDATE EVERY $update_every
DIMENSION number1 absolute unsigned int
DIMENSION number2 absolute unsigned int
DIMENSION number3 absolute unsigned int
EOF

# You can create more charts if you like.
# Just add more chart definitions.

while [ 1 ]
do
	# do all the work to calculate the numbers

	value1=$RANDOM
	value2=$RANDOM
	value3=$RANDOM


	# write the result of the work.
	# it is important to keep this short.
	# between BEGIN and END statements the clients are blocked access to the chart.
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

