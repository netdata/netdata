#!/bin/sh

# the URL to download apache status info
apache_url="http://127.0.0.1:80/server-status?auto"

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
apache_update_every=

# let netdata calculate averages (0)
# or use apache calculated averages (1)
apache_averages=0

# convert apache floating point values
# to integer using this multiplier
# this only affects precision - the values
# will be in the proper units
apache_decimal_detail=1000000

declare -a apache_response=()
apache_accesses=0
apache_kbytes=0
apache_cpuload=0
apache_uptime=0
apache_reqpersec=0
apache_bytespersec=0
apache_bytesperreq=0
apache_busyworkers=0
apache_idleworkers=0
apache_scoreboard=

apache_get() {
	apache_response=($(curl -s "${apache_url}"))
	[ $? -ne 0 -o "${#apache_response[@]}" -eq 0 ] && return 1

	if [ "${apache_response[0]}" != "Total" \
		 -o "${apache_response[1]}" != "Accesses:" \
		 -o "${apache_response[3]}" != "Total" \
		 -o "${apache_response[4]}" != "kBytes:" \
		 -o "${apache_response[6]}" != "CPULoad:" \
		 -o "${apache_response[8]}" != "Uptime:" \
		 -o "${apache_response[10]}" != "ReqPerSec:" \
		 -o "${apache_response[12]}" != "BytesPerSec:" \
		 -o "${apache_response[14]}" != "BytesPerReq:" \
		 -o "${apache_response[16]}" != "BusyWorkers:" \
		 -o "${apache_response[18]}" != "IdleWorkers:" \
		 -o "${apache_response[20]}" != "Scoreboard:" \
	   ]
		then
		echo >&2 "apache: Invalid response from apache server: ${apache_response[*]}"
		return 1
	fi

	apache_accesses="${apache_response[2]}"
	apache_kbytes="${apache_response[5]}"
	
	# float2int "${apache_response[7]}" ${apache_decimal_detail}
	# apache_cpuload=${FLOAT2INT_RESULT}
	
	# apache_uptime="${apache_response[9]}"
	
	if [ ${apache_averages} -eq 1 ]
		then
		float2int "${apache_response[11]}" ${apache_decimal_detail}
		apache_reqpersec=${FLOAT2INT_RESULT}

		float2int "${apache_response[13]}" ${apache_decimal_detail}
		apache_bytespersec=${FLOAT2INT_RESULT}
	fi

	float2int "${apache_response[15]}" ${apache_decimal_detail}
	apache_bytesperreq=${FLOAT2INT_RESULT}

	apache_busyworkers="${apache_response[17]}"
	apache_idleworkers="${apache_response[19]}"
	apache_scoreboard="${apache_response[21]}"

	if [ -z "${apache_accesses}" \
		-o -z "${apache_kbytes}" \
		-o -z "${apache_cpuload}" \
		-o -z "${apache_uptime}" \
		-o -z "${apache_reqpersec}" \
		-o -z "${apache_bytespersec}" \
		-o -z "${apache_bytesperreq}" \
		-o -z "${apache_busyworkers}" \
		-o -z "${apache_idleworkers}" \
		-o -z "${apache_scoreboard}" \
		]
		then
		echo >&2 "apache: empty values got from apache server: ${apache_response[*]}"
		return 1
	fi

	return 0
}

# _check is called once, to find out if this chart should be enabled or not
apache_check() {

	apache_get
	if [ $? -ne 0 ]
		then
		echo >&2 "apache: cannot find stub_status on URL '${apache_url}'. Please set apache_url='http://apache.server:80/server-status?auto' in $confd/apache.conf"
		return 1
	fi

	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	return 0
}

# _create is called once, to create the charts
apache_create() {
	cat <<EOF
CHART apache.bytesperreq '' "apache Average Response Size (lifetime)" "bytes/request" apache apache area 16005 $apache_update_every
DIMENSION size '' absolute 1 ${apache_decimal_detail}
CHART apache.workers '' "apache Workers" "workers" apache apache stacked 16006 $apache_update_every
DIMENSION idle '' absolute 1 1
DIMENSION busy '' absolute 1 1
EOF

	if [ ${apache_averages} -eq 1 ]
		then
		# apache calculated averages
		cat <<EOF2
CHART apache.requests '' "apache Requests" "requests/s" apache apache line 16001 $apache_update_every
DIMENSION requests '' absolute 1 ${apache_decimal_detail}
CHART apache.net '' "apache Bandwidth" "kilobits/s" apache apache area 16002 $apache_update_every
DIMENSION sent '' absolute 8 $[apache_decimal_detail * 1000]
EOF2
	else
		# netdata calculated averages
		cat <<EOF3
CHART apache.requests '' "apache Requests" "requests/s" apache apache line 16001 $apache_update_every
DIMENSION requests '' incremental 1 1
CHART apache.net '' "apache Bandwidth" "kilobits/s" apache apache area 16002 $apache_update_every
DIMENSION sent '' incremental 8 1
EOF3
	fi

	return 0
}

# _update is called continiously, to collect the values
apache_update() {
	local reqs net
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	apache_get || return 1

	if [ ${apache_averages} -eq 1 ]
		then
		reqs=${apache_reqpersec}
		net=${apache_bytespersec}
	else
		reqs=${apache_accesses}
		net=${apache_kbytes}
	fi

	# write the result of the work.
	cat <<VALUESEOF
BEGIN apache.requests $1
SET requests = $[reqs]
END
BEGIN apache.net $1
SET sent = $[net]
END
BEGIN apache.bytesperreq $1
SET size = $[apache_bytesperreq]
END
BEGIN apache.workers $1
SET idle = $[apache_idleworkers]
SET busy = $[apache_busyworkers]
END
VALUESEOF

	return 0
}
