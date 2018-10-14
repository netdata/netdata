# shellcheck shell=bash disable=SC2154,SC1072,SC1073,SC2009,SC2162,SC2006,SC2002,SC2086,SC1117
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0-or-later

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#
# THIS PLUGIN IS OBSOLETE
# USE apps.plugin INSTEAD

# a space separated list of command to monitor
cpu_apps_apps=

# these are required for computing memory in bytes and cpu in seconds
#cpu_apps_pagesize="`getconf PAGESIZE`"
cpu_apps_clockticks="$(getconf CLK_TCK)"

cpu_apps_update_every=60

cpu_apps_check() {
	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	if [ -z "$cpu_apps_apps" ]
	then
		error "manual configuration required: please set cpu_apps_apps='command1 command2 ...' in $confd/cpu_apps_apps.conf"
		return 1
	fi
	return 0
}

cpu_apps_bc_finalze=

cpu_apps_create() {

	echo "CHART chartsd_apps.cpu '' 'Apps CPU' 'milliseconds / $cpu_apps_update_every sec' apps apps stacked 20001 $cpu_apps_update_every"

	local x=
	for x in $cpu_apps_apps
	do
		echo "DIMENSION $x $x incremental 1000 $cpu_apps_clockticks"

		# this string is needed later in the update() function
		# to finalize the instructions for the bc command
		cpu_apps_bc_finalze="$cpu_apps_bc_finalze \"SET $x = \"; $x;"
	done
	return 0
}

cpu_apps_update() {
	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	echo "BEGIN chartsd_apps.cpu"
	ps -o pid,comm -C "$cpu_apps_apps" |\
		grep -v "COMMAND" |\
		(
			while read pid name
			do
				echo "$name+=`cat /proc/$pid/stat | cut -d ' ' -f 14-15`"
			done
		) |\
		(	sed -e "s/ \+/ /g" -e "s/ /+/g";
			echo "$cpu_apps_bc_finalze"
		) | bc
	echo "END"

	return 0
}
