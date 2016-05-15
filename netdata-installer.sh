#!/bin/bash

# reload the user profile
[ -f /etc/profile ] && . /etc/profile

# fix PKG_CHECK_MODULES error
if [ -d /usr/share/aclocal ]
then
        ACLOCAL_PATH=${ACLOCAL_PATH-/usr/share/aclocal}
        export ACLOCAL_PATH
fi

LC_ALL=C
umask 022

# you can set CFLAGS before running installer
CFLAGS="${CFLAGS--O3}"

# keep a log of this command
printf "\n# " >>netdata-installer.log
date >>netdata-installer.log
printf "CFLAGS=\"%s\" " "${CFLAGS}" >>netdata-installer.log
printf "%q " "$0" "${@}" >>netdata-installer.log
printf "\n" >>netdata-installer.log

ME="$0"
DONOTSTART=0
DONOTWAIT=0
NETDATA_PREFIX=
LIBS_ARE_HERE=0

usage() {
	cat <<-USAGE

	${ME} <installer options>

	Valid <installer options> are:

	   --install /PATH/TO/INSTALL

			If your give: --install /opt
			netdata will be installed in /opt/netdata

	   --dont-start-it

			Do not (re)start netdata.
			Just install it.

	   --dont-wait

			Do not wait for the user to press ENTER.
			Start immediately building it.

	   --zlib-is-really-here
	   --libs-are-really-here

			If you get errors about missing zlib,
			or libuuid but you know it is available,
			you have a broken pkg-config.
			Use this option to allow it continue
			without checking pkg-config.

	Netdata will by default be compiled with gcc optimization -O3
	If you need to pass different CFLAGS, use something like this:

	  CFLAGS="<gcc options>" ${ME} <installer options>

	For the installer to complete successfully, you will need
	these packages installed:

	   gcc make autoconf automake pkg-config zlib1g-dev (or zlib-devel)
	   uuid-dev (or libuuid-devel)

	For the plugins, you will at least need:

	   curl nodejs

USAGE
}

while [ ! -z "${1}" ]
do
	if [ "$1" = "--install" ]
		then
		NETDATA_PREFIX="${2}/netdata"
		shift 2
	elif [ "$1" = "--zlib-is-really-here" -o "$1" = "--libs-are-really-here" ]
		then
		LIBS_ARE_HERE=1
		shift 1
	elif [ "$1" = "--dont-start-it" ]
		then
		DONOTSTART=1
		shift 1
	elif [ "$1" = "--dont-wait" ]
		then
		DONOTWAIT=1
		shift 1
	elif [ "$1" = "--help" -o "$1" = "-h" ]
		then
		usage
		exit 1
	else
		echo >&2
		echo >&2 "ERROR:"
		echo >&2 "I cannot understand option '$1'."
		usage
		exit 1
	fi
done

cat <<-BANNER

	Welcome to netdata!
	Nice to see you are giving it a try!

	You are about to build and install netdata to your system.

	It will be installed at these locations:

	  - the daemon    at ${NETDATA_PREFIX}/usr/sbin/netdata
	  - config files  at ${NETDATA_PREFIX}/etc/netdata
	  - web files     at ${NETDATA_PREFIX}/usr/share/netdata
	  - plugins       at ${NETDATA_PREFIX}/usr/libexec/netdata
	  - cache files   at ${NETDATA_PREFIX}/var/cache/netdata
	  - db files      at ${NETDATA_PREFIX}/var/lib/netdata
	  - log files     at ${NETDATA_PREFIX}/var/log/netdata
	  - pid file      at ${NETDATA_PREFIX}/var/run

	This installer allows you to change the installation path.
	Press Control-C and run the same command with --help for help.

BANNER

if [ "${UID}" -ne 0 ]
	then
	if [ -z "${NETDATA_PREFIX}" ]
		then
		cat <<-NONROOTNOPREFIX

		Sorry! This will fail!

		You are attempting to install netdata as non-root, but you plan to install it
		in system paths.

		Please set an installation prefix, like this:

			$0 ${@} --install /tmp

		or, run the installer as root:

			sudo $0 ${@}

		We suggest to install it as root, or certain data collectors will not be able
		to work. Netdata drops root privileges when running. So, if you plan to keep
		it, install it as root to get the full functionality.

