# no shebang necessary - this is a library to be sourced
# SPDX-License-Identifier: GPL-3.0-or-later
# shellcheck disable=SC1091,SC1117,SC2002,SC2004,SC2034,SC2046,SC2059,SC2086,SC2129,SC2148,SC2154,SC2155,SC2162,SC2166,SC2181,SC2193

# make sure we have a UID
[ -z "${UID}" ] && UID="$(id -u)"

# -----------------------------------------------------------------------------

setup_terminal() {
	TPUT_RESET=""
	TPUT_BLACK=""
	TPUT_RED=""
	TPUT_GREEN=""
	TPUT_YELLOW=""
	TPUT_BLUE=""
	TPUT_PURPLE=""
	TPUT_CYAN=""
	TPUT_WHITE=""
	TPUT_BGBLACK=""
	TPUT_BGRED=""
	TPUT_BGGREEN=""
	TPUT_BGYELLOW=""
	TPUT_BGBLUE=""
	TPUT_BGPURPLE=""
	TPUT_BGCYAN=""
	TPUT_BGWHITE=""
	TPUT_BOLD=""
	TPUT_DIM=""
	TPUT_UNDERLINED=""
	TPUT_BLINK=""
	TPUT_INVERTED=""
	TPUT_STANDOUT=""
	TPUT_BELL=""
	TPUT_CLEAR=""

	# Is stderr on the terminal? If not, then fail
	test -t 2 || return 1

	if command -v tput 1>/dev/null 2>&1; then
		if [ $(($(tput colors 2>/dev/null))) -ge 8 ]; then
			# Enable colors
			TPUT_RESET="$(tput sgr 0)"
			TPUT_BLACK="$(tput setaf 0)"
			TPUT_RED="$(tput setaf 1)"
			TPUT_GREEN="$(tput setaf 2)"
			TPUT_YELLOW="$(tput setaf 3)"
			TPUT_BLUE="$(tput setaf 4)"
			TPUT_PURPLE="$(tput setaf 5)"
			TPUT_CYAN="$(tput setaf 6)"
			TPUT_WHITE="$(tput setaf 7)"
			TPUT_BGBLACK="$(tput setab 0)"
			TPUT_BGRED="$(tput setab 1)"
			TPUT_BGGREEN="$(tput setab 2)"
			TPUT_BGYELLOW="$(tput setab 3)"
			TPUT_BGBLUE="$(tput setab 4)"
			TPUT_BGPURPLE="$(tput setab 5)"
			TPUT_BGCYAN="$(tput setab 6)"
			TPUT_BGWHITE="$(tput setab 7)"
			TPUT_BOLD="$(tput bold)"
			TPUT_DIM="$(tput dim)"
			TPUT_UNDERLINED="$(tput smul)"
			TPUT_BLINK="$(tput blink)"
			TPUT_INVERTED="$(tput rev)"
			TPUT_STANDOUT="$(tput smso)"
			TPUT_BELL="$(tput bel)"
			TPUT_CLEAR="$(tput clear)"
		fi
	fi

	return 0
}
setup_terminal || echo >/dev/null

progress() {
	echo >&2 " --- ${TPUT_DIM}${TPUT_BOLD}${*}${TPUT_RESET} --- "
}

# -----------------------------------------------------------------------------

