#!/usr/bin/env bash

# SPDX-License-Identifier: GPL-3.0-or-later

# This is a simple backend database proxy, written in BASH, using the nc command.
# Run the script without any parameters for help.

MODE="${1}"
MY_PORT="${2}"
BACKEND_HOST="${3}"
BACKEND_PORT="${4}"
FILE="${NETDATA_NC_BACKEND_DIR-/tmp}/netdata-nc-backend-${MY_PORT}"

log() {
	logger --stderr --id=$$ --tag "netdata-nc-backend" "${*}"
}

mync() {
	local ret

	log "Running: nc ${*}"
	nc "${@}"
	ret=$?

	log "nc stopped with return code ${ret}."

	return ${ret}
}

listen_save_replay_forever() {
	local file="${1}" port="${2}" real_backend_host="${3}" real_backend_port="${4}" ret delay=1 started ended

	while true
	do
		log "Starting nc to listen on port ${port} and save metrics to ${file}"
		
		started=$(date +%s)
		mync -l -p "${port}" | tee -a -p --output-error=exit "${file}"
		ended=$(date +%s)
		
		if [ -s "${file}" ]
			then
			if [ ! -z "${real_backend_host}" ] && [ ! -z "${real_backend_port}" ]
				then
				log "Attempting to send the metrics to the real backend at ${real_backend_host}:${real_backend_port}"
				
				mync "${real_backend_host}" "${real_backend_port}" <"${file}"
				ret=$?

				if [ ${ret} -eq 0 ]
					then
					log "Successfuly sent the metrics to ${real_backend_host}:${real_backend_port}"
					mv "${file}" "${file}.old"
					touch "${file}"
				else
					log "Failed to send the metrics to ${real_backend_host}:${real_backend_port} (nc returned ${ret}) - appending more data to ${file}"
				fi
			else
				log "No backend configured - appending more data to ${file}"
			fi
		fi

		# prevent a CPU hungry infinite loop
		# if nc cannot listen to port
		if [ $((ended - started)) -lt 5 ]
			then
			log "nc has been stopped too fast."
			delay=30
		else
			delay=1
		fi

		log "Waiting ${delay} seconds before listening again for data."
		sleep ${delay}
	done
}

if [ "${MODE}" = "start" ]
	then

	# start the listener, in exclusive mode
	# only one can use the same file/port at a time
	{
		flock -n 9
		# shellcheck disable=SC2181
		if [ $? -ne 0 ]
			then
			log "Cannot get exclusive lock on file ${FILE}.lock - Am I running multiple times?"
			exit 2
		fi

		# save our PID to the lock file
		echo "$$" >"${FILE}.lock"

		listen_save_replay_forever "${FILE}" "${MY_PORT}" "${BACKEND_HOST}" "${BACKEND_PORT}"
		ret=$?

		log "listener exited."
		exit ${ret}

	} 9>>"${FILE}.lock"

	# we can only get here if ${FILE}.lock cannot be created
	log "Cannot create file ${FILE}."
	exit 3

elif [ "${MODE}" = "stop" ]
	then

	{
		flock -n 9
		# shellcheck disable=SC2181
		if [ $? -ne 0 ]
			then
			pid=$(<"${FILE}".lock)
			log "Killing process ${pid}..."
			kill -TERM "-${pid}"
			exit 0
		fi

		log "File ${FILE}.lock has been locked by me but it shouldn't. Is a collector running?"
		exit 4

	} 9<"${FILE}.lock"

	log "File ${FILE}.lock does not exist. Is a collector running?"
	exit 5

else

	cat <<EOF
Usage:

    "${0}" start|stop PORT [BACKEND_HOST BACKEND_PORT]

    PORT          The port this script will listen
                  (configure netdata to use this as a second backend)

    BACKEND_HOST  The real backend host
    BACKEND_PORT  The real backend port

    This script can act as fallback backend for netdata.
    It will receive metrics from netdata, save them to
    ${FILE}
    and once netdata reconnects to the real-backend, this script
    will push all metrics collected to the real-backend too and
    wait for a failure to happen again.

    Only one netdata can connect to this script at a time.
    If you need fallback for multiple netdata, run this script
    multiple times with different ports.

    You can run me in the background with this:

    screen -d -m "${0}" start PORT [BACKEND_HOST BACKEND_PORT]
EOF
	exit 1
fi
