#!/bin/sh

squid_url="http://127.0.0.1:8080/squid-internal-mgr/counters"
squid_update_every=5

squid_check() {
	# check once if the url works
	wget 2>/dev/null -O /dev/null "$squid_url"
	if [ ! $? -eq 0 ]
	then
		echo >&2 "squid: cannot fetch the counters url: $squid_url. Please set squid_url='url' in $confd/squid.conf"
		return 1
	fi

	return 0
}

squid_create() {
	# create the charts
	cat <<EOF
CHART squid.clients_net '' "Squid Client Bandwidth" "kilobits / $squid_update_every sec" squid '' area 20001 $squid_update_every
DIMENSION client_http_kbytes_in in incremental 8 $((1 * squid_update_every))
DIMENSION client_http_kbytes_out out incremental -8 $((1 * squid_update_every))
DIMENSION client_http_hit_kbytes_out hits incremental -8 $((1 * squid_update_every))

CHART squid.clients_requests '' "Squid Client Requests" "requests / $squid_update_every sec" squid '' line 20003 $squid_update_every
DIMENSION client_http_requests requests incremental 1 $((1 * squid_update_every))
DIMENSION client_http_hits hits incremental 1 $((1 * squid_update_every))
DIMENSION client_http_errors errors incremental -1 $((1 * squid_update_every))

CHART squid.servers_net '' "Squid Server Bandwidth" "kilobits / $squid_update_every sec" squid '' area 20002 $squid_update_every
DIMENSION server_all_kbytes_in in incremental 8 $((1 * squid_update_every))
DIMENSION server_all_kbytes_out out incremental -8 $((1 * squid_update_every))

CHART squid.servers_requests '' "Squid Server Requests" "requests / $squid_update_every sec" squid '' line 20004 $squid_update_every
DIMENSION server_all_requests requests incremental 1 $((1 * squid_update_every))
DIMENSION server_all_errors errors incremental -1 $((1 * squid_update_every))
EOF
	
	return 0
}


squid_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	# 1. wget the counters page from squid
	# 2. sed to remove spaces; replace . with _; remove spaces around =; prepend each line with: local squid_
	# 3. egrep lines starting with:
	#    local squid_client_http_ then one or more of these a-z 0-9 _ then = and one of more of 0-9
	#    local squid_server_all_ then one or more of these a-z 0-9 _ then = and one of more of 0-9
	# 4. then execute this as a script with the eval
	#
	# be very carefull with eval:
	# prepare the script and always grep at the end the lines that are usefull, so that
	# even if something goes wrong, no other code can be executed

	eval "`wget 2>/dev/null -O - "$squid_url" |\
		sed -e "s/ \+/ /g" -e "s/\./_/g" -e "s/ = /=/g" -e "s/^/local squid_/g" |\
		egrep "^local squid_(client_http|server_all)_[a-z0-9_]+=[0-9]+$"`"

	# write the result of the work.
	cat <<VALUESEOF
BEGIN squid.clients_net $1
SET client_http_kbytes_in = $squid_client_http_kbytes_in
SET client_http_kbytes_out = $squid_client_http_kbytes_out
SET client_http_hit_kbytes_out = $squid_client_http_hit_kbytes_out
END

BEGIN squid.clients_requests $1
SET client_http_requests = $squid_client_http_requests
SET client_http_hits = $squid_client_http_hits
SET client_http_errors = $squid_client_http_errors
END

BEGIN squid.servers_net $1
SET server_all_kbytes_in = $squid_server_all_kbytes_in
SET server_all_kbytes_out = $squid_server_all_kbytes_out
END

BEGIN squid.servers_requests $1
SET server_all_requests = $squid_server_all_requests
SET server_all_errors = $squid_server_all_errors
END
VALUESEOF

	return 0
}

