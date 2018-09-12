# shellcheck shell=bash
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0+

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#
# contributed by @paulfantom with PR #511

# if this chart is called X.chart.sh, then all functions and global variables
# must start with X_
hddtemp_host="localhost"
hddtemp_port="7634"
declare -A hddtemp_disks=()

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
hddtemp_update_every=3
hddtemp_priority=90000

# _check is called once, to find out if this chart should be enabled or not
hddtemp_check() {
    require_cmd nc || return 1
	run nc $hddtemp_host $hddtemp_port && return 0 || return 1
}

# _create is called once, to create the charts
hddtemp_create() {
	if [ ${#hddtemp_disks[@]} -eq 0 ]; then
		local all
		all=$(nc $hddtemp_host $hddtemp_port )
		unset hddtemp_disks
		# shellcheck disable=SC2190,SC2207
		hddtemp_disks=( $(grep -Po '/dev/[^|]+' <<< "$all" | cut -c 6-) )
	fi
#	local disk_names
#	disk_names=(`sed -e 's/||/\n/g;s/^|//' <<< "$all" | cut -d '|' -f2 | tr ' ' '_'`)

	echo "CHART hddtemp.temperature 'disks_temp' 'temperature' 'Celsius' 'Disks temperature' 'hddtemp.temp' line $((hddtemp_priority)) $hddtemp_update_every"
	for i in $(seq 0 $((${#hddtemp_disks[@]}-1))); do
#		echo "DIMENSION ${hddtemp_disks[i]} ${disk_names[i]} absolute 1 1"
		echo "DIMENSION ${hddtemp_disks[$i]} '' absolute 1 1"
	done
	return 0
}

# _update is called continuously, to collect the values
#hddtemp_last=0
#hddtemp_count=0
hddtemp_update() {
#        local all=( `nc $hddtemp_host $hddtemp_port | sed -e 's/||/\n/g;s/^|//' | cut -d '|' -f3` )
#	local all=( `nc $hddtemp_host $hddtemp_port | awk 'BEGIN { FS="|" };{i=4; while (i <= NF) {print $i+0;i+=5;};}'` )
	OLD_IFS=$IFS
	set -f
	# shellcheck disable=SC2207
	IFS="|" all=( $(nc $hddtemp_host $hddtemp_port 2>/dev/null) )
	set +f
	IFS=$OLD_IFS

	# check if there is some data
	if [ -z "${all[3]}" ]; then
		return 1
	fi

	# write the result of the work.
	echo "BEGIN hddtemp.temperature $1"
	end=${#hddtemp_disks[@]}
	for ((i=0; i<end; i++)); do
		# temperature - this will turn SLP to zero
                t=$(( ${all[ $((i * 5 + 3)) ]} ))
		echo "SET ${hddtemp_disks[$i]} = $t"
	done
	echo "END"

	return 0
}
