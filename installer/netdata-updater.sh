#!/usr/bin/env bash

# this script will uninstall netdata

# Variables needed by script:
#  - PATH
#  - CFLAGS
#  - NETDATA_CONFIGURE_OPTIONS
#  - REINSTALL_PWD
#  - REINSTALL_COMMAND

force=0
[ "${1}" = "-f" ] && force=1

# make sure we cd to the working directory
REINSTALL_PWD="THIS_SHOULD_BE_REPLACED_BY_INSTALLER_SCRIPT"
cd "${REINSTALL_PWD}" || exit 1

#shellcheck source=/dev/null
source installer/.environment.sh || exit 1

if [ "${INSTALL_UID}" != "$(id -u)" ]
    then
    echo >&2 "You are running this script as user with uid $(id -u). We recommend to run this script as root (user with uid 0)"
    exit 1
fi

# make sure there is .git here
[ ${force} -eq 0 -a ! -d .git ] && echo >&2 "No git structures found at: ${REINSTALL_PWD} (use -f for force re-install)" && exit 1

# signal netdata to start saving its database
# this is handy if your database is big
pids=$(pidof netdata)
do_not_start=
if [ ! -z "${pids}" ]; then
	#shellcheck disable=SC2086
	kill -USR1 ${pids}
else
	# netdata is currently not running, so do not start it after updating
	do_not_start="--dont-start-it"
fi

tmp=
if [ -t 2 ]; then
	# we are running on a terminal
	# open fd 3 and send it to stderr
	exec 3>&2
else
	# we are headless
	# create a temporary file for the log
	tmp=$(mktemp /tmp/netdata-updater.log.XXXXXX)
	# open fd 3 and send it to tmp
	exec 3>"${tmp}"
fi

info() {
	echo >&3 "$(date) : INFO: " "${@}"
}

emptyline() {
	echo >&3
}

error() {
	echo >&3 "$(date) : ERROR: " "${@}"
}

# this is what we will do if it fails (head-less only)
failed() {
	error "FAILED TO UPDATE NETDATA : ${1}"

	if [ ! -z "${tmp}" ]; then
		cat >&2 "${tmp}"
		rm "${tmp}"
	fi
	exit 1
}

get_latest_commit_id() {
	git rev-parse HEAD 2>&3
}

update() {
	[ -z "${tmp}" ] && info "Running on a terminal - (this script also supports running headless from crontab)"

	emptyline

	if [ -d .git ]; then
		info "Updating netdata source from github..."

		last_commit="$(get_latest_commit_id)"
		if [ ${force} -eq 0 ] && [ -z "${last_commit}" ]; then
			failed "CANNOT GET LAST COMMIT ID (use -f for force re-install)"
		fi

		info "Stashing local git changes. You can use $(git stash pop) to reapply your changes."
		git stash 2>&3 >&3
		git fetch --all 2>&3 >&3
		git fetch --tags 2>&3 >&3
		git checkout master 2>&3 >&3
		git reset --hard origin/master 2>&3 >&3
		git pull 2>&3 >&3

		new_commit="$(get_latest_commit_id)"
		if [ ${force} -eq 0 ]; then
			[ -z "${new_commit}" ] && failed "CANNOT GET NEW LAST COMMIT ID (use -f for force re-install)"
			[ "${new_commit}" = "${last_commit}" ] && info "Nothing to be done! (use -f to force re-install)" && exit 0
		fi
	elif [ ${force} -eq 0 ]; then
		failed "CANNOT FIND GIT STRUCTURES IN $(pwd) (use -f for force re-install)"
	fi

	emptyline
	info "Re-installing netdata..."
	${REINSTALL_COMMAND} --dont-wait ${do_not_start} >&3 2>&3 || failed "FAILED TO COMPILE/INSTALL NETDATA"

	[ ! -z "${tmp}" ] && rm "${tmp}" && tmp=
	return 0
}

# the installer updates this script - so we run and exit in a single line
update && exit 0
