#!/usr/bin/env sh
#
# SPDX-License-Identifier: GPL-3.0-or-later
# shellcheck disable=SC1117,SC2039,SC2059,SC2086
#
#  Options to run
#  --dont-wait              do not wait for input
#  --non-interactive        do not wait for input
#  --dont-start-it          do not start netdata after install
#  --stable-channel         Use the stable release channel, rather than the nightly to fetch sources
#  --disable-telemetry      Opt-out of anonymous telemetry program
#  --local-files            Use a manually provided tarball for the installation
#
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
setup_terminal || echo >/dev/null

# -----------------------------------------------------------------------------
fatal() {
	printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ABORTED ${TPUT_RESET} ${*} \n\n"
	exit 1
}

run_ok() {
	printf >&2 "${TPUT_BGGREEN}${TPUT_WHITE}${TPUT_BOLD} OK ${TPUT_RESET} ${*} \n\n"
}

run_failed() {
	printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} FAILED ${TPUT_RESET} ${*} \n\n"
}

ESCAPED_PRINT_METHOD=
printf "%q " test >/dev/null 2>&1
[ $? -eq 0 ] && ESCAPED_PRINT_METHOD="printfq"
escaped_print() {
	if [ "${ESCAPED_PRINT_METHOD}" = "printfq" ]; then
		printf "%q " "${@}"
	else
		printf "%s" "${*}"
	fi
	return 0
}

progress() {
	echo >&2 " --- ${TPUT_DIM}${TPUT_BOLD}${*}${TPUT_RESET} --- "
}

run_logfile="/dev/null"
run() {
	local user="${USER--}" dir="${PWD}" info info_console

	if [ "${UID}" = "0" ]; then
		info="[root ${dir}]# "
		info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]# "
	else
		info="[${user} ${dir}]$ "
		info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]$ "
	fi

	printf >>"${run_logfile}" "${info}"
	escaped_print >>"${run_logfile}" "${@}"
	printf >>"${run_logfile}" " ... "

	printf >&2 "${info_console}${TPUT_BOLD}${TPUT_YELLOW}"
	escaped_print >&2 "${@}"
	printf >&2 "${TPUT_RESET}\n"

	"${@}"

	local ret=$?
	if [ ${ret} -ne 0 ]; then
		run_failed
		printf >>"${run_logfile}" "FAILED with exit code ${ret}\n"
	else
		run_ok
		printf >>"${run_logfile}" "OK\n"
	fi

	return ${ret}
}

fatal() {
	printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ABORTED ${TPUT_RESET} ${*} \n\n"
	exit 1
}

create_tmp_directory() {
	# Check if tmp is mounted as noexec
	if grep -Eq '^[^ ]+ /tmp [^ ]+ ([^ ]*,)?noexec[, ]' /proc/mounts; then
		pattern="$(pwd)/netdata-kickstart-XXXXXX"
	else
		pattern="/tmp/netdata-kickstart-XXXXXX"
	fi

	mktemp -d $pattern
}

download() {
	url="${1}"
	dest="${2}"
	if command -v curl >/dev/null 2>&1; then
		run curl -sSL --connect-timeout 10 --retry 3 "${url}" >"${dest}" || fatal "Cannot download ${url}"
	elif command -v wget >/dev/null 2>&1; then
		run wget -T 15 -O - "${url}" >"${dest}" || fatal "Cannot download ${url}"
	else
		fatal "I need curl or wget to proceed, but neither is available on this system."
	fi
}

set_tarball_urls() {
	if [ -n "${NETDATA_LOCAL_TARBALL_OVERRIDE}" ]; then
		progress "Not fetching remote tarballs, local override was given"
		return
	fi

	if [ "$1" = "stable" ]; then
		local latest
		# Simple version
		# latest="$(curl -sSL https://api.github.com/repos/netdata/netdata/releases/latest | grep tag_name | cut -d'"' -f4)"
		latest="$(download "https://api.github.com/repos/netdata/netdata/releases/latest" /dev/stdout | grep tag_name | cut -d'"' -f4)"
		export NETDATA_TARBALL_URL="https://github.com/netdata/netdata/releases/download/$latest/netdata-$latest.gz.run"
		export NETDATA_TARBALL_CHECKSUM_URL="https://github.com/netdata/netdata/releases/download/$latest/sha256sums.txt"
	else
		export NETDATA_TARBALL_URL="https://storage.googleapis.com/netdata-nightlies/netdata-latest.gz.run"
		export NETDATA_TARBALL_CHECKSUM_URL="https://storage.googleapis.com/netdata-nightlies/sha256sums.txt"
	fi
}