NONROOTNOPREFIX
		exit 1

	else
		cat <<-NONROOT

		IMPORTANT:
		You are about to install netdata as a non-root user.
		Netdata will work, but a few data collection modules that
		require root access will fail.

		If you installing permanently on your system, run the
		installer like this:

			sudo $0 ${@}

NONROOT
	fi
fi

have_autotools=
if [ "$(type autoreconf 2> /dev/null)" ]
then
	autoconf_maj_min() {
		local maj min IFS=.-

		maj=$1
		min=$2

		set -- $(autoreconf -V | sed -ne '1s/.* \([^ ]*\)$/\1/p')
		eval $maj=\$1 $min=\$2
	}
	autoconf_maj_min AMAJ AMIN

	if [ "$AMAJ" -gt 2 ]
	then
		have_autotools=Y
	elif [ "$AMAJ" -eq 2 -a "$AMIN" -ge 60 ]
	then
		have_autotools=Y
	else
		echo "Found autotools $AMAJ.$AMIN"
	fi
else
	echo "No autotools found"
fi

if [ ! "$have_autotools" ]
then
	if [ -f configure ]
	then
		echo "Will skip autoreconf step"
	else
		cat <<-"EOF"

		-------------------------------------------------------------------------------
		autotools 2.60 or later is required

		Sorry, you do not seem to have autotools 2.60 or later, which is
		required to build from the git sources of netdata.

		You can either install a suitable version of autotools and automake
		or download a netdata package which does not have these dependencies.

		Source packages where autotools have already been run are available
		here:
			   https://firehol.org/download/netdata/

		The unsigned/master folder tracks the head of the git tree and released
		packages are also available.
EOF
		exit 1
	fi
fi

if [ ${DONOTWAIT} -eq 0 ]
	then
	if [ ! -z "${NETDATA_PREFIX}" ]
		then
		read -p "Press ENTER to build and install netdata to '${NETDATA_PREFIX}' > "
	else
		read -p "Press ENTER to build and install netdata to your system > "
	fi
fi

build_error() {
	cat <<-EOF

	^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

	Sorry! NetData failed to build...

	You many need to check these:

	1. The package uuid-dev (or libuuid-devel) has to be installed.

	   If your system cannot find libuuid, although it is installed
	   run me with the option:  --libs-are-really-here

	2. The package zlib1g-dev (or zlib-devel) has to be installed.

	   If your system cannot find zlib, although it is installed
	   run me with the option:  --libs-are-really-here

	3. You need basic build tools installed, like:

	   gcc make autoconf automake pkg-config

	   Autoconf version 2.60 or higher is required.

	If you still cannot get it to build, ask for help at github:

	   https://github.com/firehol/netdata/issues


EOF
	trap - EXIT
	exit 1
}

run() {
	printf >>netdata-installer.log "# "
	printf >>netdata-installer.log "%q " "${@}"
	printf >>netdata-installer.log " ... "

	printf >&2 "\n"
	printf >&2 ":-----------------------------------------------------------------------------\n"
	printf >&2 "Running command:\n"
	printf >&2 "\n"
	printf >&2 "%q " "${@}"
	printf >&2 "\n"

	"${@}"

	local ret=$?
	if [ ${ret} -ne 0 ]
		then
		printf >>netdata-installer.log "FAILED!\n"
	else
		printf >>netdata-installer.log "OK\n"
	fi

	return ${ret}
}

if [ ${LIBS_ARE_HERE} -eq 1 ]
	then
	shift
	echo >&2 "ok, assuming libs are really installed."
	export ZLIB_CFLAGS=" "
	export ZLIB_LIBS="-lz"
	export UUID_CFLAGS=" "
	export UUID_LIBS="-luuid"
fi

trap build_error EXIT

if [ "$have_autotools" ]
then
	run ./autogen.sh || exit 1
fi

run ./configure \
	--prefix="${NETDATA_PREFIX}/usr" \
	--sysconfdir="${NETDATA_PREFIX}/etc" \
	--localstatedir="${NETDATA_PREFIX}/var" \
	--with-zlib --with-math --with-user=netdata \
	CFLAGS="${CFLAGS}" || exit 1

# remove the build_error hook
trap - EXIT

if [ -f src/netdata ]
	then
	echo >&2 "Cleaning a possibly old compilation ..."
	run make clean
fi

echo >&2 "Compiling netdata ..."
run make || exit 1

