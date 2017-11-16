# no need for shebang - this file is loaded from charts.d.plugin

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
# GPL v3+
#

# the URL to download apache status info
apache_url="http://127.0.0.1:80/server-status?auto"
apache_curl_opts=

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
apache_update_every=

apache_priority=60000

# convert apache floating point values
# to integer using this multiplier
# this only affects precision - the values
# will be in the proper units
apache_decimal_detail=1000000

declare -a apache_response=()
apache_accesses=0
apache_kbytes=0
apache_reqpersec=0
apache_bytespersec=0
apache_bytesperreq=0
apache_busyworkers=0
apache_idleworkers=0
apache_connstotal=0
apache_connsasyncwriting=0
apache_connsasynckeepalive=0
apache_connsasyncclosing=0

apache_keys_detected=0
apache_has_conns=0
apache_key_accesses=
apache_key_kbytes=
apache_key_reqpersec=
apache_key_bytespersec=
apache_key_bytesperreq=
apache_key_busyworkers=
apache_key_idleworkers=
apache_key_scoreboard=
apache_key_connstotal=
apache_key_connsasyncwriting=
apache_key_connsasynckeepalive=
apache_key_connsasyncclosing=
apache_detect() {
	local i=0
	for x in "${@}"
	do
		case "${x}" in
			'Total Accesses') 		apache_key_accesses=$((i + 1)) ;;
			'Total kBytes') 		apache_key_kbytes=$((i + 1)) ;;
			'ReqPerSec') 			apache_key_reqpersec=$((i + 1)) ;;
			'BytesPerSec')			apache_key_bytespersec=$((i + 1)) ;;
			'BytesPerReq')			apache_key_bytesperreq=$((i + 1)) ;;
			'BusyWorkers')			apache_key_busyworkers=$((i + 1)) ;;
			'IdleWorkers')			apache_key_idleworkers=$((i + 1));;
			'ConnsTotal')			apache_key_connstotal=$((i + 1)) ;;
			'ConnsAsyncWriting')	apache_key_connsasyncwriting=$((i + 1)) ;;
			'ConnsAsyncKeepAlive')	apache_key_connsasynckeepalive=$((i + 1)) ;;
			'ConnsAsyncClosing')	apache_key_connsasyncclosing=$((i + 1)) ;;
			'Scoreboard')			apache_key_scoreboard=$((i)) ;;
		esac

		i=$((i + 1))
	done

	# we will not check of the Conns*
	# keys, since these are apache 2.4 specific
	[ -z "${apache_key_accesses}"    ] && error "missing 'Total Accesses' from apache server: ${*}" && return 1
	[ -z "${apache_key_kbytes}"      ] && error "missing 'Total kBytes' from apache server: ${*}" && return 1
	[ -z "${apache_key_reqpersec}"   ] && error "missing 'ReqPerSec' from apache server: ${*}" && return 1
	[ -z "${apache_key_bytespersec}" ] && error "missing 'BytesPerSec' from apache server: ${*}" && return 1
	[ -z "${apache_key_bytesperreq}" ] && error "missing 'BytesPerReq' from apache server: ${*}" && return 1
	[ -z "${apache_key_busyworkers}" ] && error "missing 'BusyWorkers' from apache server: ${*}" && return 1
	[ -z "${apache_key_idleworkers}" ] && error "missing 'IdleWorkers' from apache server: ${*}" && return 1
	[ -z "${apache_key_scoreboard}"  ] && error "missing 'Scoreboard' from apache server: ${*}" && return 1

	if [ ! -z "${apache_key_connstotal}" \
		-a ! -z "${apache_key_connsasyncwriting}" \
		-a ! -z "${apache_key_connsasynckeepalive}" \
		-a ! -z "${apache_key_connsasyncclosing}" \
		]
		then
		apache_has_conns=1
	else
		apache_has_conns=0
	fi

	return 0
}