safe_sha256sum() {
	# Within the contexct of the installer, we only use -c option that is common between the two commands
	# We will have to reconsider if we start non-common options
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum $@
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 $@
	else
		fatal "I could not find a suitable checksum binary to use"
	fi
}

# ---------------------------------------------------------------------------------------------------------------------
umask 022

sudo=""
[ -z "${UID}" ] && UID="$(id -u)"
[ "${UID}" -ne "0" ] && sudo="sudo"

# ---------------------------------------------------------------------------------------------------------------------
if [ "$(uname -m)" != "x86_64" ]; then
	fatal "Static binary versions of netdata are available only for 64bit Intel/AMD CPUs (x86_64), but yours is: $(uname -m)."
fi

if [ "$(uname -s)" != "Linux" ]; then
	fatal "Static binary versions of netdata are available only for Linux, but this system is $(uname -s)"
fi

# ---------------------------------------------------------------------------------------------------------------------
opts=
NETDATA_INSTALLER_OPTIONS=""
NETDATA_UPDATES="--auto-update"
RELEASE_CHANNEL="nightly"
while [ -n "${1}" ]; do
	if [ "${1}" = "--dont-wait" ] || [ "${1}" = "--non-interactive" ] || [ "${1}" = "--accept" ]; then
		opts="${opts} --accept"
		shift 1
	elif [ "${1}" = "--dont-start-it" ]; then
		NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS:+${NETDATA_INSTALLER_OPTIONS} }${1}"
		shift 1
	elif [ "${1}" = "--no-updates" ]; then
		NETDATA_UPDATES=""
		shift 1
	elif [ "${1}" = "--stable-channel" ]; then
		RELEASE_CHANNEL="stable"
		NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS:+${NETDATA_INSTALLER_OPTIONS} }${1}"
		shift 1
	elif [ "${1}" = "--disable-telemetry" ]; then
		NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS:+${NETDATA_INSTALLER_OPTIONS} }${1}"
		shift 1
	elif [ "${1}" = "--local-files" ]; then
		shift 1
		if [ -z "${1}" ]; then
			fatal "Option --local-files requires extra information. The desired tarball full filename is needed"
		fi

		NETDATA_LOCAL_TARBALL_OVERRIDE="${1}"
		shift 1
		if [ -z "${1}" ]; then
			fatal "Option --local-files requires a pair of the tarball source and the checksum file"
		fi

		NETDATA_LOCAL_TARBALL_OVERRIDE_CHECKSUM="${1}"
		shift 1
	else
		echo >&2 "Unknown option '${1}' or invalid number of arguments. Please check the README for the available arguments of ${0} and try again"
		exit 1
	fi
done

# ---------------------------------------------------------------------------------------------------------------------
TMPDIR=$(create_tmp_directory)
cd "${TMPDIR}"

if [ -z "${NETDATA_LOCAL_TARBALL_OVERRIDE}" ]; then
	set_tarball_urls "${RELEASE_CHANNEL}"
	progress "Downloading static netdata binary: ${NETDATA_TARBALL_URL}"

	download "${NETDATA_TARBALL_CHECKSUM_URL}" "${TMPDIR}/sha256sum.txt"
	download "${NETDATA_TARBALL_URL}" "${TMPDIR}/netdata-latest.gz.run"
else
	progress "Installation sources were given as input, running installation using \"${NETDATA_LOCAL_TARBALL_OVERRIDE}\""
	run cp "${NETDATA_LOCAL_TARBALL_OVERRIDE}" "${TMPDIR}/netdata-latest.gz.run"
	run cp "${NETDATA_LOCAL_TARBALL_OVERRIDE_CHECKSUM}" "${TMPDIR}/sha256sum.txt"
fi

if ! grep netdata-latest.gz.run "${TMPDIR}/sha256sum.txt" | safe_sha256sum -c - >/dev/null 2>&1; then
	fatal "Static binary checksum validation failed. Stopping netdata installation and leaving binary in ${TMPDIR}"
fi

# ---------------------------------------------------------------------------------------------------------------------
progress "Installing netdata"
run ${sudo} sh "${TMPDIR}/netdata-latest.gz.run" ${opts} -- ${NETDATA_UPDATES} ${NETDATA_INSTALLER_OPTIONS}

#shellcheck disable=SC2181
if [ $? -eq 0 ]; then
	run ${sudo} rm "${TMPDIR}/netdata-latest.gz.run"
	if [ ! "${TMPDIR}" = "/" ] && [ -d "${TMPDIR}" ]; then
		run ${sudo} rm -rf "${TMPDIR}"
	fi
else
	echo >&2 "NOTE: did not remove: ${TMPDIR}/netdata-latest.gz.run"
fi
