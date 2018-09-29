# shellcheck shell=bash disable=SC2154,SC1072,SC1073,SC2009,SC2162,SC2006,SC2002,SC2086,SC1117
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0+

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#

load_average_update_every=5
load_priority=100

# this is an example charts.d collector
# it is disabled by default.
# there is no point to enable it, since netdata already
# collects this information using its internal plugins.
load_average_enabled=0

load_average_check() {
	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	if [ ${load_average_update_every} -lt 5 ]
		then
		# there is no meaning for shorter than 5 seconds
		# the kernel changes this value every 5 seconds
		load_average_update_every=5
	fi

	[ ${load_average_enabled} -eq 0 ] && return 1
	return 0
}

load_average_create() {
	# create a chart with 3 dimensions
cat <<EOF
CHART system.load '' "System Load Average" "load" load system.load line $((load_priority + 1)) $load_average_update_every
DIMENSION load1 '1 min' absolute 1 100
DIMENSION load5 '5 mins' absolute 1 100
DIMENSION load15 '15 mins' absolute 1 100
EOF

	return 0
}

load_average_update() {
	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	# here we parse the system average load
	# it is decimal (with 2 decimal digits), so we remove the dot and
	# at the definition we have divisor = 100, to have the graph show the right value
	loadavg="`cat /proc/loadavg | sed -e "s/\.//g"`"
	load1=`echo $loadavg | cut -d ' ' -f 1`
	load5=`echo $loadavg | cut -d ' ' -f 2`
	load15=`echo $loadavg | cut -d ' ' -f 3`

	# write the result of the work.
	cat <<VALUESEOF
BEGIN system.load
SET load1 = $load1
SET load5 = $load5
SET load15 = $load15
END
VALUESEOF

	return 0
}

