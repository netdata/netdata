#!/bin/sh

airsearches_url="https://services.viva.gr/vivatravelwcf_v2/airsabre/webtesting/searchcounters.ashx"
airsearches_cmds=""
airsearches_update_every=60

airsearches_get() {
	wget 2>/dev/null -O - "$airsearches_url" |\
		sed -e "s|<br />|\n|g" -e "s|: |=|g" -e "s| \+|_|g" |\
		tr "[A-Z]\.\!@#\$%^&*()_+\-" "[a-z]_____________" |\
		egrep "^[a-z0-9_]+=[0-9]+$" |\
		sort -u
}

airsearches_check() {
	# check once if the url works
	wget 2>/dev/null -O /dev/null "$airsearches_url"
	if [ ! $? -eq 0 ]
	then
		echo >&2 "airsearches: cannot fetch the url: $airsearches_url. Please set airsearches_url='url' in $confd/airsearches.conf"
		return 1
	fi

	if [ -z "$airsearches_cmds" ]
	then
		airsearches_cmds="`airsearches_get | cut -d '=' -f 1`"
		echo
	fi
	if [ -z "$airsearches_cmds" ]
	then
		echo >&2 "airsearches: cannot find command list automatically. Please set airsearches_cmds='...' in $confd/airsearches.conf"
		return 1
	fi
	return 0
}

airsearches_create() {
	# create the charts
	local x=
	echo "CHART airsearches.affiliates '' 'Air Searches per affiliate' 'requests / $airsearches_update_every secs' airsearches '' stacked 20000 $airsearches_update_every"
	for x in $airsearches_cmds
	do
		echo "DIMENSION $x '' incremental 1 1"
	done

	return 0
}

airsearches_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	# get the values from airsearches
	eval "`airsearches_get | sed "s/^/airsearches_/g"`"

	# write the result of the work.
	local x=

	echo "BEGIN airsearches.affiliates $1"
	for x in $airsearches_cmds
	do
		eval "v=\$airsearches_$x"
		echo "SET $x = $v"
	done
	echo "END"

	airsearches_dt=0

	return 0
}
