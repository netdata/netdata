#!/usr/bin/env bash

source_dir="$(pwd)"
installer_dir="$(dirname "${0}")"

if [ "${source_dir}" != "${installer_dir}" -a "${installer_dir}" != "." ]
    then
    echo >&2 "Warning: you are currently in '${source_dir}' but the installer is in '${installer_dir}'."
fi


# -----------------------------------------------------------------------------
# reload the user profile

[ -f /etc/profile ] && . /etc/profile

# make sure /etc/profile does not change our current directory
cd "${source_dir}" || exit 1


# -----------------------------------------------------------------------------
# load the required functions

if [ -f "${installer_dir}/installer/functions.sh" ]
    then
    source "${installer_dir}/installer/functions.sh" || exit 1
else
    source "${source_dir}/installer/functions.sh" || exit 1
fi

run_logfile="netdata-plugin-java-daemon-installer.log"

umask 002

# Be nice on production environments
renice 19 $$ >/dev/null 2>/dev/null

ME="$0"
DONOTSTART=0
DONOTWAIT=0
NETDATA_PREFIX=
NETDATA_CONFIGURE_OPTIONS="${NETDATA_CONFIGURE_OPTIONS-}"

usage() {
    netdata_banner "installer command line options"
    cat <<USAGE

${ME} <installer options>

Valid <installer options> are:

   --install /PATH/TO/INSTALL

        If you give: --install /opt
        netdata-plugin-java-daemon will be installed in /opt/netdata
        Use the same value you used for netdata-installer.sh.

   --dont-start-it

        Do not (re)start netdata.
        Just install it.

   --dont-wait

        Do not wait for the user to press ENTER.
        Start immediately building it.

For the installer to complete successfully, you will need
these packages installed:

   netdata
   jdk8
   jre8

USAGE
}

# Parse command line
while [ ! -z "${1}" ]
do
    if [ "$1" = "--install" ]
        then
        NETDATA_PREFIX="${2}/netdata"
        shift 2
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
    elif [ "$1" = "get_git_config_signatures" ]
        then
        get_git_config_signatures && exit 0
        exit 1
    else
        echo >&2
        echo >&2 "ERROR:"
        echo >&2 "I cannot understand option '$1'."
        usage
        exit 1
    fi
done