# backup user configurations
installer_backup_suffix="${PID}.${RANDOM}"
for x in apps_groups.conf charts.d.conf
do
	if [ -f "${NETDATA_PREFIX}/etc/netdata/${x}" ]
		then
		cp -p "${NETDATA_PREFIX}/etc/netdata/${x}" "${NETDATA_PREFIX}/etc/netdata/${x}.installer_backup.${installer_backup_suffix}"

	elif [ -f "${NETDATA_PREFIX}/etc/netdata/${x}.installer_backup.${installer_backup_suffix}" ]
		then
		rm -f "${NETDATA_PREFIX}/etc/netdata/${x}.installer_backup.${installer_backup_suffix}"
	fi
done

echo >&2 "Installing netdata ..."
run make install || exit 1

# restore user configurations
for x in apps_groups.conf charts.d.conf
do
	if [ -f "${NETDATA_PREFIX}/etc/netdata/${x}.installer_backup.${installer_backup_suffix}" ]
		then
		cp -p "${NETDATA_PREFIX}/etc/netdata/${x}.installer_backup.${installer_backup_suffix}" "${NETDATA_PREFIX}/etc/netdata/${x}"
	fi
done

NETDATA_ADDED_TO_DOCKER=0
if [ ${UID} -eq 0 ]
	then
	getent group netdata > /dev/null
	if [ $? -ne 0 ]
		then
		echo >&2 "Adding netdata user group ..."
		run groupadd -r netdata
	fi

	getent passwd netdata > /dev/null
	if [ $? -ne 0 ]
		then
		echo >&2 "Adding netdata user account ..."
		run useradd -r -g netdata -c netdata -s /sbin/nologin -d / netdata
	fi

	getent group docker > /dev/null
	if [ $? -eq 0 ]
		then
		# find the users in the docker group
		docker=$(getent group docker | cut -d ':' -f 4)
		if [[ ",${docker}," =~ ,netdata, ]]
			then
			# netdata is already there
			:
		else
			# netdata is not in docker group
			echo >&2 "Adding netdata user to the docker group (needed to get container names) ..."
			run usermod -a -G docker netdata
		fi
		# let the uninstall script know
		NETDATA_ADDED_TO_DOCKER=1
	fi

	if [ -d /etc/logrotate.d -a ! -f /etc/logrotate.d/netdata ]
		then
		echo >&2 "Adding netdata logrotate configuration ..."
		run cp system/netdata.logrotate /etc/logrotate.d/netdata
	fi
fi


# -----------------------------------------------------------------------------
# load options from the configuration file

# create an empty config if it does not exist
[ ! -f "${NETDATA_PREFIX}/etc/netdata/netdata.conf" ] && touch "${NETDATA_PREFIX}/etc/netdata/netdata.conf"

# function to extract values from the config file
config_option() {
	local key="${1}" value="${2}" line=

	if [ -s "${NETDATA_PREFIX}/etc/netdata/netdata.conf" ]
		then
		line="$( grep "^[[:space:]]*${key}[[:space:]]*=[[:space:]]*" "${NETDATA_PREFIX}/etc/netdata/netdata.conf" | head -n 1 )"
		[ ! -z "${line}" ] && value="$( echo "${line}" | cut -d '=' -f 2 | sed -e "s/^[[:space:]]\+//g" -e "s/[[:space:]]\+$//g" )"
	fi

	echo "${value}"
}

# user
defuser="netdata"
[ ! "${UID}" = "0" ] && defuser="${USER}"
NETDATA_USER="$( config_option "run as user" "${defuser}" )"

NETDATA_WEB_USER="$( config_option "web files owner" "${defuser}" )"
NETDATA_WEB_GROUP="$( config_option "web files group" "${NETDATA_WEB_USER}" )"

# debug flags
defdebug=0
NETDATA_DEBUG="$( config_option "debug flags" ${defdebug} )"

# port
defport=19999
NETDATA_PORT="$( config_option "port" ${defport} )"

# directories
NETDATA_LIB_DIR="$( config_option "lib directory" "${NETDATA_PREFIX}/var/lib/netdata" )"
NETDATA_CACHE_DIR="$( config_option "cache directory" "${NETDATA_PREFIX}/var/cache/netdata" )"
NETDATA_WEB_DIR="$( config_option "web files directory" "${NETDATA_PREFIX}/usr/share/netdata/web" )"
NETDATA_LOG_DIR="$( config_option "log directory" "${NETDATA_PREFIX}/var/log/netdata" )"
NETDATA_CONF_DIR="$( config_option "config directory" "${NETDATA_PREFIX}/etc/netdata" )"
NETDATA_BIND="$( config_option "bind socket to IP" "*" )"
NETDATA_RUN_DIR="${NETDATA_PREFIX}/var/run"


