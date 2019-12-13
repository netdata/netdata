#!/usr/bin/env bash
#shellcheck disable=SC2181
#
# This is the netdata uninstaller script
#
# Variables needed by script and taken from '.environment' file:
#  - NETDATA_PREFIX
#  - NETDATA_ADDED_TO_GROUPS
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author: Paweł Krupa <paulfantom@gmail.com>
# Author: Pavlos Emm. Katsoulakis <paul@netdata.cloud>

usage="$(basename "$0") [-h] [-f ] -- program to calculate the answer to life, the universe and everything

where:
    -e, --env    path to environment file (defauls to '/etc/netdata/.environment'
    -f, --force  force uninstallation and do not ask any questions
    -h           show this help text
    -y, --yes    flag needs to be set to proceed with uninstallation"

FILE_REMOVAL_STATUS=0
ENVIRONMENT_FILE="/etc/netdata/.environment"
INTERACTIVITY="-i"
YES=0
while :; do
	case "$1" in
	-h | --help)
		echo "$usage" >&2
		exit 1
		;;
	-f | --force)
		INTERACTIVITY="-f"
		shift
		;;
	-y | --yes)
		YES=1
		shift
		;;
	-e | --env)
		ENVIRONMENT_FILE="$2"
		shift 2
		;;
	-*)
		echo "$usage" >&2
		exit 1
		;;
	*) break ;;
	esac
done

if [ "$YES" != "1" ]; then
	echo >&2 "This script will REMOVE netdata from your system."
	echo >&2 "Run it again with --yes to do it."
	exit 1
fi

if [[ $EUID -ne 0 ]]; then
	echo >&2 "This script SHOULD be run as root or otherwise it won't delete all installed components."
	key="n"
	read -r -s -n 1 -p "Do you want to continue as non-root user [y/n] ? " key
	if [ "$key" != "y" ] && [ "$key" != "Y" ]; then
		exit 1
	fi
fi

# -----------------------------------------------------------------------------

setup_terminal() {
	TPUT_RESET=""
	TPUT_YELLOW=""
	TPUT_WHITE=""
	TPUT_BGRED=""
	TPUT_BGGREEN=""
	TPUT_BOLD=""
	TPUT_DIM=""

	# Is stderr on the terminal? If not, then fail
	test -t 2 || return 1

	if command -v tput 1>/dev/null 2>&1; then
		if [ $(($(tput colors 2>/dev/null))) -ge 8 ]; then
			# Enable colors
			TPUT_RESET="$(tput sgr 0)"
			TPUT_YELLOW="$(tput setaf 3)"
			TPUT_WHITE="$(tput setaf 7)"
			TPUT_BGRED="$(tput setab 1)"
			TPUT_BGGREEN="$(tput setab 2)"
			TPUT_BOLD="$(tput bold)"
			TPUT_DIM="$(tput dim)"
		fi
	fi

	return 0
}
setup_terminal || echo >/dev/null

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

portable_del_group() {
	local groupname="${1}"

	# Check if group exist
	echo >&2 "Removing ${groupname} user group ..."

	# Linux
	if command -v groupdel 1>/dev/null 2>&1; then
		if grep -q "${groupname}" /etc/group; then
		  run groupdel "${groupname}" && return 0
		else
		  echo >&2 "Group ${groupname} already removed in a previous step."
		  run_ok
		fi
	fi

	# mac OS
	if command -v dseditgroup 1> /dev/null 2>&1; then
		if dseditgroup -o read netdata 1> /dev/null 2>&1; then
			run dseditgroup -o delete "${groupname}" && return 0
		else
			echo >&2 "Could not find group ${groupname}, nothing to do"
		fi
	fi

	echo >&2 "Group ${groupname} was not automatically removed, you might have to remove it manually"
	return 1
}

portable_del_user() {
	local username="${1}"
	echo >&2 "Deleting ${username} user account ..."

	# Linux
	if command -v userdel 1>/dev/null 2>&1; then
		run userdel -f "${username}" && return 0
	fi

	# mac OS
	if command -v sysadminctl 1>/dev/null 2>&1; then
		run sysadminctl -deleteUser "${username}" && return 0
	fi

	echo >&2 "User ${username} could not be deleted from system, you might have to remove it manually"
	return 1
}

