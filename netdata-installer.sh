#!/bin/bash

LC_ALL=C

cat <<BANNER


This script will build and install netdata to your system.

By default netdata will be installed as any other package.
If you want to install it to another directory, run me
with the option:

$0 --install /path/to/install

A new directory called netdata will be created as

/path/to/install/netdata


BANNER

if [ ! "${UID}" = 0 -o "$1" = "-h" -o "$1" = "--help" ]
	then
	echo >&2
	echo >&2 "You have to run netdata are root."
	echo >&2 "The netdata daemon will drop priviliges"
	echo >&2 "but you have to start it as root."
	echo >&2
	exit 1
fi

NETDATA_PREFIX=
if [ "$1" = "--install" ]
	then
	NETDATA_PREFIX="${2}/netdata"
	shift 2
fi

if [ ! -z "${NETDATA_PREFIX}" ]
	then
	read -p "Press ENTER to build and install netdata to '${NETDATA_PREFIX}' > "
else
	read -p "Press ENTER to build and install netdata to your system > "
fi

# reload the profile
[ -f /etc/profile ] && . /etc/profile

build_error() {
	cat <<EOF

^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

We are very sorry! NetData failed to build...

You many need to check these:

1. The package zlib1g-dev has to be installed.

2. You need basic build tools installed, like:

   gcc autoconf automake pgk-config

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

if [ "$1" = "--zlib-is-really-here" ]
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
	CFLAGS="-O3" || exit 1

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
# stop a running netdata

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
	run killall netdata 2>/dev/null
	ret=$?
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
