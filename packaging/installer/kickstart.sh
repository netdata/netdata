#!/usr/bin/env sh
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Run me with:
#
# bash <(curl -Ss https://my-netdata.io/kickstart.sh)
#
# or (to install all netdata dependencies):
#
# bash <(curl -Ss https://my-netdata.io/kickstart.sh) all
#
# Other options:
#  --dont-wait       do not prompt for user input
#  --non-interactive do not prompt for user input
#  --no-updates      do not install script for daily updates
#
# This script will:
#
# 1. install all netdata compilation dependencies
#    using the package manager of the system
#
# 2. download netdata nightly package to temporary directory
#
# 3. install netdata

# shellcheck disable=SC2039,SC2059,SC2086

# External files
PACKAGES_SCRIPT="https://raw.githubusercontent.com/netdata/netdata-demo-site/master/install-required-packages.sh"

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

fatal() {
	printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ABORTED ${TPUT_RESET} ${*} \n\n"
	exit 1
}

warning() {
	printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} WARNING ${TPUT_RESET} ${*} \n\n"
	if [ "${INTERACTIVE}" = "0" ]; then
		fatal "Stopping due to non-interactive mode. Fix the issue or retry installation in an interactive mode."
	else
		read -r -p "Press ENTER to attempt netdata installation > "
		progress "OK, let's give it a try..."
	fi
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
	if [ "$1" == "stable" ]; then
		local latest
		# Simple version
		# latest="$(curl -sSL https://api.github.com/repos/netdata/netdata/releases/latest | grep tag_name | cut -d'"' -f4)"
		latest="$(download "https://api.github.com/repos/netdata/netdata/releases/latest" /dev/stdout | grep tag_name | cut -d'"' -f4)"
		export NETDATA_TARBALL_URL="https://github.com/netdata/netdata/releases/download/$latest/netdata-$latest.tar.gz"
		export NETDATA_TARBALL_CHECKSUM_URL="https://github.com/netdata/netdata/releases/download/$latest/sha256sums.txt"
	else
		export NETDATA_TARBALL_URL="https://storage.googleapis.com/netdata-nightlies/netdata-latest.tar.gz"
		export NETDATA_TARBALL_CHECKSUM_URL="https://storage.googleapis.com/netdata-nightlies/sha256sums.txt"
	fi
}

detect_bash4() {
	bash="${1}"
	if [ -z "${BASH_VERSION}" ]; then
		# we don't run under bash
		if [ -n "${bash}" ] && [ -x "${bash}" ]; then
			# shellcheck disable=SC2016
			BASH_MAJOR_VERSION=$(${bash} -c 'echo "${BASH_VERSINFO[0]}"')
		fi
	else
		# we run under bash
		BASH_MAJOR_VERSION="${BASH_VERSINFO[0]}"
	fi

	if [ -z "${BASH_MAJOR_VERSION}" ]; then
		echo >&2 "No BASH is available on this system"
		return 1
	elif [ $((BASH_MAJOR_VERSION)) -lt 4 ]; then
		echo >&2 "No BASH v4+ is available on this system (installed bash is v${BASH_MAJOR_VERSION}"
		return 1
	fi
	return 0
}

dependencies() {
	SYSTEM="$(uname -s)"
	OS="$(uname -o)"
	MACHINE="$(uname -m)"

	echo "System            : ${SYSTEM}"
	echo "Operating System  : ${OS}"
	echo "Machine           : ${MACHINE}"
	echo "BASH major version: ${BASH_MAJOR_VERSION}"

	if [ "${OS}" != "GNU/Linux" ] && [ "${SYSTEM}" != "Linux" ]; then
		warning "Cannot detect the packages to be installed on a ${SYSTEM} - ${OS} system."
	else
		bash="$(command -v bash 2>/dev/null)"
		if ! detect_bash4 "${bash}"; then
			warning "Cannot detect packages to be installed in this system, without BASH v4+."
		else
			progress "Downloading script to detect required packages..."
			download "${PACKAGES_SCRIPT}" "${TMPDIR}/install-required-packages.sh"
			if [ ! -s "${TMPDIR}/install-required-packages.sh" ]; then
				warning "Downloaded dependency installation script is empty."
			else
				progress "Running downloaded script to detect required packages..."
				run ${sudo} "${bash}" "${TMPDIR}/install-required-packages.sh" ${PACKAGES_INSTALLER_OPTIONS}
				# shellcheck disable=SC2181
				if [ $? -ne 0 ] ; then
					warning "It failed to install all the required packages, but installation might still be possible."
				fi
			fi

		fi
	fi
}