# -----------------------------------------------------------------------------
# prepare the directories

# this is needed if NETDATA_PREFIX is not empty
if [ ! -d "${NETDATA_RUN_DIR}" ]
	then
	mkdir -p "${NETDATA_RUN_DIR}" || exit 1
fi

echo >&2
echo >&2 "Fixing directories (user: ${NETDATA_USER})..."
for x in "${NETDATA_WEB_DIR}" "${NETDATA_CONF_DIR}" "${NETDATA_CACHE_DIR}" "${NETDATA_LOG_DIR}" "${NETDATA_LIB_DIR}"
do
	if [ ! -d "${x}" ]
		then
		echo >&2 "Creating directory '${x}'"
		run mkdir -p "${x}" || exit 1
	fi

	if [ ${UID} -eq 0 ]
		then
		if [ "${x}" = "${NETDATA_WEB_DIR}" ]
			then
			run chown -R "${NETDATA_WEB_USER}:${NETDATA_WEB_GROUP}" "${x}" || echo >&2 "WARNING: Cannot change the ownership of the files in directory ${x} to ${NETDATA_WEB_USER}:${NETDATA_WEB_GROUP}..."
		else
			run chown -R "${NETDATA_USER}:${NETDATA_USER}" "${x}" || echo >&2 "WARNING: Cannot change the ownership of the files in directory ${x} to ${NETDATA_USER}..."
		fi
	fi

	run chmod 0755 "${x}" || echo >&2 "WARNING: Cannot change the permissions of the directory ${x} to 0755..."
done

if [ ${UID} -eq 0 ]
	then
	run chown root "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"
	run chmod 0755 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"
	run setcap cap_dac_read_search,cap_sys_ptrace+ep "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"
	if [ $? -ne 0 ]
		then
		# fix apps.plugin to be setuid to root
		run chown root "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"
		run chmod 4755 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"
	fi
fi

# -----------------------------------------------------------------------------
# check if we can re-start netdata

if [ ${DONOTSTART} -eq 1 ]
	then
	if [ ! -s "${NETDATA_PREFIX}/etc/netdata/netdata.conf" ]
		then
		echo >&2 "Generating empty config file in: ${NETDATA_PREFIX}/etc/netdata/netdata.conf"
		echo "# Get config from http://127.0.0.1:${NETDATA_PORT}/netdata.conf" >"${NETDATA_PREFIX}/etc/netdata/netdata.conf"

		if [ "${UID}" -eq 0 ]
			then
			chown "${NETDATA_USER}" "${NETDATA_PREFIX}/etc/netdata/netdata.conf"
		fi
		chmod 0664 "${NETDATA_PREFIX}/etc/netdata/netdata.conf"
	fi
	echo >&2 "OK. It is now installed and ready."
	exit 0
fi

# -----------------------------------------------------------------------------
# stop a running netdata

isnetdata() {
	[ -z "$1" -o ! -f "/proc/$1/stat" ] && return 1
	[ "$(cat "/proc/$1/stat" | cut -d '(' -f 2 | cut -d ')' -f 1)" = "netdata" ] && return 0
	return 1
}


echo >&2
echo >&2 "-------------------------------------------------------------------------------"
echo >&2
printf >&2 "Stopping a (possibly) running netdata..."
ret=0
count=0
while [ $ret -eq 0 ]
do
	if [ $count -gt 30 ]
		then
		echo >&2 "Cannot stop the running netdata."
		exit 1
	fi

	count=$((count + 1))

	pid=$(cat "${NETDATA_RUN_DIR}/netdata.pid" 2>/dev/null)
	# backwards compatibility
	[ -z "${pid}" ] && pid=$(cat /var/run/netdata.pid 2>/dev/null)
	[ -z "${pid}" ] && pid=$(cat /var/run/netdata/netdata.pid 2>/dev/null)
	
	isnetdata $pid || pid=
	if [ ! -z "${pid}" ]
		then
		run kill $pid 2>/dev/null
		ret=$?
	else
		run killall netdata 2>/dev/null
		ret=$?
	fi

	test $ret -eq 0 && printf >&2 "." && sleep 2
