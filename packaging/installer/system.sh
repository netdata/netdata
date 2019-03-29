# no shebang necessary - this is a library to be sourced
# SPDX-License-Identifier: GPL-3.0-or-later
# shellcheck disable=SC1091,SC1117,SC2002,SC2004,SC2034,SC2046,SC2059,SC2086,SC2129,SC2148,SC2154,SC2155,SC2162,SC2166,SC2181,SC2193

# make sure we have a UID
[ -z "${UID}" ] && UID="$(id -u)"

# -----------------------------------------------------------------------------
# pretty-print run command wrapper lib

fatal() {
	printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ABORTED ${TPUT_RESET} ${*} \n\n"
	exit 1
}

run_ok() {
	printf >&2 "${TPUT_BGGREEN}${TPUT_WHITE}${TPUT_BOLD} OK ${TPUT_RESET} ${*} \n\n"
}

run_failed() {
	printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} FAILED ${TPUT_RESET} ${*} \n\n"
}

ESCAPED_PRINT_METHOD=
printf "%q " test >/dev/null 2>&1
[ $? -eq 0 ] && ESCAPED_PRINT_METHOD="printfq"
escaped_print() {
	if [ "${ESCAPED_PRINT_METHOD}" = "printfq" ]; then
		printf "%q " "${@}"
	else
		printf "%s" "${*}"
	fi
	return 0
}

run_logfile="/dev/null"
run() {
	local user="${USER--}" dir="${PWD}" info info_console

	if [ "${UID}" = "0" ]; then
		info="[root ${dir}]# "
		info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]# "
	else
		info="[${user} ${dir}]$ "
		info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]$ "
	fi

	printf >>"${run_logfile}" "${info}"
	escaped_print >>"${run_logfile}" "${@}"
	printf >>"${run_logfile}" " ... "

	printf >&2 "${info_console}${TPUT_BOLD}${TPUT_YELLOW}"
	escaped_print >&2 "${@}"
	printf >&2 "${TPUT_RESET}\n"

	"${@}"

	local ret=$?
	if [ ${ret} -ne 0 ]; then
		run_failed
		printf >>"${run_logfile}" "FAILED with exit code ${ret}\n"
	else
		run_ok
		printf >>"${run_logfile}" "OK\n"
	fi

	return ${ret}
}

# -----------------------------------------------------------------------------
# portable service command

service_cmd="$(command -v service 2>/dev/null)"
rcservice_cmd="$(command -v rc-service 2>/dev/null)"
systemctl_cmd="$(command -v systemctl 2>/dev/null)"
service() {

	local cmd="${1}" action="${2}"

	if [ -n "${systemctl_cmd}" ]; then
		run "${systemctl_cmd}" "${action}" "${cmd}"
		return $?
	elif [ -n "${service_cmd}" ]; then
		run "${service_cmd}" "${cmd}" "${action}"
		return $?
	elif [ -n "${rcservice_cmd}" ]; then
		run "${rcservice_cmd}" "${cmd}" "${action}"
		return $?
	fi
	return 1
}

# -----------------------------------------------------------------------------
# portable pidof

safe_pidof() {
	local pidof_cmd="$(command -v pidof 2>/dev/null)"
	if [ -n "${pidof_cmd}" ]; then
		${pidof_cmd} "${@}"
		return $?
	else
		ps -acxo pid,comm |
			sed "s/^ *//g" |
			grep netdata |
			cut -d ' ' -f 1
		return $?
	fi
}

# -----------------------------------------------------------------------------
find_processors() {
	local cpus
	if [ -f "/proc/cpuinfo" ]; then
		# linux
		cpus=$(grep -c ^processor /proc/cpuinfo)
	else
		# freebsd
		cpus=$(sysctl hw.ncpu 2>/dev/null | grep ^hw.ncpu | cut -d ' ' -f 2)
	fi
	if [ -z "${cpus}" ] || [ $((cpus)) -lt 1 ]; then
		echo 1
	else
		echo "${cpus}"
	fi
}

