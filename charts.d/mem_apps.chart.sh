#!/bin/sh

mem_apps_apps="netdata asterisk squid apache2 mysqld dovecot cupsd sshd named clamd smbd"

# these are required for computing memory in bytes and cpu in seconds
#mem_apps_pagesize="`getconf PAGESIZE`"
#mem_apps_clockticks="`getconf CLK_TCK`"

mem_apps_check() {
	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	if [ -z "$mem_apps_apps" ]
	then
		echo >&2 "mem_apps: Please set mem_apps_apps='command1 command2 ...' in $confd/mem_apps_apps.conf"
		return 1
	fi
	return 0
}

mem_apps_bc_finalze=

mem_apps_create() {

	cat <<EOF1
CHART system.apps '' "Apps Memory" "MB" "mem" "mem" stacked 20000 $update_every
EOF1

	local x=
	for x in $mem_apps_apps
	do

cat <<EOF1
DIMENSION $x $x absolute 1 1024
EOF1
		# this string is needed later in the update() function
		# to finalize the instructions for the bc command
		mem_apps_bc_finalze="$mem_apps_bc_finalze \"SET $x = \"; $x;"
	done
	return 0
}

mem_apps_egrep="(`echo "$mem_apps_apps" | sed -e "s/^ \+//g" -e "s/ \+$//g" -e "s/ /|/g"`)"
mem_apps_update() {
	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	echo "BEGIN system.apps"
	ps -e -o comm,rss |\
		egrep "^$mem_apps_egrep " |\
		(	sed -e "s/ \+/ /g" -e "s/ /+=/g";
			echo "$mem_apps_bc_finalze"
		) | bc
	echo "END"

	return 0
}

