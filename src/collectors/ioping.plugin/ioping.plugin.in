#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# This plugin requires a latest version of ioping.
# You can compile it from source, by running me with option: install

export PATH="${PATH}:/sbin:/usr/sbin:/usr/local/sbin:@sbindir_POST@"
export LC_ALL=C

usage="$(basename "$0") [install] [-h] [-e]

where:
    install      install ioping binary
    -e, --env    path to environment file (defaults to '/etc/netdata/.environment'
    -h           show this help text"

INSTALL=0
ENVIRONMENT_FILE="/etc/netdata/.environment"

while :; do
    case "$1" in
    -h | --help)
        echo "$usage" >&2
        exit 1
        ;;
    install)
        INSTALL=1
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

if [ "$INSTALL" == "1" ]
    then
    [ "${UID}" != 0 ] && echo >&2 "Please run me as root. This will install a single binary file: /usr/libexec/netdata/plugins.d/ioping." && exit 1

    source "${ENVIRONMENT_FILE}" || exit 1

    run() {
        printf >&2 " > "
        printf >&2 "%q " "${@}"
        printf >&2 "\n"
        "${@}" || exit 1
    }

    download() {
        local git="$(which git 2>/dev/null || command -v git 2>/dev/null)"
        [ ! -z "${git}" ] && run git clone "${1}" "${2}" && return 0

        echo >&2 "Cannot find 'git' in this system." && exit 1
    }

    tmp=$(mktemp -d /tmp/netdata-ioping-XXXXXX)
    [ ! -d "${NETDATA_PREFIX}/usr/libexec/netdata" ] && run mkdir -p "${NETDATA_PREFIX}/usr/libexec/netdata"

    run cd "${tmp}"

    if [ -d ioping-netdata ]
        then
        run rm -rf ioping-netdata || exit 1
    fi

    download 'https://github.com/netdata/ioping.git' 'ioping-netdata'
    [ $? -ne 0 ] && exit 1
    run cd ioping-netdata || exit 1

    INSTALL_PATH="${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/ioping"

    run make clean
    run make
    run mv ioping "${INSTALL_PATH}"
    run chown root:"${NETDATA_GROUP}" "${INSTALL_PATH}"
    run chmod 4750 "${INSTALL_PATH}"
    echo >&2
    echo >&2 "All done, you have a compatible ioping now at ${INSTALL_PATH}."
    echo >&2

    exit 0
fi

# -----------------------------------------------------------------------------
# logging

PROGRAM_NAME="$(basename "${0}")"

# these should be the same with syslog() priorities
NDLP_EMERG=0   # system is unusable
NDLP_ALERT=1   # action must be taken immediately
NDLP_CRIT=2    # critical conditions
NDLP_ERR=3     # error conditions
NDLP_WARN=4    # warning conditions
NDLP_NOTICE=5  # normal but significant condition
NDLP_INFO=6    # informational
NDLP_DEBUG=7   # debug-level messages

# the max (numerically) log level we will log
LOG_LEVEL=$NDLP_INFO

set_log_min_priority() {
  case "${NETDATA_LOG_LEVEL,,}" in
    "emerg" | "emergency")
      LOG_LEVEL=$NDLP_EMERG
      ;;

    "alert")
      LOG_LEVEL=$NDLP_ALERT
      ;;

    "crit" | "critical")
      LOG_LEVEL=$NDLP_CRIT
      ;;

    "err" | "error")
      LOG_LEVEL=$NDLP_ERR
      ;;

    "warn" | "warning")
      LOG_LEVEL=$NDLP_WARN
      ;;

    "notice")
      LOG_LEVEL=$NDLP_NOTICE
      ;;

    "info")
      LOG_LEVEL=$NDLP_INFO
      ;;

    "debug")
      LOG_LEVEL=$NDLP_DEBUG
      ;;
  esac
}

set_log_min_priority