iscontainer() {
	# man systemd-detect-virt
	local cmd=$(command -v systemd-detect-virt 2>/dev/null)
	if [ -n "${cmd}" ] && [ -x "${cmd}" ]; then
		"${cmd}" --container >/dev/null 2>&1 && return 0
	fi

	# /proc/1/sched exposes the host's pid of our init !
	# http://stackoverflow.com/a/37016302
	local pid=$(cat /proc/1/sched 2>/dev/null | head -n 1 | {
		IFS='(),#:' read name pid th threads
		echo "$pid"
	})
	if [ -n "${pid}" ]; then
		pid=$(( pid + 0 ))
		[ ${pid} -gt 1 ] && return 0
	fi

	# lxc sets environment variable 'container'
	[ -n "${container}" ] && return 0

	# docker creates /.dockerenv
	# http://stackoverflow.com/a/25518345
	[ -f "/.dockerenv" ] && return 0

	# ubuntu and debian supply /bin/running-in-container
	# https://www.apt-browse.org/browse/ubuntu/trusty/main/i386/upstart/1.12.1-0ubuntu4/file/bin/running-in-container
	if [ -x "/bin/running-in-container" ]; then
		"/bin/running-in-container" >/dev/null 2>&1 && return 0
	fi

	return 1
}

issystemd() {
	local pids p myns ns systemctl

	# if the directory /lib/systemd/system OR /usr/lib/systemd/system (SLES 12.x) does not exit, it is not systemd
	[ ! -d /lib/systemd/system -a ! -d /usr/lib/systemd/system ] && return 1

	# if there is no systemctl command, it is not systemd
	# shellcheck disable=SC2230
	systemctl=$(command -v systemctl 2>/dev/null)
	[ -z "${systemctl}" -o ! -x "${systemctl}" ] && return 1

	# if pid 1 is systemd, it is systemd
	[ "$(basename $(readlink /proc/1/exe) 2>/dev/null)" = "systemd" ] && return 0

	# if systemd is not running, it is not systemd
	pids=$(safe_pidof systemd 2>/dev/null)
	[ -z "${pids}" ] && return 1

	# check if the running systemd processes are not in our namespace
	myns="$(readlink /proc/self/ns/pid 2>/dev/null)"
	for p in ${pids}; do
		ns="$(readlink "/proc/${p}/ns/pid" 2>/dev/null)"

		# if pid of systemd is in our namespace, it is systemd
		[ -n "${myns}" ] && [ "${myns}" = "${ns}" ] && return 0
	done

	# else, it is not systemd
	return 1
}

install_non_systemd_init() {
	[ "${UID}" != 0 ] && return 1

	local key="unknown"
	if [ -f /etc/os-release ]; then
		source /etc/os-release || return 1
		key="${ID}-${VERSION_ID}"

	elif [ -f /etc/redhat-release ]; then
		key=$(</etc/redhat-release)
	fi

	if [ -d /etc/init.d ] && [ ! -f /etc/init.d/netdata ]; then
		if [[ ${key} =~ ^(gentoo|alpine).* ]]; then
			echo >&2 "Installing OpenRC init file..."
			run cp system/netdata-openrc /etc/init.d/netdata &&
				run chmod 755 /etc/init.d/netdata &&
				run rc-update add netdata default &&
				return 0

		elif [ "${key}" = "debian-7" ] || [ "${key}" = "ubuntu-12.04" ] || [ "${key}" = "ubuntu-14.04" ]; then
			echo >&2 "Installing LSB init file..."
			run cp system/netdata-lsb /etc/init.d/netdata &&
				run chmod 755 /etc/init.d/netdata &&
				run update-rc.d netdata defaults &&
				run update-rc.d netdata enable &&
				return 0
		elif [[ ${key} =~ ^(amzn-201[5678]|ol|CentOS release 6|Red Hat Enterprise Linux Server release 6|Scientific Linux CERN SLC release 6|CloudLinux Server release 6).* ]]; then
			echo >&2 "Installing init.d file..."
			run cp system/netdata-init-d /etc/init.d/netdata &&
				run chmod 755 /etc/init.d/netdata &&
				run chkconfig netdata on &&
				return 0
		else
			echo >&2 "I don't know what init file to install on system '${key}'. Open a github issue to help us fix it."
			return 1
		fi
	elif [ -f /etc/init.d/netdata ]; then
		echo >&2 "file '/etc/init.d/netdata' already exists."
		return 0
	else
		echo >&2 "I don't know what init file to install on system '${key}'. Open a github issue to help us fix it."
	fi

	return 1
}

