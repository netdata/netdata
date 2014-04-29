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
# > CHART type.chartname family[=chartname] homegroup[=type] charttype[=line] priority[=1000] update_every[=user default]
# charttype = line, area or stacked
# homegroup = any name or the word 'none' which hides the chart from the home web page
# 
# set the chart title
# > TITLE My Super Title
#
# set the units of measurement
# > UNITS my wonderfull unit
#
# create all the dimensions you need
# > DIMENSION CREATE dimensionname algorithm multiplier divisor [hidden]"
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
# A NOTE ABOUT VALUES
# NetData will collect any signed value in the 64bit range:
#
#    -9.223.372.036.854.775.807   to   +9.223.372.036.854.775.807
#
# However, to lower its memory requirements, it stores all values in the
# signed 32bit range, divided by 10, that is:
#
#                  -214.748.364   to    214.748.364
#
# This division by 10, is used to give you a decimal point in the charts.
# In memory, every number is 4 bytes (32bits).
#
# To work with this without loosing detail, you should set the proper
# algorithm of calculation, together with a multiplier and a divider.
#
# The algorithm is applied in the wider 64bit numbers. Once the calculation
# is complete the value is multiplied by the multiplier, by 10, and then
# divided by the divider (all of these at the 64bit level).
# The 64bit result is then stored in a 32 bit signed int.
#
# So, at the chart level:
#
#  - the finest number is 0.1
#  - the smallest -214.748.364,7
#  - the highest   214.748.364,7
#
# You should choose a multiplier and divider to stay within these limits.
#

cat <<EOF
CHART example.random ExampleGroup ExampleCategory stacked 1 1
TITLE Random Numbers Example Chart
UNITS random numbers
UPDATE EVERY $update_every
DIMENSION number1 absolute unsigned int 1 1
DIMENSION number2 absolute unsigned int 1 1
DIMENSION number3 absolute unsigned int 1 1
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

