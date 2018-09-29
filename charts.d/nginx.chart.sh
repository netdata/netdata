# shellcheck shell=bash
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0+

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#

# if this chart is called X.chart.sh, then all functions and global variables
# must start with X_

nginx_url="http://127.0.0.1:80/stub_status"
nginx_curl_opts=""

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
nginx_update_every=
nginx_priority=60000

declare -a nginx_response=()
nginx_active_connections=0
nginx_accepts=0
nginx_handled=0
nginx_requests=0
nginx_reading=0
nginx_writing=0
nginx_waiting=0
nginx_get() {
	# shellcheck disable=SC2207
	nginx_response=($(run curl -Ss ${nginx_curl_opts} "${nginx_url}"))
	# shellcheck disable=SC2181
	if [ $? -ne 0 ] || [ "${#nginx_response[@]}" -eq 0 ]; then return 1; fi

	if [ "${nginx_response[0]}" != "Active" ] ||\
	   [ "${nginx_response[1]}" != "connections:" ] ||\
	   [ "${nginx_response[3]}" != "server" ] ||\
	   [ "${nginx_response[4]}" != "accepts" ] ||\
	   [ "${nginx_response[5]}" != "handled" ] ||\
	   [ "${nginx_response[6]}" != "requests" ] ||\
	   [ "${nginx_response[10]}" != "Reading:" ] ||\
	   [ "${nginx_response[12]}" != "Writing:" ] ||\
	   [ "${nginx_response[14]}" != "Waiting:" ]
	then
		error "Invalid response from nginx server: ${nginx_response[*]}"
		return 1
	fi

	nginx_active_connections="${nginx_response[2]}"
	nginx_accepts="${nginx_response[7]}"
	nginx_handled="${nginx_response[8]}"
	nginx_requests="${nginx_response[9]}"
	nginx_reading="${nginx_response[11]}"
	nginx_writing="${nginx_response[13]}"
	nginx_waiting="${nginx_response[15]}"

	if [ -z "${nginx_active_connections}" ] ||\
	   [ -z "${nginx_accepts}" ] ||\
	   [ -z "${nginx_handled}" ] ||\
	   [ -z "${nginx_requests}" ] ||\
	   [ -z "${nginx_reading}" ] ||\
	   [ -z "${nginx_writing}" ] ||\
	   [ -z "${nginx_waiting}" ]
	then
		error "empty values got from nginx server: ${nginx_response[*]}"
		return 1
	fi

	return 0
}

# _check is called once, to find out if this chart should be enabled or not
nginx_check() {

	nginx_get
	# shellcheck disable=2181
	if [ $? -ne 0 ]
	then
		# shellcheck disable=SC2154
		error "cannot find stub_status on URL '${nginx_url}'. Please set nginx_url='http://nginx.server/stub_status' in $confd/nginx.conf"
		return 1
	fi

	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	return 0
}

# _create is called once, to create the charts
nginx_create() {
	cat <<EOF
CHART nginx_local.connections '' "nginx Active Connections" "connections" nginx nginx.connections line $((nginx_priority + 1)) $nginx_update_every
DIMENSION active '' absolute 1 1

CHART nginx_local.requests '' "nginx Requests" "requests/s" nginx nginx.requests line $((nginx_priority + 2)) $nginx_update_every
DIMENSION requests '' incremental 1 1

CHART nginx_local.connections_status '' "nginx Active Connections by Status" "connections" nginx nginx.connections.status line $((nginx_priority + 3)) $nginx_update_every
DIMENSION reading '' absolute 1 1
DIMENSION writing '' absolute 1 1
DIMENSION waiting idle absolute 1 1

CHART nginx_local.connect_rate '' "nginx Connections Rate" "connections/s" nginx nginx.connections.rate line $((nginx_priority + 4)) $nginx_update_every
DIMENSION accepts accepted incremental 1 1
DIMENSION handled '' incremental 1 1
EOF

	return 0
}

# _update is called continuously, to collect the values
nginx_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	nginx_get || return 1

	# write the result of the work.
	cat <<VALUESEOF
BEGIN nginx_local.connections $1
SET active = $((nginx_active_connections))
END
BEGIN nginx_local.requests $1
SET requests = $((nginx_requests))
END
BEGIN nginx_local.connections_status $1
SET reading = $((nginx_reading))
SET writing = $((nginx_writing))
SET waiting = $((nginx_waiting))
END
BEGIN nginx_local.connect_rate $1
SET accepts = $((nginx_accepts))
SET handled = $((nginx_handled))
END
VALUESEOF

	return 0
}