portable_add_user() {
	local username="${1}" homedir="${2}"

	[ -z "${homedir}" ] && homedir="/tmp"

    # Check if user exists
	if cut -d ':' -f 1 </etc/passwd | grep "^${username}$" 1>/dev/null 2>&1; then
        echo >&2 "User '${username}' already exists."
        return 0
    fi

	echo >&2 "Adding ${username} user account with home ${homedir} ..."

	# shellcheck disable=SC2230
	local nologin="$(command -v nologin >/dev/null 2>&1 || echo '/bin/false')"

	# Linux
	if command -v useradd 1>/dev/null 2>&1; then
		run useradd -r -g "${username}" -c "${username}" -s "${nologin}" --no-create-home -d "${homedir}" "${username}" && return 0
	fi

	# FreeBSD
	if command -v pw 1>/dev/null 2>&1; then
		run pw useradd "${username}" -d "${homedir}" -g "${username}" -s "${nologin}" && return 0
	fi

	# BusyBox
	if command -v adduser 1>/dev/null 2>&1; then
		run adduser -h "${homedir}" -s "${nologin}" -D -G "${username}" "${username}" && return 0
	fi

	echo >&2 "Failed to add ${username} user account !"

	return 1
}

portable_add_group() {
	local groupname="${1}"

    # Check if group exist
	if cut -d ':' -f 1 </etc/group | grep "^${groupname}$" 1>/dev/null 2>&1; then
        echo >&2 "Group '${groupname}' already exists."
        return 0
    fi

	echo >&2 "Adding ${groupname} user group ..."

	# Linux
	if command -v groupadd 1>/dev/null 2>&1; then
		run groupadd -r "${groupname}" && return 0
	fi

	# FreeBSD
	if command -v pw 1>/dev/null 2>&1; then
		run pw groupadd "${groupname}" && return 0
	fi

	# BusyBox
	if command -v addgroup 1>/dev/null 2>&1; then
		run addgroup "${groupname}" && return 0
	fi

	echo >&2 "Failed to add ${groupname} user group !"
	return 1
}

portable_add_user_to_group() {
	local groupname="${1}" username="${2}"

    # Check if group exist
	if ! cut -d ':' -f 1 </etc/group | grep "^${groupname}$" >/dev/null 2>&1; then
        echo >&2 "Group '${groupname}' does not exist."
        return 1
    fi

    # Check if user is in group
	if [[ ",$(grep "^${groupname}:" </etc/group | cut -d ':' -f 4)," =~ ,${username}, ]]; then
		# username is already there
		echo >&2 "User '${username}' is already in group '${groupname}'."
		return 0
	else
		# username is not in group
		echo >&2 "Adding ${username} user to the ${groupname} group ..."

		# Linux
		if command -v usermod 1>/dev/null 2>&1; then
			run usermod -a -G "${groupname}" "${username}" && return 0
		fi

		# FreeBSD
		if command -v pw 1>/dev/null 2>&1; then
			run pw groupmod "${groupname}" -m "${username}" && return 0
		fi

		# BusyBox
		if command -v addgroup 1>/dev/null 2>&1; then
			run addgroup "${username}" "${groupname}" && return 0
		fi

		echo >&2 "Failed to add user ${username} to group ${groupname} !"
		return 1
	fi
}