done
echo >&2
echo >&2


# -----------------------------------------------------------------------------
# run netdata

echo >&2 "Starting netdata..."
run ${NETDATA_PREFIX}/usr/sbin/netdata -pidfile ${NETDATA_RUN_DIR}/netdata.pid "${@}"

if [ $? -ne 0 ]
	then
	echo >&2
	echo >&2 "SORRY! FAILED TO START NETDATA!"
	exit 1
else
	echo >&2 "OK. NetData Started!"
fi
echo >&2


# -----------------------------------------------------------------------------
# save a config file, if it is not already there

if [ ! -s "${NETDATA_PREFIX}/etc/netdata/netdata.conf" ]
	then
	echo >&2
	echo >&2 "-------------------------------------------------------------------------------"
	echo >&2
	echo >&2 "Downloading default configuration from netdata..."
	sleep 5

	# remove a possibly obsolete download
	[ -f "${NETDATA_PREFIX}/etc/netdata/netdata.conf.new" ] && rm "${NETDATA_PREFIX}/etc/netdata/netdata.conf.new"

	# disable a proxy to get data from the local netdata
	export http_proxy=
	export https_proxy=

	# try wget
	wget 2>/dev/null -O "${NETDATA_PREFIX}/etc/netdata/netdata.conf.new" "http://localhost:${NETDATA_PORT}/netdata.conf"
	ret=$?

	# try curl
	if [ $ret -ne 0 -o ! -s "${NETDATA_PREFIX}/etc/netdata/netdata.conf.new" ]
		then
		curl -s -o "${NETDATA_PREFIX}/etc/netdata/netdata.conf.new" "http://localhost:${NETDATA_PORT}/netdata.conf"
		ret=$?
	fi

	if [ $ret -eq 0 -a -s "${NETDATA_PREFIX}/etc/netdata/netdata.conf.new" ]
		then
		mv "${NETDATA_PREFIX}/etc/netdata/netdata.conf.new" "${NETDATA_PREFIX}/etc/netdata/netdata.conf"
		echo >&2 "New configuration saved for you to edit at ${NETDATA_PREFIX}/etc/netdata/netdata.conf"

		if [ "${UID}" -eq 0 ]
			then
			chown "${NETDATA_USER}" "${NETDATA_PREFIX}/etc/netdata/netdata.conf"
		fi
		chmod 0664 "${NETDATA_PREFIX}/etc/netdata/netdata.conf"
	else
		echo >&2 "Cannnot download configuration from netdata daemon using url 'http://localhost:${NETDATA_PORT}/netdata.conf'"
		[ -f "${NETDATA_PREFIX}/etc/netdata/netdata.conf.new" ] && rm "${NETDATA_PREFIX}/etc/netdata/netdata.conf.new"
	fi
fi

# -----------------------------------------------------------------------------
# Check for KSM

ksm_is_available_but_disabled() {
	cat <<-KSM1

	-------------------------------------------------------------------------------
	Memory de-duplication instructions

	I see you have kernel memory de-duper (called Kernel Same-page Merging,
	or KSM) available, but it is not currently enabled.

	To enable it run:

	echo 1 >/sys/kernel/mm/ksm/run
	echo 1000 >/sys/kernel/mm/ksm/sleep_millisecs

	If you enable it, you will save 40-60% of netdata memory.

KSM1
}

ksm_is_not_available() {
	cat <<-KSM2

	-------------------------------------------------------------------------------
	Memory de-duplication not present in your kernel

	It seems you do not have kernel memory de-duper (called Kernel Same-page
	Merging, or KSM) available.

	To enable it, you need a kernel built with CONFIG_KSM=y

	If you can have it, you will save 40-60% of netdata memory.

KSM2
}

if [ -f "/sys/kernel/mm/ksm/run" ]
	then
	if [ $(cat "/sys/kernel/mm/ksm/run") != "1" ]
		then
		ksm_is_available_but_disabled
	fi
else
	ksm_is_not_available
fi

# -----------------------------------------------------------------------------
# Check for version.txt

if [ ! -s web/version.txt ]
	then
	cat <<-VERMSG

	-------------------------------------------------------------------------------
	Version update check warning

	The way you downloaded netdata, we cannot find its version. This means the
	Update check on the dashboard, will not work.

	If you want to have version update check, please re-install it
	following the procedure in:

	https://github.com/firehol/netdata/wiki/Installation

