# shellcheck shell=bash
# no need for shebang - this file is loaded from charts.d.plugin
# SPDX-License-Identifier: GPL-3.0+

# netdata
# real-time performance and health monitoring, done right!
# (C) 2018 Costa Tsaousis <costa@tsaousis.gr>
#

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
libreswan_update_every=1

# the priority is used to sort the charts on the dashboard
# 1 = the first chart
libreswan_priority=90000

# set to 1, to run ipsec with sudo
libreswan_sudo=1

# global variables to store our collected data

# [TUNNELID] = TUNNELNAME
# here we track the *latest* established tunnels
# as detected by: ipsec whack --status
declare -A libreswan_connected_tunnels=()

# [TUNNELID] = VALUE
# here we track values of all established tunnels (not only the latest)
# as detected by: ipsec whack --trafficstatus
declare -A libreswan_traffic_in=()
declare -A libreswan_traffic_out=()
declare -A libreswan_established_add_time=()

# [TUNNELNAME] = CHARTID
# here we remember CHARTIDs of all tunnels
# we need this to avoid converting tunnel names to chart IDs on every iteration
declare -A libreswan_tunnel_charts=()

# run the ipsec command
libreswan_ipsec() {
	if [ ${libreswan_sudo} -ne 0 ]
		then
		sudo -n "${IPSEC_CMD}" "${@}"
		return $?
	else
		"${IPSEC_CMD}" "${@}"
		return $?
	fi
}

# fetch latest values - fill the arrays
libreswan_get() {
	# do all the work to collect / calculate the values
	# for each dimension

	# empty the variables
	libreswan_traffic_in=()
	libreswan_traffic_out=()
	libreswan_established_add_time=()
	libreswan_connected_tunnels=()

	# convert the ipsec command output to a shell script
	# and source it to get the values
	source <(
		{
			libreswan_ipsec whack --status;
			libreswan_ipsec whack --trafficstatus;
		} | sed -n \
				-e "s|[0-9]\+ #\([0-9]\+\): \"\(.*\)\".*IPsec SA established.*newest IPSEC.*|libreswan_connected_tunnels[\"\1\"]=\"\2\"|p" \
				-e "s|[0-9]\+ #\([0-9]\+\): \"\(.*\)\",.* add_time=\([0-9]\+\),.* inBytes=\([0-9]\+\),.* outBytes=\([0-9]\+\).*|libreswan_traffic_in[\"\1\"]=\"\4\"; libreswan_traffic_out[\"\1\"]=\"\5\"; libreswan_established_add_time[\"\1\"]=\"\3\";|p"
	) || return 1

	# check we got some data
	[ ${#libreswan_connected_tunnels[@]} -eq 0 ] && return 1

	return 0
}

# _check is called once, to find out if this chart should be enabled or not
libreswan_check() {
	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	require_cmd ipsec || return 1

	# make sure it is libreswan
	if [ -z "$(ipsec --version | grep -i libreswan)" ]
	then
	    error "ipsec command is not Libreswan. Disabling Libreswan plugin."
	    return 1
	fi

	# check that we can collect data
	libreswan_get || return 1

	return 0
}

# create the charts for an ipsec tunnel
libreswan_create_one() {
	local n="${1}" name

	name="${libreswan_connected_tunnels[${n}]}"

	[ ! -z "${libreswan_tunnel_charts[${name}]}" ] && return 0

	libreswan_tunnel_charts[${name}]="$(fixid "${name}")"

	cat <<EOF
CHART libreswan.${libreswan_tunnel_charts[${name}]}_net '${name}_net' "LibreSWAN Tunnel ${name} Traffic" "kilobits/s" "${name}" libreswan.net area $((libreswan_priority)) $libreswan_update_every
DIMENSION in '' incremental 8 1000
DIMENSION out '' incremental -8 1000
CHART libreswan.${libreswan_tunnel_charts[${name}]}_uptime '${name}_uptime' "LibreSWAN Tunnel ${name} Uptime" "seconds" "${name}" libreswan.uptime line $((libreswan_priority + 1)) $libreswan_update_every
DIMENSION uptime '' absolute 1 1
EOF

	return 0

}

# _create is called once, to create the charts
libreswan_create() {
	local n
	for n in "${!libreswan_connected_tunnels[@]}"
	do
		libreswan_create_one "${n}"
	done
	return 0
}

libreswan_now=$(date +%s)

# send the values to netdata for an ipsec tunnel
libreswan_update_one() {
	local n="${1}" microseconds="${2}" name id uptime

	name="${libreswan_connected_tunnels[${n}]}"
	id="${libreswan_tunnel_charts[${name}]}"

	[ -z "${id}" ] && libreswan_create_one "${name}"

	uptime=$(( ${libreswan_now} - ${libreswan_established_add_time[${n}]} ))
	[ ${uptime} -lt 0 ] && uptime=0

		# write the result of the work.
	cat <<VALUESEOF
BEGIN libreswan.${id}_net ${microseconds}
SET in = ${libreswan_traffic_in[${n}]}
SET out = ${libreswan_traffic_out[${n}]}
END
BEGIN libreswan.${id}_uptime ${microseconds}
SET uptime = ${uptime}
END
VALUESEOF
}

# _update is called continiously, to collect the values
libreswan_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	libreswan_get || return 1
	libreswan_now=$(date +%s)

	local n
	for n in "${!libreswan_connected_tunnels[@]}"
	do
		libreswan_update_one "${n}" "${@}"
	done

	return 0
}