log() {
  local level="${1}"
  shift 1

  [[ -n "$level" && -n "$LOG_LEVEL" && "$level" -gt "$LOG_LEVEL" ]] && return

  systemd-cat-native --log-as-netdata <<EOFLOG
INVOCATION_ID=${NETDATA_INVOCATION_ID}
SYSLOG_IDENTIFIER=${PROGRAM_NAME}
PRIORITY=${level}
THREAD_TAG=ioping.plugin
ND_LOG_SOURCE=collector
MESSAGE=${MODULE_NAME}: ${*//$'\n'/\\n}

EOFLOG
  # AN EMPTY LINE IS NEEDED ABOVE
}

info() {
  log "$NDLP_INFO" "${@}"
}

warning() {
  log "$NDLP_WARN" "${@}"
}

error() {
  log "$NDLP_ERR" "${@}"
}

disable() {
  log "${@}"
	echo "DISABLE"
  exit 1
}

fatal() {
  disable "$NDLP_ALERT" "${@}"
}

debug() {
  log "$NDLP_DEBUG" "${@}"
}

# -----------------------------------------------------------------------------

# store in ${plugin} the name we run under
# this allows us to copy/link ioping.plugin under a different name
# to have multiple ioping plugins running with different settings
plugin="${PROGRAM_NAME/.plugin/}"


# -----------------------------------------------------------------------------

# the frequency to send info to netdata
# passed by netdata as the first parameter
update_every="${1-1}"

# the netdata configuration directory
# passed by netdata as an environment variable
[ -z "${NETDATA_USER_CONFIG_DIR}"  ] && NETDATA_USER_CONFIG_DIR="@configdir_POST@"
[ -z "${NETDATA_STOCK_CONFIG_DIR}" ] && NETDATA_STOCK_CONFIG_DIR="@libconfigdir_POST@"

# the netdata directory for internal binaries
[ -z "${NETDATA_PLUGINS_DIR}"      ] && NETDATA_PLUGINS_DIR="@pluginsdir_POST@"

# -----------------------------------------------------------------------------
# configuration options
# can be overwritten at /etc/netdata/ioping.conf

# the ioping binary to use
# we need one that can output netdata friendly info (supporting: -N)
# if you have multiple versions, put here the full filename of the right one
ioping="${NETDATA_PLUGINS_DIR}/ioping"

# the destination to ioping
destination=""

# the request size in bytes to ping the disk
request_size="4k"

# ioping options
ioping_opts="-T 1000000"

# -----------------------------------------------------------------------------
# load the configuration files

for CONFIG in "${NETDATA_STOCK_CONFIG_DIR}/${plugin}.conf" "${NETDATA_USER_CONFIG_DIR}/${plugin}.conf"; do
  if [ -f "${CONFIG}" ]; then
    debug "Loading config file '${CONFIG}'..."
    source "${CONFIG}"
    [ $? -ne 0 ] && warn "Failed to load config file '${CONFIG}'."
  elif [[ $CONFIG =~ ^$NETDATA_USER_CONFIG_DIR ]]; then
    debug "Cannot find file '${CONFIG}'."
  fi
done

if [ -z "${destination}" ]
then
	disable $NDLP_DEBUG "destination is not configured - nothing to do."
fi

if [ ! -f "${ioping}" ]
then
	disable $NDLP_ERR "ioping command is not found. Please set its full path in '${NETDATA_USER_CONFIG_DIR}/${plugin}.conf'"
fi

if [ ! -x "${ioping}" ]
then
	disable $NDLP_ERR "ioping command '${ioping}' is not executable - cannot proceed."
fi

# the ioping options we will use
options=( -N -i ${update_every} -s ${request_size} ${ioping_opts} ${destination} )

# execute ioping
debug "starting ioping: ${ioping} ${options[*]}"

exec "${ioping}" "${options[@]}"

# if we cannot execute ioping, stop
error "command '${ioping} ${options[*]}' failed to be executed (returned code $?)."
