# shellcheck shell=bash
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0+

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#

# if this chart is called X.chart.sh, then all functions and global variables
# must start with X_

cpufreq_sys_dir="${NETDATA_HOST_PREFIX}/sys/devices"
cpufreq_sys_depth=10
cpufreq_source_update=1

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
cpufreq_update_every=
cpufreq_priority=10000

cpufreq_find_all_files() {
	find "$1" -maxdepth $cpufreq_sys_depth -name scaling_cur_freq 2>/dev/null
}

# _check is called once, to find out if this chart should be enabled or not
cpufreq_check() {

	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	[ -z "$( cpufreq_find_all_files "$cpufreq_sys_dir" )" ] && return 1
	return 0
}

# _create is called once, to create the charts
cpufreq_create() {
	local dir file id i

	# we create a script with the source of the
	# cpufreq_update() function
	# - the highest speed we can achieve -
	[ $cpufreq_source_update -eq 1 ] && echo >"$TMP_DIR/cpufreq.sh" "cpufreq_update() {"

	echo "CHART cpu.cpufreq '' 'CPU Clock' 'MHz' 'cpufreq' '' line $((cpufreq_priority + 1)) $cpufreq_update_every"
	echo >>"$TMP_DIR/cpufreq.sh" "echo \"BEGIN cpu.cpufreq \$1\""

	i=0
	for file in $( cpufreq_find_all_files "$cpufreq_sys_dir" | sort -u )
	do
		i=$(( i + 1 ))
		dir=$( dirname "$file" )
		cpu=

		[ -f "$dir/affected_cpus" ] && cpu=$( cat "$dir/affected_cpus" )
		[ -z "$cpu" ] && cpu="$i.a"

		id="$( fixid "cpu$cpu" )"

		debug "file='$file', dir='$dir', cpu='$cpu', id='$id'"

		echo "DIMENSION $id '$id' absolute 1 1000"
		echo >>"$TMP_DIR/cpufreq.sh" "echo \"SET $id = \"\$(< $file )"
	done
	echo >>"$TMP_DIR/cpufreq.sh" "echo END"

	[ $cpufreq_source_update -eq 1 ] && echo >>"$TMP_DIR/cpufreq.sh" "}"

	# ok, load the function cpufreq_update() we created
        # shellcheck disable=SC1090
	[ $cpufreq_source_update -eq 1 ] && . "$TMP_DIR/cpufreq.sh"

	return 0
}

# _update is called continuously, to collect the values
cpufreq_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT
        # shellcheck disable=SC1090
	[ $cpufreq_source_update -eq 0 ] && . "$TMP_DIR/cpufreq.sh" "$1"

	return 0
}

