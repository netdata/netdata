#!/usr/bin/env bash
#shellcheck disable=SC2181

# this script will uninstall netdata

# Variables needed by script and taken from '.environment' file:
#  - NETDATA_PREFIX
#  - NETDATA_ADDED_TO_GROUPS

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
	echo "This script will REMOVE netdata from your system."
	echo "Run it again with --yes to do it."
	exit 1
fi

if [[ $EUID -ne 0 ]]; then
	echo "This script SHOULD be run as root or otherwise it won't delete all installed components."
	key="n"
	read -r -s -n 1 -p "Do you want to continue as non-root user [y/n] ? " key
	if [ "$key" != "y" ] && [ "$key" != "Y" ]; then
		exit 1
	fi
fi

function quit_msg() {
	echo
	if [ "$FILE_REMOVAL_STATUS" -eq 0 ]; then
		echo "Something went wrong :("
	else
		echo "Netdata files were successfully removed from your system"
	fi
}

function user_input() {
	TEXT="$1"
	if [ "${INTERACTIVITY}" == "-i" ]; then
		read -r -p "$TEXT" >&2
	fi
}

function rm_file() {
	FILE="$1"
	if [ -f "${FILE}" ]; then
		rm -v ${INTERACTIVITY} "${FILE}"
	fi
}

function rm_dir() {
	DIR="$1"
	if [ -n "$DIR" ] && [ -d "$DIR" ]; then
		user_input "Press ENTER to recursively delete directory '$DIR' > "
		rm -v -f -R "${DIR}"
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
echo "Stopping a possibly running netdata..."
for p in $(netdata_pids); do
	i=0
	while kill "${p}" 2>/dev/null; do
		if [ "$i" -gt 30 ]; then
			echo "Forcefully stopping netdata with pid ${p}"
			kill -9 "${p}"
			sleep 2
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
	rm_dir "/usr/share/netdata"
	rm_dir "/usr/libexec/netdata"
	rm_dir "/var/lib/netdata"
	rm_dir "/var/cache/netdata"
	rm_dir "/var/log/netdata"
	rm_dir "/etc/netdata"
fi

FILE_REMOVAL_STATUS=1

#### REMOVE NETDATA USER & GROUP
if [ -n "$NETDATA_ADDED_TO_GROUPS" ]; then
	user_input "Press ENTER to delete 'netdata' from following groups: '$NETDATA_ADDED_TO_GROUPS' > "
	for group in $NETDATA_ADDED_TO_GROUPS; do
		gpasswd -d netdata "${group}"
	done
fi

user_input "Press ENTER to delete 'netdata' system user > "
userdel -f netdata || :
user_input "Press ENTER to delete 'netdata' system group > "
groupdel -f netdata || :
