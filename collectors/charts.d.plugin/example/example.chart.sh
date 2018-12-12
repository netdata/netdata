# shellcheck shell=bash
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0-or-later

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#

# if this chart is called X.chart.sh, then all functions and global variables
# must start with X_

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
example_update_every=

# the priority is used to sort the charts on the dashboard
# 1 = the first chart
example_priority=150000

# to enable this chart, you have to set this to 12345
# (just a demonstration for something that needs to be checked)
example_magic_number=

# global variables to store our collected data
# remember: they need to start with the module name example_
example_value1=
example_value2=
example_value3=
example_value4=
example_last=0
example_count=0

example_get() {
	# do all the work to collect / calculate the values
	# for each dimension
	#
	# Remember:
	# 1. KEEP IT SIMPLE AND SHORT
	# 2. AVOID FORKS (avoid piping commands)
	# 3. AVOID CALLING TOO MANY EXTERNAL PROGRAMS
	# 4. USE LOCAL VARIABLES (global variables may overlap with other modules)

	example_value1=$RANDOM
	example_value2=$RANDOM
	example_value3=$RANDOM
	example_value4=$((8192 + (RANDOM * 16383 / 32767)))

	if [ $example_count -gt 0 ]; then
		example_count=$((example_count - 1))

		[ $example_last -gt 16383 ] && example_value4=$((example_last + (RANDOM * ((32767 - example_last) / 2) / 32767)))
		[ $example_last -le 16383 ] && example_value4=$((example_last - (RANDOM * (example_last / 2) / 32767)))
	else
		example_count=$((1 + (RANDOM * 5 / 32767)))

		if [ $example_last -gt 16383 ] && [ $example_value4 -gt 16383 ]; then
			example_value4=$((example_value4 - 16383))
		fi
		if [ $example_last -le 16383 ] && [ $example_value4 -lt 16383 ]; then
			example_value4=$((example_value4 + 16383))
		fi
	fi
	example_last=$example_value4

	# this should return:
	#  - 0 to send the data to netdata
	#  - 1 to report a failure to collect the data

	return 0
}

# _check is called once, to find out if this chart should be enabled or not
example_check() {
	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	# check something
	[ "${example_magic_number}" != "12345" ] && error "manual configuration required: you have to set example_magic_number=$example_magic_number in example.conf to start example chart." && return 1

	# check that we can collect data
	example_get || return 1

	return 0
}

# _create is called once, to create the charts
example_create() {
	# create the chart with 3 dimensions
	cat <<EOF
CHART example.random '' "Random Numbers Stacked Chart" "% of random numbers" random random stacked $((example_priority)) $example_update_every
DIMENSION random1 '' percentage-of-absolute-row 1 1
DIMENSION random2 '' percentage-of-absolute-row 1 1
DIMENSION random3 '' percentage-of-absolute-row 1 1
CHART example.random2 '' "A random number" "random number" random random area $((example_priority + 1)) $example_update_every
DIMENSION random '' absolute 1 1
EOF

	return 0
}

# _update is called continuously, to collect the values
example_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	example_get || return 1

	# write the result of the work.
	cat <<VALUESEOF
BEGIN example.random $1
SET random1 = $example_value1
SET random2 = $example_value2
SET random3 = $example_value3
END
BEGIN example.random2 $1
SET random = $example_value4
END
VALUESEOF

	return 0
}