apache_get() {
	local oIFS="${IFS}" ret
	IFS=$':\n' apache_response=($(run curl -Ss ${apache_curl_opts} "${apache_url}"))
	ret=$?
	IFS="${oIFS}"

	[ $ret -ne 0 -o "${#apache_response[@]}" -eq 0 ] && return 1

	# the last line on the apache output is "Scoreboard"
	# we use this label to detect that the output has a new word count
	if [ ${apache_keys_detected} -eq 0 -o "${apache_response[${apache_key_scoreboard}]}" != "Scoreboard" ]
		then
		apache_detect "${apache_response[@]}" || return 1
		apache_keys_detected=1
	fi

	apache_accesses="${apache_response[${apache_key_accesses}]}"
	apache_kbytes="${apache_response[${apache_key_kbytes}]}"

	float2int "${apache_response[${apache_key_reqpersec}]}" ${apache_decimal_detail}
	apache_reqpersec=${FLOAT2INT_RESULT}

	float2int "${apache_response[${apache_key_bytespersec}]}" ${apache_decimal_detail}
	apache_bytespersec=${FLOAT2INT_RESULT}

	float2int "${apache_response[${apache_key_bytesperreq}]}" ${apache_decimal_detail}
	apache_bytesperreq=${FLOAT2INT_RESULT}

	apache_busyworkers="${apache_response[${apache_key_busyworkers}]}"
	apache_idleworkers="${apache_response[${apache_key_idleworkers}]}"

	if [ -z "${apache_accesses}" \
		-o -z "${apache_kbytes}" \
		-o -z "${apache_reqpersec}" \
		-o -z "${apache_bytespersec}" \
		-o -z "${apache_bytesperreq}" \
		-o -z "${apache_busyworkers}" \
		-o -z "${apache_idleworkers}" \
		]
		then
		error "empty values got from apache server: ${apache_response[*]}"
		return 1
	fi

	if [ ${apache_has_conns} -eq 1 ]
		then
		apache_connstotal="${apache_response[${apache_key_connstotal}]}"
		apache_connsasyncwriting="${apache_response[${apache_key_connsasyncwriting}]}"
		apache_connsasynckeepalive="${apache_response[${apache_key_connsasynckeepalive}]}"
		apache_connsasyncclosing="${apache_response[${apache_key_connsasyncclosing}]}"
	fi

	return 0
}

# _check is called once, to find out if this chart should be enabled or not
apache_check() {

	apache_get
	if [ $? -ne 0 ]
		then
		error "cannot find stub_status on URL '${apache_url}'. Please set apache_url='http://apache.server:80/server-status?auto' in $confd/apache.conf"
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
CHART apache_local.bytesperreq '' "apache Lifetime Avg. Response Size" "bytes/request" statistics apache.bytesperreq area $((apache_priority + 8)) $apache_update_every
DIMENSION size '' absolute 1 ${apache_decimal_detail}
CHART apache_local.workers '' "apache Workers" "workers" workers apache.workers stacked $((apache_priority + 5)) $apache_update_every
DIMENSION idle '' absolute 1 1
DIMENSION busy '' absolute 1 1
CHART apache_local.reqpersec '' "apache Lifetime Avg. Requests/s" "requests/s" statistics apache.reqpersec line $((apache_priority + 6)) $apache_update_every
DIMENSION requests '' absolute 1 ${apache_decimal_detail}
CHART apache_local.bytespersec '' "apache Lifetime Avg. Bandwidth/s" "kilobits/s" statistics apache.bytespersec area $((apache_priority + 7)) $apache_update_every
DIMENSION sent '' absolute 8 $((apache_decimal_detail * 1000))
CHART apache_local.requests '' "apache Requests" "requests/s" requests apache.requests line $((apache_priority + 1)) $apache_update_every
DIMENSION requests '' incremental 1 1
CHART apache_local.net '' "apache Bandwidth" "kilobits/s" bandwidth apache.net area $((apache_priority + 3)) $apache_update_every
DIMENSION sent '' incremental 8 1
EOF

	if [ ${apache_has_conns} -eq 1 ]
		then
		cat <<EOF2
CHART apache_local.connections '' "apache Connections" "connections" connections apache.connections line $((apache_priority + 2)) $apache_update_every
DIMENSION connections '' absolute 1 1
CHART apache_local.conns_async '' "apache Async Connections" "connections" connections apache.conns_async stacked $((apache_priority + 4)) $apache_update_every
DIMENSION keepalive '' absolute 1 1
DIMENSION closing '' absolute 1 1
DIMENSION writing '' absolute 1 1
EOF2
	fi

	return 0
}

# _update is called continuously, to collect the values
apache_update() {
	local reqs net
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	apache_get || return 1

	# write the result of the work.
	cat <<VALUESEOF
BEGIN apache_local.requests $1
SET requests = $((apache_accesses))
END
BEGIN apache_local.net $1
SET sent = $((apache_kbytes))
END
BEGIN apache_local.reqpersec $1
SET requests = $((apache_reqpersec))
END
BEGIN apache_local.bytespersec $1
SET sent = $((apache_bytespersec))
END
BEGIN apache_local.bytesperreq $1
SET size = $((apache_bytesperreq))
END
BEGIN apache_local.workers $1
SET idle = $((apache_idleworkers))
SET busy = $((apache_busyworkers))
END
VALUESEOF

	if [ ${apache_has_conns} -eq 1 ]
		then
	cat <<VALUESEOF2
BEGIN apache_local.connections $1
SET connections = $((apache_connstotal))
END
BEGIN apache_local.conns_async $1
SET keepalive = $((apache_connsasynckeepalive))
SET closing = $((apache_connsasyncclosing))
SET writing = $((apache_connsasyncwriting))
END
VALUESEOF2
	fi

	return 0
}