umask 022

sudo=""
[ -z "${UID}" ] && UID="$(id -u)"
[ "${UID}" -ne "0" ] && sudo="sudo"
export PATH="${PATH}:/usr/local/bin:/usr/local/sbin"

setup_terminal || echo >/dev/null

# ---------------------------------------------------------------------------------------------------------------------
# try to update using autoupdater in the first place

updater=""
[ -x /etc/periodic/daily/netdata-updater ] && updater=/etc/periodic/daily/netdata-updater
[ -x /etc/cron.daily/netdata-updater ] && updater=/etc/cron.daily/netdata-updater
if [ -L "${updater}" ]; then
	# remove old updater (symlink)
	run "${sudo}" rm -f "${updater}"
	updater=""
fi
if [ -n "${updater}" ]; then
	# attempt to run the updater, to respect any compilation settings already in place
	progress "Re-installing netdata..."
	run "${sudo}" "${updater}" -f || fatal "Failed to forcefully update netdata"
	exit 0
fi

# ---------------------------------------------------------------------------------------------------------------------
# install required system packages

INTERACTIVE=1
PACKAGES_INSTALLER_OPTIONS="netdata"
NETDATA_INSTALLER_OPTIONS=""
NETDATA_UPDATES="--auto-update"
RELEASE_CHANNEL="nightly"
while [ -n "${1}" ]; do
	if [ "${1}" = "all" ]; then
		PACKAGES_INSTALLER_OPTIONS="netdata-all"
		shift 1
	elif [ "${1}" = "--dont-wait" ] || [ "${1}" = "--non-interactive" ]; then
		INTERACTIVE=0
		shift 1
	elif [ "${1}" = "--no-updates" ]; then
		# echo >&2 "netdata will not auto-update"
		NETDATA_UPDATES=
		shift 1
	elif [ "${1}" = "--stable-channel" ]; then
		RELEASE_CHANNEL="stable"
		shift 1
	else
		break
	fi
done

if [ "${INTERACTIVE}" = "0" ]; then
	PACKAGES_INSTALLER_OPTIONS="--dont-wait --non-interactive ${PACKAGES_INSTALLER_OPTIONS}"
	NETDATA_INSTALLER_OPTIONS="--dont-wait"
fi

TMPDIR=$(create_tmp_directory)
cd ${TMPDIR} || :

dependencies

# ---------------------------------------------------------------------------------------------------------------------
# download netdata package

set_tarball_urls "${RELEASE_CHANNEL}"

download "${NETDATA_TARBALL_CHECKSUM_URL}" "${TMPDIR}/sha256sum.txt"
download "${NETDATA_TARBALL_URL}" "${TMPDIR}/netdata-latest.tar.gz"
if ! grep netdata-latest.tar.gz "${TMPDIR}/sha256sum.txt" | sha256sum --check - >/dev/null 2>&1; then
	fatal "Tarball checksum validation failed. Stopping netdata installation and leaving tarball in ${TMPDIR}"
fi
run tar -xf netdata-latest.tar.gz
rm -rf netdata-latest.tar.gz >/dev/null 2>&1
cd netdata-* || fatal "Cannot cd to netdata source tree"

# ---------------------------------------------------------------------------------------------------------------------
# install netdata from source

if [ -x netdata-installer.sh ]; then
	progress "Installing netdata..."
	run ${sudo} ./netdata-installer.sh ${NETDATA_UPDATES} ${NETDATA_INSTALLER_OPTIONS} "${@}" || fatal "netdata-installer.sh exited with error"
	rm -rf "${TMPDIR}" >/dev/null 2>&1
else
	fatal "Cannot install netdata from source (the source directory does not include netdata-installer.sh). Leaving all files in ${TMPDIR}"
fi
