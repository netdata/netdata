#!/bin/bash

LC_ALL=C

# you can set CFLAGS before running installer
CFLAGS="${CFLAGS--O3}"

ME="$0"
DONOTSTART=0
DONOTWAIT=0
NETDATA_PREFIX=
ZLIB_IS_HERE=0

usage() {
	cat <<USAGE

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

        If you get errors about missing zlib,
        but you know it is available,
        you have a broken pkg-config.
        Use this option to allow it continue
        without checking pkg-config.

Netdata will by default be compiled with gcc optimization -O3
If you need to pass different CFLAGS, use something like this:

  CFLAGS="<gcc options>" $ME <installer options>

For the installer to complete successfully, you will need
these packages installed:

   gcc make autoconf automake pkg-config zlib1g-dev

For the plugins, you will at least need:

   curl node

USAGE
}

while [ ! -z "${1}" ]
do
	if [ "$1" = "--install" ]
		then
		NETDATA_PREFIX="${2}/netdata"
		shift 2
	elif [ "$1" = "--zlib-is-really-here" ]
		then
		ZLIB_IS_HERE=1
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

cat <<BANNER

Welcome to netdata!
Nice to see you are giving it a try!

You are about to build and install netdata to your system.

It will be installed at these locations:

  - the daemon    at ${NETDATA_PREFIX}/usr/sbin/netdata
  - config files  at ${NETDATA_PREFIX}/etc/netdata
  - web files     at ${NETDATA_PREFIX}/usr/share/web/netdata
  - plugins       at ${NETDATA_PREFIX}/usr/libexec/netdata
  - cache files   at ${NETDATA_PREFIX}/var/cache/netdata
  - log files     at ${NETDATA_PREFIX}/var/log/netdata

This installer allows you to change the installation path.
Press Control-C and run the same command with --help for help.

BANNER

if [ ! "${UID}" = 0 -o "$1" = "-h" -o "$1" = "--help" ]
	then
	echo >&2
	echo >&2 "You have to run netdata as root."
	echo >&2 "The netdata daemon will drop priviliges"
	echo >&2 "but you have to start it as root."
	echo >&2
	exit 1
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

# reload the profile
[ -f /etc/profile ] && . /etc/profile

build_error() {
	cat <<EOF

^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Sorry! NetData failed to build...

You many need to check these:

1. The package zlib1g-dev has to be installed.

2. You need basic build tools installed, like:

   gcc make autoconf automake pkg-config

3. If your system cannot find ZLIB, although it is installed
   run me with the option:  --zlib-is-really-here

If you still cannot get it to build, ask for help at github:

   https://github.com/firehol/netdata/issues


EOF
	trap - EXIT
	exit 1
}

run() {
	printf >&2 ":-----------------------------------------------------------------------------\n"
	printf >&2 "Running command:\n"
	printf >&2 "\n"
	printf >&2 "%q " "${@}"
	printf >&2 "\n"
	printf >&2 "\n"

	"${@}"
}

if [ ${ZLIB_IS_HERE} -eq 1 ]
	then
	shift
	echo >&2 "ok, assuming zlib is really installed."
	export ZLIB_CFLAGS=" "
	export ZLIB_LIBS="-lz"
fi

trap build_error EXIT

echo >&2 "Running ./autogen.sh ..."
run ./autogen.sh || exit 1

echo >&2 "Running ./configure ..."
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

echo >&2 "Installing netdata ..."
run make install || exit 1

echo >&2 "Adding netdata user group ..."
getent group netdata > /dev/null || run groupadd -r netdata

echo >&2 "Adding netdata user account ..."
getent passwd netdata > /dev/null || run useradd -r -g netdata -c netdata -s /sbin/nologin -d / netdata



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

# debug flags
defdebug=0
NETDATA_DEBUG="$( config_option "debug flags" ${defdebug} )"

# port
defport=19999
NETDATA_PORT="$( config_option "port" ${defport} )"

# directories
NETDATA_CACHE_DIR="$( config_option "cache directory" "${NETDATA_PREFIX}/var/cache/netdata" )"
NETDATA_WEB_DIR="$( config_option "web files directory" "${NETDATA_PREFIX}/usr/share/netdata/web" )"
NETDATA_LOG_DIR="$( config_option "log directory" "${NETDATA_PREFIX}/var/log/netdata" )"
NETDATA_CONF_DIR="$( config_option "config directory" "${NETDATA_PREFIX}/etc/netdata" )"


