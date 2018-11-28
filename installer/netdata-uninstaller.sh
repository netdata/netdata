#!/usr/bin/env bash
#shellcheck disable=SC2181

# this script will uninstall netdata

# Variables needed by script:
#  - NETDATA_PREFIX
#  - NETDATA_ADDED_TO_GROUPS

if [ "$1" != "--force" ]; then
	echo >&2 "This script will REMOVE netdata from your system."
	echo >&2 "Run it again with --force to do it."
	exit 1
fi

#shellcheck source=/dev/null
source installer/.environment.sh || exit 1
#shellcheck source=/dev/null
source installer/functions.sh || exit 1

echo >&2 "Stopping a possibly running netdata..."
for p in $(stop_all_netdata netdata); do run kill "$p"; done
sleep 2

if [ ! -z "${NETDATA_PREFIX}" ] && [ -d "${NETDATA_PREFIX}" ]; then
	# installation prefix was given

	portable_deletedir_recursively_interactively "${NETDATA_PREFIX}"

else
	# installation prefix was NOT given

	if [ -f "${NETDATA_PREFIX}/usr/sbin/netdata" ]; then
		echo "Deleting ${NETDATA_PREFIX}/usr/sbin/netdata ..."
		run rm -i "${NETDATA_PREFIX}/usr/sbin/netdata"
	fi

	portable_deletedir_recursively_interactively "${NETDATA_PREFIX}/etc/netdata"
	portable_deletedir_recursively_interactively "${NETDATA_PREFIX}/usr/share/netdata"
	portable_deletedir_recursively_interactively "${NETDATA_PREFIX}/usr/libexec/netdata"
	portable_deletedir_recursively_interactively "${NETDATA_PREFIX}/var/lib/netdata"
	portable_deletedir_recursively_interactively "${NETDATA_PREFIX}/var/cache/netdata"
	portable_deletedir_recursively_interactively "${NETDATA_PREFIX}/var/log/netdata"
fi

if [ -f /etc/logrotate.d/netdata ]; then
	echo "Deleting /etc/logrotate.d/netdata ..."
	run rm -i /etc/logrotate.d/netdata
fi

if [ -f /etc/systemd/system/netdata.service ]; then
	echo "Deleting /etc/systemd/system/netdata.service ..."
	run rm -i /etc/systemd/system/netdata.service
fi

if [ -f /lib/systemd/system/netdata.service ]; then
	echo "Deleting /lib/systemd/system/netdata.service ..."
	run rm -i /lib/systemd/system/netdata.service
fi

if [ -f /etc/init.d/netdata ]; then
	echo "Deleting /etc/init.d/netdata ..."
	run rm -i /etc/init.d/netdata
fi

if [ -f /etc/periodic/daily/netdata-updater ]; then
	echo "Deleting /etc/periodic/daily/netdata-updater ..."
	run rm -i /etc/periodic/daily/netdata-updater
fi

if [ -f /etc/cron.daily/netdata-updater ]; then
	echo "Deleting /etc/cron.daily/netdata-updater ..."
	run rm -i /etc/cron.daily/netdata-updater
fi

portable_check_user_exists netdata
if [ $? -eq 0 ]; then
	echo
	echo "You may also want to remove the user netdata"
	echo "by running:"
	echo "   userdel netdata"
fi

portable_check_group_exists netdata >/dev/null
if [ $? -eq 0 ]; then
	echo
	echo "You may also want to remove the group netdata"
	echo "by running:"
	echo "   groupdel netdata"
fi

for g in ${NETDATA_ADDED_TO_GROUPS}; do
	portable_check_group_exists "$g" >/dev/null
	if [ $? -eq 0 ]; then
		echo
		echo "You may also want to remove the netdata user from the $g group"
		echo "by running:"
		echo "   gpasswd -d netdata $g"
	fi
done