VERMSG
fi

# -----------------------------------------------------------------------------
# apps.plugin warning

if [ "${UID}" -ne 0 ]
	then
	cat <<-SETUID_WARNING

	-------------------------------------------------------------------------------
	apps.plugin needs privileges

	Since you have installed netdata as a normal user, to have apps.plugin collect
	all the needed data, you have to give it the access rights it needs, by running
	either of the following sets of commands:

	To run apps.plugin with escalated capabilities:

		sudo chown root:${NETDATA_USER} "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"
		sudo chmod 0750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"
		sudo setcap cap_dac_read_search,cap_sys_ptrace+ep "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"

	or, to run apps.plugin as root:

		sudo chown root "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"
		sudo chmod 4755 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"

	apps.plugin is performing a hard-coded function of data collection for all
	running processes. It cannot be instructed from the netdata daemon to perform
	any task, so it is pretty safe to do this.

SETUID_WARNING
fi

# -----------------------------------------------------------------------------
# Keep un-install info

cat >netdata-uninstaller.sh <<-UNINSTALL
	#!/bin/bash

	# this script will uninstall netdata

	if [ "\$1" != "--force" ]
		then
		echo >&2 "This script will REMOVE netdata from your system."
		echo >&2 "Run it again with --force to do it."
		exit 1
	fi

	echo >&2 "Stopping a possibly running netdata..."
	killall netdata
	sleep 2

	deletedir() {
		if [ ! -z "\$1" -a -d "\$1" ]
			then
			echo
			echo "Deleting directory '\$1' ..."
			rm -I -R "\$1"
		fi
	}

	if [ ! -z "${NETDATA_PREFIX}" -a -d "${NETDATA_PREFIX}" ]
		then
		# installation prefix was given

		deletedir "${NETDATA_PREFIX}"

	else
		# installation prefix was NOT given

		if [ -f "${NETDATA_PREFIX}/usr/sbin/netdata" ]
			then
			echo "Deleting ${NETDATA_PREFIX}/usr/sbin/netdata ..."
			rm -i "${NETDATA_PREFIX}/usr/sbin/netdata"
		fi

		deletedir "${NETDATA_PREFIX}/etc/netdata"
		deletedir "${NETDATA_PREFIX}/usr/share/netdata"
		deletedir "${NETDATA_PREFIX}/usr/libexec/netdata"
		deletedir "${NETDATA_PREFIX}/var/lib/netdata"
		deletedir "${NETDATA_PREFIX}/var/cache/netdata"
		deletedir "${NETDATA_PREFIX}/var/log/netdata"
	fi

	if [ -f /etc/logrotate.d/netdata ]
		then
		echo "Deleting /etc/logrotate.d/netdata ..."
		rm -i /etc/logrotate.d/netdata
	fi

	getent passwd netdata > /dev/null
	if [ $? -eq 0 ]
		then
		echo
		echo "You may also want to remove the user netdata"
		echo "by running:"
		echo "   userdel netdata"
	fi

	getent group netdata > /dev/null
	if [ $? -eq 0 ]
		then
		echo
		echo "You may also want to remove the group netdata"
		echo "by running:"
		echo "   groupdel netdata"
	fi

	getent group docker > /dev/null
	if [ $? -eq 0 -a "${NETDATA_ADDED_TO_DOCKER}" = "1" ]
		then
		echo
		echo "You may also want to remove the netdata user from the docker group"
		echo "by running:"
		echo "   gpasswd -d netdata docker"
	fi

UNINSTALL
chmod 750 netdata-uninstaller.sh

# -----------------------------------------------------------------------------

if [ "${NETDATA_BIND}" = "*" ]
	then
	access="localhost"
else
	access="${NETDATA_BIND}"
fi

cat <<-END


	-------------------------------------------------------------------------------

	OK. NetData is installed and it is running (listening to ${NETDATA_BIND}:${NETDATA_PORT}).

	-------------------------------------------------------------------------------


	Hit http://${access}:${NETDATA_PORT}/ from your browser.

	To stop netdata, just kill it, with:

	  killall netdata

	To start it, just run it:

	  ${NETDATA_PREFIX}/usr/sbin/netdata


	Enjoy!

END
echo >&2 "Uninstall script generated: ./netdata-uninstaller.sh"
