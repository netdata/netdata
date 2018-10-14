# shellcheck shell=bash
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0-or-later

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#
# Contributed by @safeie with PR #276

# first, you need open php-fpm status in php-fpm.conf
# second, you need add status location in nginx.conf
# you can see, https://easyengine.io/tutorials/php/fpm-status-page/

declare -A phpfpm_urls=()
declare -A phpfpm_curl_opts=()

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
phpfpm_update_every=
phpfpm_priority=60000

declare -a phpfpm_response=()
phpfpm_pool=""
phpfpm_start_time=""
phpfpm_start_since=0
phpfpm_accepted_conn=0
phpfpm_listen_queue=0
phpfpm_max_listen_queue=0
phpfpm_listen_queue_len=0
phpfpm_idle_processes=0
phpfpm_active_processes=0
phpfpm_total_processes=0
phpfpm_max_active_processes=0
phpfpm_max_children_reached=0
phpfpm_slow_requests=0
phpfpm_get() {
	local opts="${1}" url="${2}"

	# shellcheck disable=SC2207,2086
	phpfpm_response=($(run curl -Ss ${opts} "${url}"))
	# shellcheck disable=SC2181
	if [ $? -ne 0 ] || [ "${#phpfpm_response[@]}" -eq 0 ]; then
		return 1
	fi

	if [[ "${phpfpm_response[0]}" != "pool:" \
		|| "${phpfpm_response[2]}" != "process" \
		|| "${phpfpm_response[5]}" != "start" \
		|| "${phpfpm_response[12]}" != "accepted" \
		|| "${phpfpm_response[15]}" != "listen" \
		|| "${phpfpm_response[16]}" != "queue:" \
		|| "${phpfpm_response[26]}" != "idle" \
		|| "${phpfpm_response[29]}" != "active" \
		|| "${phpfpm_response[32]}" != "total" \
	]]
		then
		error "invalid response from phpfpm status server: ${phpfpm_response[*]}"
		return 1
	fi

	phpfpm_pool="${phpfpm_response[1]}"
	phpfpm_start_time="${phpfpm_response[7]} ${phpfpm_response[8]}"
	phpfpm_start_since="${phpfpm_response[11]}"
	phpfpm_accepted_conn="${phpfpm_response[14]}"
	phpfpm_listen_queue="${phpfpm_response[17]}"
	phpfpm_max_listen_queue="${phpfpm_response[21]}"
	phpfpm_listen_queue_len="${phpfpm_response[25]}"
	phpfpm_idle_processes="${phpfpm_response[28]}"
	phpfpm_active_processes="${phpfpm_response[31]}"
	phpfpm_total_processes="${phpfpm_response[34]}"
	phpfpm_max_active_processes="${phpfpm_response[38]}"
	phpfpm_max_children_reached="${phpfpm_response[42]}"
	if [ "${phpfpm_response[43]}" == "slow" ]
		then
	  	phpfpm_slow_requests="${phpfpm_response[45]}"
	else
	  	phpfpm_slow_requests="-1"
	fi

	if [[ -z "${phpfpm_pool}" \
		|| -z "${phpfpm_start_time}" \
		|| -z "${phpfpm_start_since}" \
		|| -z "${phpfpm_accepted_conn}" \
		|| -z "${phpfpm_listen_queue}" \
		|| -z "${phpfpm_max_listen_queue}" \
		|| -z "${phpfpm_listen_queue_len}" \
		|| -z "${phpfpm_idle_processes}" \
		|| -z "${phpfpm_active_processes}" \
		|| -z "${phpfpm_total_processes}" \
		|| -z "${phpfpm_max_active_processes}" \
		|| -z "${phpfpm_max_children_reached}" \
	]]
		then
		error "empty values got from phpfpm status server: ${phpfpm_response[*]}"
		return 1
	fi

	return 0
}

# _check is called once, to find out if this chart should be enabled or not
phpfpm_check() {
	if [ ${#phpfpm_urls[@]} -eq 0 ]; then
		phpfpm_urls[local]="http://localhost/status"
	fi

	local m
	for m in "${!phpfpm_urls[@]}"
	do
		phpfpm_get "${phpfpm_curl_opts[$m]}" "${phpfpm_urls[$m]}"
		# shellcheck disable=SC2181
		if [ $? -ne 0 ]; then
			# shellcheck disable=SC2154
			error "cannot find status on URL '${phpfpm_urls[$m]}'. Please set phpfpm_urls[$m]='http://localhost/status' in $confd/phpfpm.conf"
			unset "phpfpm_urls[$m]"
			continue
		fi
	done

	if [ ${#phpfpm_urls[@]} -eq 0 ]; then
		error "no phpfpm servers found. Please set phpfpm_urls[name]='url' to whatever needed to get status to the phpfpm server, in $confd/phpfpm.conf"
		return 1
	fi

	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	return 0
}

# _create is called once, to create the charts
phpfpm_create() {
	local m
	for m in "${!phpfpm_urls[@]}"
	do
		cat <<EOF
CHART phpfpm_$m.connections '' "PHP-FPM Active Connections" "connections" phpfpm phpfpm.connections line $((phpfpm_priority + 1)) $phpfpm_update_every
DIMENSION active '' absolute 1 1
DIMENSION maxActive 'max active' absolute 1 1
DIMENSION idle '' absolute 1 1

CHART phpfpm_$m.requests '' "PHP-FPM Requests" "requests/s" phpfpm phpfpm.requests line $((phpfpm_priority + 2)) $phpfpm_update_every
DIMENSION requests '' incremental 1 1

CHART phpfpm_$m.performance '' "PHP-FPM Performance" "status" phpfpm phpfpm.performance line $((phpfpm_priority + 3)) $phpfpm_update_every
DIMENSION reached 'max children reached' absolute 1 1
EOF
		if [ $((phpfpm_slow_requests)) -ne -1 ]
			then
			echo "DIMENSION slow 'slow requests' absolute 1 1"
		fi
	done

	return 0
}

# _update is called continuously, to collect the values
phpfpm_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	local m
	for m in "${!phpfpm_urls[@]}"
	do
		phpfpm_get "${phpfpm_curl_opts[$m]}" "${phpfpm_urls[$m]}"
		# shellcheck disable=SC2181
		if [ $? -ne 0 ]; then
			continue
		fi

		# write the result of the work.
		cat <<EOF
BEGIN phpfpm_$m.connections $1
SET active = $((phpfpm_active_processes))
SET maxActive = $((phpfpm_max_active_processes))
SET idle = $((phpfpm_idle_processes))
END
BEGIN phpfpm_$m.requests $1
SET requests = $((phpfpm_accepted_conn))
END
BEGIN phpfpm_$m.performance $1
SET reached = $((phpfpm_max_children_reached))
EOF
		if [ $((phpfpm_slow_requests)) -ne -1 ]
			then
			echo "SET slow = $((phpfpm_slow_requests))"
		fi
		echo "END"
	done

	return 0
}
