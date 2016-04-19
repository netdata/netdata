#!/bin/sh

# Description: Tomcat netdata charts.d plugin
# Author: Jorge Romero

# the URL to download tomcat status info
tomcat_url="http://<user>:<password>@localhost:8080/manager/status?XML=true"

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
tomcat_update_every=

tomcat_priority=60000

# convert tomcat floating point values
# to integer using this multiplier
# this only affects precision - the values
# will be in the proper units
tomcat_decimal_detail=1000000

# used by volume chart to convert bytes to KB
tomcat_decimal_KB_detail=1000

tomcat_check() {

	tomcat_get
	if [ $? -ne 0 ]
		then
		echo >&2 "tomcat: cannot find stub_status on URL '${tomcat_url}'. Please set tomcat_url='http://<user>:<password>@localhost:8080/manager/status?XML=true'"
		return 1
	fi

	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	return 0
}

tomcat_get() {

	tomcat_accesses=$(curl -s "${tomcat_url}" | xmllint --xpath "string(/status/connector[@name='\"http-bio-8080\"']/requestInfo/@requestCount)" - )
	tomcat_volume=$(curl -s "${tomcat_url}" | xmllint --xpath "string(/status/connector[@name='\"http-bio-8080\"']/requestInfo/@bytesSent)" - )
	tomcat_threads=$(curl -s "${tomcat_url}" | xmllint --xpath "string(/status/connector[@name='\"http-bio-8080\"']/threadInfo/@currentThreadCount)" -)
	tomcat_threads_busy=$(curl -s "${tomcat_url}" | xmllint --xpath "string(/status/connector[@name='\"http-bio-8080\"']/threadInfo/@currentThreadsBusy)" -)
	tomcat_jvm_freememory=$(curl -s "${tomcat_url}" | xmllint --xpath "string(/status/jvm/memory/@free)" - )

	return 0
}

# _create is called once, to create the charts
tomcat_create() {
	cat <<EOF
CHART tomcat.accesses '' "tomcat requests" "requests/s" statistics tomcat.accesses area $[tomcat_priority + 8] $tomcat_update_every
DIMENSION accesses '' incremental
CHART tomcat.volume '' "tomcat volume" "KB/s" volume tomcat.volume area $[tomcat_priority + 5] $tomcat_update_every
DIMENSION volume '' incremental divisor ${tomcat_decimal_KB_detail}
CHART tomcat.threads '' "tomcat threads" "current threads" statistics tomcat.threads line $[tomcat_priority + 6] $tomcat_update_every
DIMENSION current '' absolute 1
DIMENSION busy '' absolute 1
CHART tomcat.jvm '' "JVM Free Memory" "MB" statistics tomcat.jvm area $[tomcat_priority + 8] $tomcat_update_every
DIMENSION jvm '' absolute 1 ${tomcat_decimal_detail}
EOF
	return 0
}

# _update is called continiously, to collect the values
tomcat_update() {
	local reqs net
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	tomcat_get || return 1

	# write the result of the work.
	cat <<VALUESEOF
BEGIN tomcat.accesses $1
SET accesses = $[tomcat_accesses]
END
BEGIN tomcat.volume $1
SET volume = $[tomcat_volume]
END
BEGIN tomcat.threads $1
SET current = $[tomcat_threads]
SET busy = $[tomcat_threads_busy]
END
BEGIN tomcat.jvm $1
SET jvm = $[tomcat_jvm_freememory]
END
VALUESEOF

	return 0
}
