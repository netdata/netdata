#!/bin/sh

airsearches_url=
airsearches_cmds=
airsearches_update_every=15

airsearches_get() {
	wget 2>/dev/null -O - "$airsearches_url" |\
		sed -e "s|<br />|\n|g" -e "s|: |=|g" -e "s| \+|_|g" -e "s/^/airsearches_/g" |\
		tr "[A-Z]\.\!@#\$%^&*()_+\-" "[a-z]_____________" |\
		egrep "^airsearches_[a-z0-9_]+=[0-9]+$"
}

airsearches_check() {
	# make sure we have all the commands we need
	require_cmd wget || return 1

	# make sure we are configured
	if [ -z "$airsearches_url" ]
		then
		echo >&2 "$PROGRAM_NAME: airsearches: not configured. Please set airsearches_url='url' in $confd/airsearches.conf"
		return 1
	fi

	# check once if the url works
	wget 2>/dev/null -O /dev/null "$airsearches_url"
	if [ ! $? -eq 0 ]
	then
		echo >&2 "$PROGRAM_NAME: airsearches: cannot fetch the url: $airsearches_url. Please set airsearches_url='url' in $confd/airsearches.conf"
		return 1
	fi

	# if the admin did not give any commands
	# find the available ones
	if [ -z "$airsearches_cmds" ]
	then
		airsearches_cmds="$(airsearches_get | cut -d '=' -f 1 | sed "s/^airsearches_//g" | sort -u)"
		echo
	fi

	# did we find any commands?
	if [ -z "$airsearches_cmds" ]
	then
		echo >&2 "$PROGRAM_NAME: airsearches: cannot find command list automatically. Please set airsearches_cmds='...' in $confd/airsearches.conf"
		return 1
	fi

	# ok we can do it
	return 0
}

airsearches_create() {
	[ -z "$airsearches_cmds" ] && return 1

	# create the charts
	local x=
	echo "CHART airsearches.affiliates '' 'Air Searches per affiliate' 'requests / min' airsearches '' stacked 20000 $airsearches_update_every"
	for x in $airsearches_cmds
	do
		echo "DIMENSION $x '' incremental 60 1"
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
	eval "$(airsearches_get)"

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
