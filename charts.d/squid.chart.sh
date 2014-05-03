#!/bin/sh

squid_url="http://127.0.0.1:8080/squid-internal-mgr/counters"

squid_check() {
	# check once if the url works
	wget 2>/dev/null -O /dev/null "$squid_url"
	if [ ! $? -eq 0 ]
	then
		echo >&2 "squid: cannot fetch the url: $squid_url. Please set squid_url='url' in $confd/squid.conf"
		return 1
	fi

	return 0
}

squid_create() {
	# create the charts
	cat <<EOF
CHART squid.client_bandwidth '' "Squid Client Bandwidth" "kilobits/s" squid squid area 1 $update_every
DIMENSION client_http_kbytes_in in incremental 8 1
DIMENSION client_http_kbytes_out out incremental -8 1
DIMENSION client_http_hit_kbytes_out hits incremental -8 1

CHART squid.client_requests '' "Squid Client Requests" "requests/s" squid squid line 3 $update_every
DIMENSION client_http_requests requests incremental 1 1
DIMENSION client_http_hits hits incremental 1 1
DIMENSION client_http_errors errors incremental -1 1

CHART squid.server_bandwidth '' "Squid Server Bandwidth" "kilobits/s" squid squid area 2 $update_every
DIMENSION server_all_kbytes_in in incremental 8 1
DIMENSION server_all_kbytes_out out incremental -8 1

CHART squid.server_requests '' "Squid Server Requests" "requests/s" squid squid line 4 $update_every
DIMENSION server_all_requests requests incremental 1 1
DIMENSION server_all_errors errors incremental -1 1
EOF
	
	return 0
}


squid_update() {
	# do all the work to collect / calculate the values
	# for each dimension

	# get the values from squid
	eval `wget 2>/dev/null -O - "$squid_url" | sed -e "s/\./_/g" -e "s/ = /=/g" | egrep "(^client_http_|^server_all_)"`

	# write the result of the work.
	cat <<VALUESEOF
BEGIN squid.client_bandwidth
SET client_http_kbytes_in = $client_http_kbytes_in
SET client_http_kbytes_out = $client_http_kbytes_out
SET client_http_hit_kbytes_out = $client_http_hit_kbytes_out
END

BEGIN squid.client_requests
SET client_http_requests = $client_http_requests
SET client_http_hits = $client_http_hits
SET client_http_errors = $client_http_errors
END

BEGIN squid.server_bandwidth
SET server_all_kbytes_in = $server_all_kbytes_in
SET server_all_kbytes_out = $server_all_kbytes_out
END

BEGIN squid.server_requests
SET server_all_requests = $server_all_requests
SET server_all_errors = $server_all_errors
END
VALUESEOF

	return 0
}