netdata_banner "java plugin daemon with jmx monitoring"
cat <<BANNER1

  You are about to build and install netdata-plugin-java-daemon to your system.
  Please make shure you called the installer with the same privileges and options you used to install netdata.

  It will be installed at these locations:

   - executable     at ${TPUT_CYAN}${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/java.d.plugin${TPUT_RESET}
   - jar file       at ${TPUT_CYAN}${NETDATA_PREFIX}/usr/netdata-plugin-java-daemon/java-daemon.jar${TPUT_RESET}
   - config files   in ${TPUT_CYAN}${NETDATA_PREFIX}/etc/netdata${TPUT_RESET}
BANNER1

cat <<BANNER3

  This installer allows you to change the installation path.
  Press Control-C and run the same command with --help for help.

BANNER3

if [ ! -x "${NETDATA_PREFIX}/usr/sbin/netdata" ]
    then
    cat <<NONETDATA

  ${TPUT_RED}${TPUT_BOLD}Sorry! This will fail!${TPUT_RESET}

  Could not find netdata executable at ${NETDATA_PREFIX}/usr/sbin/netdata.

  Make shure you called the installer with the same privileges and options you used to install netdata.
  If you did not install netdata please do so. This program won't work without it.

NONETDATA
    exit 1
    fi


if [ "${UID}" -ne 0 ]
    then
    # Not running as root
    if [ -z "${NETDATA_PREFIX}" ]
        then
        netdata_banner "wrong command line options!"
        cat <<NONROOTNOPREFIX
  
  ${TPUT_RED}${TPUT_BOLD}Sorry! This will fail!${TPUT_RESET}
  
  You are attempting to install netdata-plugin-java-daemon as non-root, but you plan
  to install it in system paths.
  
  Please set an installation prefix, like this:
  
      $0 ${@} --install /tmp
  
  or, run the installer as root:
  
      sudo $0 ${@}
  
  You must install it the same way you installed netdata.
  If not netdata will not find it.
  
NONROOTNOPREFIX
        exit 1

    else
        cat <<NONROOT
 
  ${TPUT_RED}${TPUT_BOLD}IMPORTANT${TPUT_RESET}:
  You are about to install netdata-plugin-java-daemon as a non-root user.
  
  If you installing netdata-plugin-java-daemon permanently on your system, run
  the installer like this:
  
     ${TPUT_YELLOW}${TPUT_BOLD}sudo $0 ${@}${TPUT_RESET}

NONROOT
    fi
fi

if [ ${DONOTWAIT} -eq 0 ]
    then
    if [ ! -z "${NETDATA_PREFIX}" ]
        then
        eval "read >&2 -ep \$'\001${TPUT_BOLD}${TPUT_GREEN}\002Press ENTER to build and install netdata-plugin-java-daemon to \'\001${TPUT_CYAN}\002${NETDATA_PREFIX}\001${TPUT_YELLOW}\002\'\001${TPUT_RESET}\002 > ' -e -r REPLY"
    else
        eval "read >&2 -ep \$'\001${TPUT_BOLD}${TPUT_GREEN}\002Press ENTER to build and install netdata-plugin-java-daemon to your system\001${TPUT_RESET}\002 > ' -e -r REPLY"
    fi
fi

build_error() {
    netdata_banner "sorry, it failed to build..."
    cat <<EOF

^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Sorry! netdata-plugin-java-daemon failed to build...

If you cannot figure out why, ask for help at github:

   https://github.com/simonnagl/netdata-plugin-java-daemon/issues


EOF
    trap - EXIT
    exit 1
}

# -----------------------------------------------------------------------------
progress "Cleanup compilation directory"

[ -d target ] && run ./mvnw clean

# -----------------------------------------------------------------------------
progress "Compile and package netdata-plugin-java-daemon"

run ./mvnw -T 1C package || build_error

# -----------------------------------------------------------------------------
progress "Install netdata-plugin-java-daemon"

# Create directory for jar if necessary
[ -d "${NETDATA_PREFIX}/usr/libexec/netdata-plugin-java-daemon" ] || run mkdir -p "${NETDATA_PREFIX}/usr/libexec/netdata-plugin-java-daemon"
# Copy the jar
run cp target/java-daemon-*.jar "${NETDATA_PREFIX}/usr/libexec/netdata-plugin-java-daemon/java-daemon.jar"

# Write the executable
run echo "exec java -Djava.util.logging.SimpleFormatter.format='%1\$tF %1\$TT: java.d: %4\$s: %3\$s: %5\$s%6\$s%n' -jar ${NETDATA_PREFIX}/usr/libexec/netdata-plugin-java-daemon/java-daemon.jar \$@" \
        > ${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/java.d.plugin
run chmod 0755 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/java.d.plugin"

# Copy configuration
NETDATA_CONFIG_DIR="${NETDATA_PREFIX}/etc/netdata/"
[ -d "${NETDATA_CONFIG_DIR}" ] || run mkdir -p "${NETDATA_CONFIG_DIR}"
[ -f "${NETDATA_CONFIG_DIR}/java.d.conf" ] || run cp "config/java.d.conf" "${NETDATA_CONFIG_DIR}"

# Copy plugin configuration files if not present.
[ -d "${NETDATA_CONFIG_DIR}/java.d" ] || run mkdir -p "${NETDATA_CONFIG_DIR}/java.d"
for filename in $(ls config/java.d)
do
    [ -f "${NETDATA_CONFIG_DIR}/java.d/${filename}" ] || run cp "config/java.d/${filename}" "${NETDATA_CONFIG_DIR}/java.d/"
done

# -----------------------------------------------------------------------------
# check if we can re-start netdata

started=0
if [ ${DONOTSTART} -ne 1 ]
    then
    restart_netdata ${NETDATA_PREFIX}/usr/sbin/netdata "${@}"
    if [ $? -ne 0 ]
        then
        echo >&2
        echo >&2 "SORRY! FAILED TO START NETDATA!"
        echo >&2
        exit 1
    fi

    started=1
    echo >&2 "OK. NetData Started!"
    echo >&2
fi

# -----------------------------------------------------------------------------
progress "Generate netdata-plugin-java-daemon-uninstaller.sh"

cat >netdata-plugin-java-daemon-uninstaller.sh <<UNINSTALL
#!/usr/bin/env bash

# this script will uninstall netdata

if [ "\$1" != "--force" ]
    then
    echo >&2 "This script will REMOVE netdata-plugin-java-daeimon from your system."
    echo >&2 "Run it again with --force to do it."
    exit 1
fi

deletedir() {
    if [ ! -z "\$1" -a -d "\$1" ]
        then
        echo
        echo "Deleting directory '\$1' ..."
        rm -r "\$1"
    fi
}

deletedir "${NETDATA_PREFIX}/usr/libexec/netdata-plugin-java-daemon"
rm -f ${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/java.d.plugin
rm -f ${NETDATA_PREFIX}/etc/netdata/java.d.conf
deletedir "${NETDATA_PREFIX}/etc/netdata/java.d"

UNINSTALL
chmod 750 netdata-plugin-java-daemon-uninstaller.sh

# -----------------------------------------------------------------------------
progress "Basic netdata-plugin-java-daemon instructions"

cat <<END

netdata-plugin-java-daemon must be called by netdata.
You may need to restart netdata.

END
echo >&2 "Uninstall script generated: ${TPUT_RED}${TPUT_BOLD}./netdata-plugin-java-daemon-uninstaller.sh${TPUT_RESET}"

# -----------------------------------------------------------------------------
echo >&2
progress "We are done!"

if [ ${started} -eq 1 ]
    then
    netdata_banner "is installed and running now!"
else
    netdata_banner "is installed now!"
fi

echo >&2 "  enjoy real-time performance and health monitoring..."
echo >&2 
exit 0
