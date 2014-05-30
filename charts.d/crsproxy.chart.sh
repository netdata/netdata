#!/bin/sh

crsproxy_url="http://127.0.0.1:7999/counters?"
crsproxy_cmds=""
crsproxy_update_every=60

crsproxy_check() {
	# check once if the url works
	wget 2>/dev/null -O /dev/null "$crsproxy_url"
	if [ ! $? -eq 0 ]
	then
		echo >&2 "crsproxy: cannot fetch the url: $crsproxy_url. Please set crsproxy_url='url' in $confd/crsproxy.conf"
		return 1
	fi

	if [ -z "$crsproxy_cmds" ]
	then
		crsproxy_cmds="`wget 2>/dev/null -O - "$crsproxy_url" | egrep "^cmd_[a-zA-Z][a-zA-Z0-9]_[a-z_]+ *= *[0-9]+$" | cut -d '_' -f 2 | sort -u`"
		echo
	fi
	if [ -z "$crsproxy_cmds" ]
	then
		echo >&2 "crsproxy: cannot find command list automatically. Please set crsproxy_cmds='...' in $confd/crsproxy.conf"
		return 1
	fi
	return 0
}

crsproxy_create() {
	# create the charts
	cat <<EOF
CHART crsproxy.connected '' "CRS Proxy Connected Clients" "clients" crsproxy '' line 20000 $crsproxy_update_every
DIMENSION web '' absolute 1 1
DIMENSION native '' absolute 1 1
DIMENSION virtual '' absolute 1 1
CHART crsproxy.requests '' "CRS Proxy Requests Rate" "requests / $crsproxy_update_every sec" crsproxy '' area 20001 $crsproxy_update_every
DIMENSION web '' incremental 1 1
DIMENSION native '' incremental -1 1
CHART crsproxy.clients '' "CRS Proxy Clients Rate" "clients / $crsproxy_update_every sec" crsproxy '' area 20010 $crsproxy_update_every
DIMENSION web '' incremental 1 1
DIMENSION native '' incremental -1 1
DIMENSION virtual '' incremental 1 1
CHART crsproxy.replies '' "CRS Replies Rate" "replies / $crsproxy_update_every sec" crsproxy '' area 20020 $crsproxy_update_every
DIMENSION ok '' incremental 1 1
DIMENSION failed '' incremental -1 1
CHART crsproxy.bconnections '' "Back-End Connections Rate" "connections / $crsproxy_update_every sec" crsproxy '' area 20030 $crsproxy_update_every
DIMENSION ok '' incremental 1 1
DIMENSION failed '' incremental -1 1
EOF

	local x=
	echo "CHART crsproxy.commands '' 'CRS Commands Requests' 'requests / $crsproxy_update_every sec' crsproxy '' stacked 20100 $crsproxy_update_every"
	for x in $crsproxy_cmds
	do
		echo "DIMENSION $x '' incremental 1 1"
	done

	echo "CHART crsproxy.commands_failed '' 'CRS Failed Commands' 'replies / $crsproxy_update_every sec' crsproxy '' stacked 20110 $crsproxy_update_every"
	for x in $crsproxy_cmds
	do
		echo "DIMENSION $x '' incremental 1 1"
	done

	return 0
}


crsproxy_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	# get the values from crsproxy
	eval "`wget 2>/dev/null -O - "$crsproxy_url" |\
		sed -e "s/ \+/ /g" -e "s/\./_/g" -e "s/ = /=/g" -e "s/^/crsproxy_/g" |\
		egrep "^crsproxy_[a-zA-Z][a-zA-Z0-9_]*=[0-9]+$"`"


	# write the result of the work.
	cat <<VALUESEOF
BEGIN crsproxy.connected $1
SET web = $((crsproxy_web_clients_opened - crsproxy_web_clients_closed))
SET native = $((crsproxy_crs_clients_opened - crsproxy_crs_clients_closed))
SET virtual = $((crsproxy_virtual_clients_opened - crsproxy_virtual_clients_closed))
END
BEGIN crsproxy.requests $1
SET web = $crsproxy_web_requests
SET native = $crsproxy_native_requests
END
BEGIN crsproxy.clients $1
SET web = $crsproxy_web_clients_opened
SET native = $crsproxy_crs_clients_opened
SET virtual = $crsproxy_virtual_clients_opened
END
BEGIN crsproxy.replies $1
SET ok = $crsproxy_replies_success
SET failed = $crsproxy_replies_error
END
BEGIN crsproxy.bconnections $1
SET ok = $crsproxy_connections_nonblocking_established
SET failed = $crsproxy_connections_nonblocking_failed
END
VALUESEOF

	local native_requests="_native_requests"
	local web_requests="_web_requests"
	local replies_error="_replies_error"
	local x=

	echo "BEGIN crsproxy.commands $1"
	for x in $crsproxy_cmds
	do
		eval "v=\$(( crsproxy_cmd_$x$native_requests + crsproxy_cmd_$x$web_requests ))"
		echo "SET $x = $v"
	done
	echo "END"

	echo "BEGIN crsproxy.commands_failed $1"
	for x in $crsproxy_cmds
	do
		eval "v=\$crsproxy_cmd_$x$replies_error"
		echo "SET $x = $v"
	done
	echo "END"

	return 0
}