# -----------------------------------------------------------------------------
# prepare the directories

echo >&2 "Fixing directory permissions for user ${NETDATA_USER}..."
for x in "${NETDATA_WEB_DIR}" "${NETDATA_CONF_DIR}" "${NETDATA_CACHE_DIR}" "${NETDATA_LOG_DIR}"
do
	if [ ! -d "${x}" ]
		then
		echo >&2 "Creating directory '${x}'"
		run mkdir -p "${x}" || exit 1
	fi
	run chown -R "${NETDATA_USER}:${NETDATA_USER}" "${x}" || echo >&2 "WARNING: Cannot change the ownership of the files in directory ${x} to ${NETDATA_USER}..."
	run chmod 0775 "${x}" || echo >&2 "WARNING: Cannot change the permissions of the directory ${x} to 0755..."
done

# fix apps.plugin to be setuid to root
run chown root "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"
run chmod 4755 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"


# -----------------------------------------------------------------------------
# check if we can re-start netdata

if [ ${DONOTSTART} -eq 1 ]
	then
	if [ ! -s "${NETDATA_PREFIX}/etc/netdata/netdata.conf" ]
		then
		echo >&2 "Generating empty config file in: ${NETDATA_PREFIX}/etc/netdata/netdata.conf"
		echo "# Get config from http://127.0.0.1:${NETDATA_PORT}/netdata.conf" >"${NETDATA_PREFIX}/etc/netdata/netdata.conf"
		chown "${NETDATA_USER}" "${NETDATA_PREFIX}/etc/netdata/netdata.conf"
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


printf >&2 "Stopping a (possibly) running netdata..."
ret=0
count=0
while [ $ret -eq 0 ]
do
	if [ $count -gt 15 ]
		then
		echo >&2 "Cannot stop the running netdata."
		exit 1
	fi

	count=$((count + 1))

	pid=$(cat /var/run/netdata/netdata.pid 2>/dev/null)
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


# -----------------------------------------------------------------------------
# run netdata

echo >&2 "Starting netdata..."
run ${NETDATA_PREFIX}/usr/sbin/netdata "${@}"

if [ $? -ne 0 ]
	then
	echo >&2
	echo >&2 "SORRY! FAILED TO START NETDATA!"
	exit 1
else
	echo >&2 "OK. NetData Started!"
fi


# -----------------------------------------------------------------------------
# save a config file, if it is not already there

if [ ! -s "${NETDATA_PREFIX}/etc/netdata/netdata.conf" ]
	then
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

		chown "${NETDATA_USER}" "${NETDATA_PREFIX}/etc/netdata/netdata.conf"
		chmod 0664 "${NETDATA_PREFIX}/etc/netdata/netdata.conf"
	else
		echo >&2 "Cannnot download configuration from netdata daemon using url 'http://localhost:${NETDATA_PORT}/netdata.conf'"
		[ -f "${NETDATA_PREFIX}/etc/netdata/netdata.conf.new" ] && rm "${NETDATA_PREFIX}/etc/netdata/netdata.conf.new"
	fi
fi

cat <<END


-------------------------------------------------------------------------------

ok. NetData is installed and is running.

Hit http://localhost:${NETDATA_PORT}/ from your browser.

To stop netdata, just kill it, with:

  killall netdata

To start it, just run it:

  ${NETDATA_PREFIX}/usr/sbin/netdata

Enjoy!

END

ksm_is_available_but_disabled() {
	cat <<KSM1

INFORMATION:

I see you have kernel memory de-duper (called Kernel Same-page Merging,
or KSM) available, but it is not currently enabled.

To enable it run:

echo 1 >/sys/kernel/mm/ksm/run
echo 1000 >/sys/kernel/mm/ksm/sleep_millisecs

If you enable it, you will save 20-60% of netdata memory.

KSM1
}

ksm_is_not_available() {
	cat <<KSM2

INFORMATION:

I see you do not have kernel memory de-duper (called Kernel Same-page
Merging, or KSM) available.

To enable it, you need a kernel built with CONFIG_KSM=y

If you can have it, you will save 20-60% of netdata memory.

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
