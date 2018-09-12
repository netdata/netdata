# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0+

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#

squid_host=
squid_port=
squid_url=
squid_timeout=2
squid_update_every=2
squid_priority=60000

squid_get_stats_internal() {
	local host="$1" port="$2" url="$3"
	run squidclient -h $host -p $port $url
}

squid_get_stats() {
	squid_get_stats_internal "$squid_host" "$squid_port" "$squid_url"
}

squid_autodetect() {
	local host="127.0.0.1" port url x

	for port in 3128 8080
	do
		for url in "cache_object://$host:$port/counters" "/squid-internal-mgr/counters"
		do
			x=$(squid_get_stats_internal "$host" "$port" "$url" | grep client_http.requests)
			if [ ! -z "$x" ]
				then
				squid_host="$host"
				squid_port="$port"
				squid_url="$url"
				debug "found squid at '$host:$port' with url '$url'"
				return 0
			fi
		done
	done

	error "cannot find squid running in localhost. Please set squid_url='url' and squid_host='IP' and squid_port='PORT' in $confd/squid.conf"
	return 1
}

squid_check() {
	require_cmd squidclient || return 1
	require_cmd sed || return 1
	require_cmd egrep || return 1

	if [ -z "$squid_host" -o -z "$squid_port" -o -z "$squid_url" ]
		then
		squid_autodetect || return 1
	fi

	# check once if the url works
	local x="$(squid_get_stats | grep client_http.requests)"
	if [ ! $? -eq 0 -o -z "$x" ]
	then
		error "cannot fetch URL '$squid_url' by connecting to $squid_host:$squid_port. Please set squid_url='url' and squid_host='host' and squid_port='port' in $confd/squid.conf"
		return 1
	fi

	return 0
}

squid_create() {
	# create the charts
	cat <<EOF
CHART squid_local.clients_net '' "Squid Client Bandwidth" "kilobits / sec" clients squid.clients.net area $((squid_priority + 1)) $squid_update_every
DIMENSION client_http_kbytes_in in incremental 8 1
DIMENSION client_http_kbytes_out out incremental -8 1
DIMENSION client_http_hit_kbytes_out hits incremental -8 1

CHART squid_local.clients_requests '' "Squid Client Requests" "requests / sec" clients squid.clients.requests line $((squid_priority + 3)) $squid_update_every
DIMENSION client_http_requests requests incremental 1 1
DIMENSION client_http_hits hits incremental 1 1
DIMENSION client_http_errors errors incremental -1 1

CHART squid_local.servers_net '' "Squid Server Bandwidth" "kilobits / sec" servers squid.servers.net area $((squid_priority + 2)) $squid_update_every
DIMENSION server_all_kbytes_in in incremental 8 1
DIMENSION server_all_kbytes_out out incremental -8 1

CHART squid_local.servers_requests '' "Squid Server Requests" "requests / sec" servers squid.servers.requests line $((squid_priority + 4)) $squid_update_every
DIMENSION server_all_requests requests incremental 1 1
DIMENSION server_all_errors errors incremental -1 1
EOF

	return 0
}


squid_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	# 1. get the counters page from squid
	# 2. sed to remove spaces; replace . with _; remove spaces around =; prepend each line with: local squid_
	# 3. egrep lines starting with:
	#    local squid_client_http_ then one or more of these a-z 0-9 _ then = and one of more of 0-9
	#    local squid_server_all_ then one or more of these a-z 0-9 _ then = and one of more of 0-9
	# 4. then execute this as a script with the eval
	#
	# be very carefull with eval:
	# prepare the script and always grep at the end the lines that are usefull, so that
	# even if something goes wrong, no other code can be executed

	eval "$(squid_get_stats |\
		 sed -e "s/ \+/ /g" -e "s/\./_/g" -e "s/^\([a-z0-9_]\+\) *= *\([0-9]\+\)$/local squid_\1=\2/g" |\
		egrep "^local squid_(client_http|server_all)_[a-z0-9_]+=[0-9]+$")"

	# write the result of the work.
	cat <<VALUESEOF
BEGIN squid_local.clients_net $1
SET client_http_kbytes_in = $squid_client_http_kbytes_in
SET client_http_kbytes_out = $squid_client_http_kbytes_out
SET client_http_hit_kbytes_out = $squid_client_http_hit_kbytes_out
END

BEGIN squid_local.clients_requests $1
SET client_http_requests = $squid_client_http_requests
SET client_http_hits = $squid_client_http_hits
SET client_http_errors = $squid_client_http_errors
END

BEGIN squid_local.servers_net $1
SET server_all_kbytes_in = $squid_server_all_kbytes_in
SET server_all_kbytes_out = $squid_server_all_kbytes_out
END

BEGIN squid_local.servers_requests $1
SET server_all_requests = $squid_server_all_requests
SET server_all_errors = $squid_server_all_errors
END
VALUESEOF

	return 0
}
