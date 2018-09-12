# shellcheck shell=bash
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0+

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#

mem_apps_apps=

# these are required for computing memory in bytes and cpu in seconds
#mem_apps_pagesize="`getconf PAGESIZE`"
#mem_apps_clockticks="`getconf CLK_TCK`"

mem_apps_update_every=

mem_apps_check() {
	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	if [ -z "$mem_apps_apps" ]
	then
		error "manual configuration required: please set mem_apps_apps='command1 command2 ...' in $confd/mem_apps_apps.conf"
		return 1
	fi
	return 0
}

mem_apps_bc_finalze=

mem_apps_create() {

	echo "CHART chartsd_apps.mem '' 'Apps Memory' MB apps apps.mem stacked 20000 $mem_apps_update_every"

	local x=
	for x in $mem_apps_apps
	do
		echo "DIMENSION $x $x absolute 1 1024"

		# this string is needed later in the update() function
		# to finalize the instructions for the bc command
		mem_apps_bc_finalze="$mem_apps_bc_finalze \"SET $x = \"; $x;"
	done
	return 0
}

mem_apps_update() {
	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	echo "BEGIN chartsd_apps.mem"
	ps -o comm,rss -C "$mem_apps_apps" |\
		grep -v "^COMMAND" |\
		(	sed -e "s/ \+/ /g" -e "s/ /+=/g";
			echo "$mem_apps_bc_finalze"
		) | bc
	echo "END"

	return 0
}
