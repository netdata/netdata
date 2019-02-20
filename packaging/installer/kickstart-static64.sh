#!/usr/bin/env sh
# SPDX-License-Identifier: GPL-3.0-or-later
# shellcheck disable=SC1117,SC2039,SC2059,SC2086

# External files
NIGHTLY_PACKAGE_TARBALL="https://storage.googleapis.com/netdata-nightlies/netdata-latest.gz.run"
NIGHTLY_PACKAGE_CHECKSUM="https://storage.googleapis.com/netdata-nightlies/sha256sums.txt"

# ---------------------------------------------------------------------------------------------------------------------
# library functions copied from packaging/installer/functions.sh

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

	if command -v tput >/dev/null 2>&1; then
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

progress() {
	echo >&2 " --- ${TPUT_DIM}${TPUT_BOLD}${*}${TPUT_RESET} --- "
}

escaped_print() {
	if printf "%q " test >/dev/null 2>&1; then
		printf "%q " "${@}"
	else
		printf "%s" "${*}"
	fi
	return 0
}

run() {
	local dir="${PWD}" info_console

	if [ "${UID}" = "0" ]; then
		info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]# "
	else
		info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]$ "
	fi

	escaped_print "${info_console}${TPUT_BOLD}${TPUT_YELLOW}" "${@}" "${TPUT_RESET}\n" >&2

	"${@}"

	local ret=$?
	if [ ${ret} -ne 0 ]; then
		printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} FAILED ${TPUT_RESET} ${*} \n\n"
	else
		printf >&2 "${TPUT_BGGREEN}${TPUT_WHITE}${TPUT_BOLD} OK ${TPUT_RESET} ${*} \n\n"
	fi

	return ${ret}
}

# ---------------------------------------------------------------------------------------------------------------------

fatal() {
	printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ABORTED ${TPUT_RESET} ${*} \n\n"
	exit 1
}

download() {
	url="${1}"
	dest="${2}"
	if command -v wget >/dev/null 2>&1; then
		run wget -O - "${url}" >"${dest}" || fatal "Cannot download ${url}"
	elif command -v curl >/dev/null 2>&1; then
		run curl -L "${url}" >"${dest}" || fatal "Cannot download ${url}"
	else
		fatal "I need curl or wget to proceed, but neither is available on this system."
	fi
}

umask 022

sudo=""
[ -z "${UID}" ] && UID="$(id -u)"
[ "${UID}" -ne "0" ] && sudo="sudo"

setup_terminal || echo >/dev/null

# ---------------------------------------------------------------------------------------------------------------------

if [ "$(uname -m)" != "x86_64" ]; then
	fatal "Static binary versions of netdata are available only for 64bit Intel/AMD CPUs (x86_64), but yours is: $(uname -m)."
fi

if [ "$(uname -s)" != "Linux" ]; then
	fatal "Static binary versions of netdata are available only for Linux, but this system is $(uname -s)"
fi

# ---------------------------------------------------------------------------------------------------------------------

# Check if tmp is mounted as noexec
if grep -Eq '^[^ ]+ /tmp [^ ]+ ([^ ]*,)?noexec[, ]' /proc/mounts; then
	pattern="$(pwd)/netdata-kickstart-static-XXXXXX"
else
	pattern="/tmp/netdata-kickstart-static-XXXXXX"
fi

tmpdir="$(mktemp -d $pattern)"
cd "${tmpdir}" || :

progress "Downloading static netdata binary: ${NIGHTLY_PACKAGE_TARBALL}"

download "${NIGHTLY_PACKAGE_CHECKSUM}" "${tmpdir}/sha256sum.txt"
download "${NIGHTLY_PACKAGE_TARBALL}" "${tmpdir}/netdata-latest.gz.run"
if ! grep netdata-latest.gz.run sha256sum.txt | sha256sum --check - >/dev/null 2>&1; then
	failed "Static binary checksum validation failed. Stopping netdata installation and leaving binary in ${tmpdir}"
fi

# ---------------------------------------------------------------------------------------------------------------------

opts=
inner_opts=
while [ ! -z "${1}" ]; do
	if [ "${1}" = "--dont-wait" ] || [ "${1}" = "--non-interactive" ] || [ "${1}" = "--accept" ]; then
		opts="${opts} --accept"
	elif [ "${1}" = "--dont-start-it" ]; then
		inner_opts="${inner_opts} ${1}"
	else
		echo >&2 "Unknown option '${1}'"
		exit 1
	fi
	shift
done
[ ! -z "${inner_opts}" ] && inner_opts="-- ${inner_opts}"

# ---------------------------------------------------------------------------------------------------------------------

progress "Installing netdata"

run ${sudo} sh "${tmpdir}/netdata-latest.gz.run" ${opts} ${inner_opts}

#shellcheck disable=SC2181
if [ $? -eq 0 ]; then
	rm "${tmpdir}/netdata-latest.gz.run"
else
	echo >&2 "NOTE: did not remove: ${tmpdir}/netdata-latest.gz.run"
fi
