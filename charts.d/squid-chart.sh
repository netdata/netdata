#!/bin/sh

url="http://127.0.0.1:3128/squid-internal-mgr/counters"

# report our PID back to netdata
# this is required for netdata to kill this process when it exits
echo "MYPID $$"

# default sleep function
loopsleepms() {
	sleep $1
}
# if found and included, this file overwrites loopsleepms()
# with a high resolution timer function for precise looping.
. "`dirname $0`/loopsleepms.sh.inc"

# netdata passes the requested update frequency as the first argument
update_every=$1
update_every=$(( update_every + 1 - 1))	# makes sure it is a number
test $update_every -eq 0 && update_every=1 # if it is zero, make it 1

# we don't allow more than 1 request every $min seconds
min=1

# check if there is a config for us - if there is, bring it in
if [ -f "$0.conf" ]
then
	. "$0.conf"
fi

# make sure we respect the $min update frequency
test $update_every -lt $min && update_every=$min

# check once if the url works
wget 2>/dev/null -O /dev/null "$url"
if [ ! $? -eq 0 ]
then
	# it does not work - disable the plugin
	echo "DISABLE"
	exit 1
fi

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

# You can create more charts if you like.
# Just add more chart definitions.

# work forever
while [ 1 ]
do
	# do all the work to collect / calculate the values
	# for each dimension

	# get the values from squid
	eval `wget 2>/dev/null -O - "$url" | sed -e "s/\./_/g" -e "s/ = /=/g" | egrep "(^client_http_|^server_all_)"`

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

	# wait the time you are required to
	loopsleepms $update_every
done