portable_del_user_from_group() {
	local groupname="${1}" username="${2}"

	# username is not in group
	echo >&2 "Deleting ${username} user from ${groupname} group ..."

	# Linux
	if command -v gpasswd 1>/dev/null 2>&1; then
		run gpasswd -d "netdata" "${group}" && return 0
	fi

	# FreeBSD
	if command -v pw 1>/dev/null 2>&1; then
		run pw groupmod "${groupname}" -d "${username}" && return 0
	fi

	# BusyBox
	if command -v delgroup 1>/dev/null 2>&1; then
		run delgroup "${username}" "${groupname}" && return 0
	fi

	# mac OS
	if command -v dseditgroup 1> /dev/null 2>&1; then
		run dseditgroup -o delete -u "${username}" "${groupname}" && return 0
	fi

	echo >&2 "Failed to delete user ${username} from group ${groupname} !"
	return 1
}

quit_msg() {
	echo
	if [ "$FILE_REMOVAL_STATUS" -eq 0 ]; then
		echo >&2 "Something went wrong :("
	else
		echo >&2 "Netdata files were successfully removed from your system"
	fi
}

user_input() {
	TEXT="$1"
	if [ "${INTERACTIVITY}" = "-i" ]; then
		read -r -p "$TEXT" >&2
	fi
}

rm_file() {
	FILE="$1"
	if [ -f "${FILE}" ]; then
		run rm -v ${INTERACTIVITY} "${FILE}"
	fi
}

rm_dir() {
	DIR="$1"
	if [ -n "$DIR" ] && [ -d "$DIR" ]; then
		user_input "Press ENTER to recursively delete directory '$DIR' > "
		run rm -v -f -R "${DIR}"
	fi
}

netdata_pids() {
	local p myns ns
	myns="$(readlink /proc/self/ns/pid 2>/dev/null)"
	for p in \
		$(cat /var/run/netdata.pid 2>/dev/null) \
		$(cat /var/run/netdata/netdata.pid 2>/dev/null) \
		$(pidof netdata 2>/dev/null); do

		ns="$(readlink "/proc/${p}/ns/pid" 2>/dev/null)"
		#shellcheck disable=SC2002
		if [ -z "${myns}" ] || [ -z "${ns}" ] || [ "${myns}" = "${ns}" ]; then
			name="$(cat "/proc/${p}/stat" 2>/dev/null | cut -d '(' -f 2 | cut -d ')' -f 1)"
			if [ "${name}" = "netdata" ]; then
				echo "${p}"
			fi
		fi
	done
}

trap quit_msg EXIT

#shellcheck source=/dev/null
source "${ENVIRONMENT_FILE}" || exit 1

#### STOP NETDATA
echo >&2 "Stopping a possibly running netdata..."
for p in $(netdata_pids); do
	i=0
	while kill "${p}" 2>/dev/null; do
		if [ "$i" -gt 30 ]; then
			echo >&2 "Forcefully stopping netdata with pid ${p}"
			run kill -9 "${p}"
			run sleep 2
			break
		fi
		sleep 1
		i=$((i + 1))
	done
done
sleep 2

#### REMOVE NETDATA FILES
rm_file /etc/logrotate.d/netdata
rm_file /etc/systemd/system/netdata.service
rm_file /lib/systemd/system/netdata.service
rm_file /usr/lib/systemd/system/netdata.service
rm_file /etc/init.d/netdata
rm_file /etc/periodic/daily/netdata-updater
rm_file /etc/cron.daily/netdata-updater

if [ -n "${NETDATA_PREFIX}" ] && [ -d "${NETDATA_PREFIX}" ]; then
	rm_dir "${NETDATA_PREFIX}"
else
	rm_file "/usr/sbin/netdata"
	rm_file "/usr/sbin/netdatacli"
	rm_dir "/usr/share/netdata"
	rm_dir "/usr/libexec/netdata"
	rm_dir "/var/lib/netdata"
	rm_dir "/var/cache/netdata"
	rm_dir "/var/log/netdata"
	rm_dir "/etc/netdata"
fi

FILE_REMOVAL_STATUS=1

#### REMOVE NETDATA USER FROM ADDED GROUPS
if [ -n "$NETDATA_ADDED_TO_GROUPS" ]; then
	user_input "Press ENTER to delete 'netdata' from following groups: '$NETDATA_ADDED_TO_GROUPS' > "
	for group in $NETDATA_ADDED_TO_GROUPS; do
		portable_del_user_from_group "${group}" "netdata"
	done
fi

#### REMOVE USER
user_input "Press ENTER to delete 'netdata' system user > "
portable_del_user "netdata" || :

### REMOVE GROUP
user_input "Press ENTER to delete 'netdata' system group > "
portable_del_group "netdata" || :