netdata_banner() {
	local l1="  ^" \
		l2="  |.-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-" \
		l3="  |   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'  " \
		l4="  +----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+--->" \
		sp="                                                                              " \
		netdata="netdata" start end msg="${*}" chartcolor="${TPUT_DIM}"

	[ ${#msg} -lt ${#netdata} ] && msg="${msg}${sp:0:$((${#netdata} - ${#msg}))}"
	[ ${#msg} -gt $((${#l2} - 20)) ] && msg="${msg:0:$((${#l2} - 23))}..."

	start="$((${#l2} / 2 - 4))"
	[ $((start + ${#msg} + 4)) -gt ${#l2} ] && start=$((${#l2} - ${#msg} - 4))
	end=$((start + ${#msg} + 4))

	echo >&2
	echo >&2 "${chartcolor}${l1}${TPUT_RESET}"
	echo >&2 "${chartcolor}${l2:0:start}${sp:0:2}${TPUT_RESET}${TPUT_BOLD}${TPUT_GREEN}${netdata}${TPUT_RESET}${chartcolor}${sp:0:$((end - start - 2 - ${#netdata}))}${l2:end:$((${#l2} - end))}${TPUT_RESET}"
	echo >&2 "${chartcolor}${l3:0:start}${sp:0:2}${TPUT_RESET}${TPUT_BOLD}${TPUT_CYAN}${msg}${TPUT_RESET}${chartcolor}${sp:0:2}${l3:end:$((${#l2} - end))}${TPUT_RESET}"
	echo >&2 "${chartcolor}${l4}${TPUT_RESET}"
	echo >&2
}

# -----------------------------------------------------------------------------
# Include system related methods
source "$(dirname $0)/system.sh"

# -----------------------------------------------------------------------------

NETDATA_START_CMD="netdata"
NETDATA_STOP_CMD="killall netdata"

install_netdata_service() {
	local uname="$(uname 2>/dev/null)"

	if [ "${UID}" -eq 0 ]; then
		if [ "${uname}" = "Darwin" ]; then

			if [ -f "/Library/LaunchDaemons/com.github.netdata.plist" ]; then
				echo >&2 "file '/Library/LaunchDaemons/com.github.netdata.plist' already exists."
				return 0
			else
				echo >&2 "Installing MacOS X plist file..."
				run cp system/netdata.plist /Library/LaunchDaemons/com.github.netdata.plist &&
					run launchctl load /Library/LaunchDaemons/com.github.netdata.plist &&
					return 0
			fi

		elif [ "${uname}" = "FreeBSD" ]; then

			run cp system/netdata-freebsd /etc/rc.d/netdata &&
				NETDATA_START_CMD="service netdata start" &&
				NETDATA_STOP_CMD="service netdata stop" &&
				return 0

		elif issystemd; then
			# systemd is running on this system
			NETDATA_START_CMD="systemctl start netdata"
			NETDATA_STOP_CMD="systemctl stop netdata"

			SYSTEMD_DIRECTORY=""

			if [ -d "/lib/systemd/system" ]; then
				SYSTEMD_DIRECTORY="/lib/systemd/system"
			elif [ -d "/usr/lib/systemd/system" ]; then
				SYSTEMD_DIRECTORY="/usr/lib/systemd/system"
			fi

			if [ "${SYSTEMD_DIRECTORY}x" != "x" ]; then
				echo >&2 "Installing systemd service..."
				run cp system/netdata.service "${SYSTEMD_DIRECTORY}/netdata.service" &&
					run systemctl daemon-reload &&
					run systemctl enable netdata &&
					return 0
			else
				echo >&2 "no systemd directory; cannot install netdata.service"
			fi
		else
			install_non_systemd_init
			local ret=$?

			if [ ${ret} -eq 0 ]; then
				if [ -n "${service_cmd}" ]; then
					NETDATA_START_CMD="service netdata start"
					NETDATA_STOP_CMD="service netdata stop"
				elif [ -n "${rcservice_cmd}" ]; then
					NETDATA_START_CMD="rc-service netdata start"
					NETDATA_STOP_CMD="rc-service netdata stop"
				fi
			fi

			return ${ret}
		fi
	fi

	return 1
}

# -----------------------------------------------------------------------------
# stop netdata

pidisnetdata() {
	if [ -d /proc/self ]; then
		[ -z "$1" -o ! -f "/proc/$1/stat" ] && return 1
		[ "$(cat "/proc/$1/stat" | cut -d '(' -f 2 | cut -d ')' -f 1)" = "netdata" ] && return 0
		return 1
	fi
	return 0
}

stop_netdata_on_pid() {
	local pid="${1}" ret=0 count=0

	pidisnetdata "${pid}" || return 0

	printf >&2 "Stopping netdata on pid %s ..." "${pid}"
	while [ -n "$pid" ] && [ ${ret} -eq 0 ]; do
		if [ ${count} -gt 45 ]; then
			echo >&2 "Cannot stop the running netdata on pid ${pid}."
			return 1
		fi

		count=$((count + 1))

		run kill "${pid}" 2>/dev/null
		ret=$?

		test ${ret} -eq 0 && printf >&2 "." && sleep 2
	done

	echo >&2
	if [ ${ret} -eq 0 ]; then
		echo >&2 "SORRY! CANNOT STOP netdata ON PID ${pid} !"
		return 1
	fi

	echo >&2 "netdata on pid ${pid} stopped."
	return 0
}

netdata_pids() {
	local p myns ns

	myns="$(readlink /proc/self/ns/pid 2>/dev/null)"

	# echo >&2 "Stopping a (possibly) running netdata (namespace '${myns}')..."

	for p in \
		$(cat /var/run/netdata.pid 2>/dev/null) \
		$(cat /var/run/netdata/netdata.pid 2>/dev/null) \
		$(safe_pidof netdata 2>/dev/null); do
		ns="$(readlink "/proc/${p}/ns/pid" 2>/dev/null)"

		if [ -z "${myns}" ] || [ -z "${ns}" ] || [ "${myns}" = "${ns}" ]; then
			pidisnetdata "${p}" && echo "${p}"
		fi
	done
}

stop_all_netdata() {
	local p
	for p in $(netdata_pids); do
		# shellcheck disable=SC2086
		stop_netdata_on_pid ${p}
	done
}

# -----------------------------------------------------------------------------
# restart netdata

restart_netdata() {
	local netdata="${1}"
	shift

	local started=0

	progress "Start netdata"

	if [ "${UID}" -eq 0 ]; then
		service netdata stop
		stop_all_netdata
		service netdata restart && started=1

		if [ ${started} -eq 1 ] && [ -z "$(netdata_pids)" ]; then
			echo >&2 "Ooops! it seems netdata is not started."
			started=0
		fi

		if [ ${started} -eq 0 ]; then
			service netdata start && started=1
		fi
	fi

	if [ ${started} -eq 1 ] && [ -z "$(netdata_pids)" ]; then
		echo >&2 "Hm... it seems netdata is still not started."
		started=0
	fi

	if [ ${started} -eq 0 ]; then
		# still not started...

		run stop_all_netdata
		run "${netdata}" "${@}"
		return $?
	fi

	return 0
}

# -----------------------------------------------------------------------------
# install netdata logrotate

install_netdata_logrotate() {
	if [ "${UID}" -eq 0 ]; then
		if [ -d /etc/logrotate.d ]; then
			if [ ! -f /etc/logrotate.d/netdata ]; then
				run cp system/netdata.logrotate /etc/logrotate.d/netdata
			fi

			if [ -f /etc/logrotate.d/netdata ]; then
				run chmod 644 /etc/logrotate.d/netdata
			fi

			return 0
		fi
	fi

	return 1
}

# -----------------------------------------------------------------------------
# create netdata.conf

create_netdata_conf() {
	local path="${1}" url="${2}"

	if [ -s "${path}" ]; then
		return 0
	fi

	if [ -n "$url" ]; then
		echo >&2 "Downloading default configuration from netdata..."
		sleep 5

		# remove a possibly obsolete configuration file
		[ -f "${path}.new" ] && rm "${path}.new"

		# disable a proxy to get data from the local netdata
		export http_proxy=
		export https_proxy=

		if command -v curl 1>/dev/null 2>&1; then
			run curl -sSL --connect-timeout 10 --retry 3 "${url}" >"${path}.new"
		elif command -v wget 1>/dev/null 2>&1; then
			run wget -T 15 -O - "${url}" >"${path}.new"
		fi

		if [ -s "${path}.new" ]; then
			run mv "${path}.new" "${path}"
			run_ok "New configuration saved for you to edit at ${path}"
		else
			[ -f "${path}.new" ] && rm "${path}.new"
			run_failed "Cannnot download configuration from netdata daemon using url '${url}'"
			url=''
		fi
	fi

	if [ -z "$url" ]; then
		echo "# netdata can generate its own config which is available at 'http://<netdata_ip>/netdata.conf'" >"${path}"
		echo "# You can download it with command like: 'wget -O ${path} http://localhost:19999/netdata.conf'" >>"${path}"
	fi

}
