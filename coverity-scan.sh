#!/usr/bin/env bash
#
# Coverity scan script
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Costa Tsaousis (costa@netdata.cloud)
# Author  : Pawel Krupa (paulfantom)
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
# shellcheck disable=SC1091,SC2230,SC2086

# To run manually, save configuration to .coverity-scan.conf like this:
#
# the repository to report to coverity - devs can set here their own fork
# REPOSITORY="netdata/netdata"
#
# the email of the developer, as given to coverity
# COVERITY_SCAN_SUBMIT_MAIL="you@example.com"
#
# the token given by coverity to the developer
# COVERITY_SCAN_TOKEN="TOKEN taken from Coverity site"
#
# the absolute path of the cov-build - optional
# COVERITY_BUILD_PATH="/opt/cov-analysis-linux64-2019.03/bin/cov-build"
#
# when set, the script will print on screen the curl command that submits the build to coverity
# this includes the token, so the default is not to print it.
# COVERITY_SUBMIT_DEBUG=1
#
# Override the standard coverity build version we know is supported
# COVERITY_BUILD_VERSION="cov-analysis-linux64-2019.03"
#
# All these variables can also be exported before running this script.
#
# If the first parameter of this script is "install",
# coverity build tools will be downloaded and installed in /opt/coverity

set -e

INSTALL_DIR="/opt"

# the version of coverity to use
COVERITY_BUILD_VERSION="${COVERITY_BUILD_VERSION:-cov-analysis-linux64-2019.03}"

# TODO: For some reasons this does not fully load on Debian 10 (Haven't checked if it happens on other distros yet), it breaks
source packaging/installer/functions.sh || echo "Failed to fully load the functions library"

cpus=$(find_processors)
[ -z "${cpus}" ] && cpus=1

if [ -f ".coverity-scan.conf" ]; then
  source ".coverity-scan.conf"
fi

repo="${REPOSITORY}"
if [ -z "${repo}" ]; then
  fatal "export variable REPOSITORY or set it in .coverity-scan.conf"
fi
repo="${repo//\//%2F}"

email="${COVERITY_SCAN_SUBMIT_MAIL}"
if [ -z "${email}" ]; then
  fatal "export variable COVERITY_SCAN_SUBMIT_MAIL or set it in .coverity-scan.conf"
fi

token="${COVERITY_SCAN_TOKEN}"
if [ -z "${token}" ]; then
  fatal "export variable COVERITY_SCAN_TOKEN or set it in .coverity-scan.conf"
fi

if ! command -v curl > /dev/null 2>&1; then
  fatal "CURL is required for coverity scan to work"
fi

# only print the output of a command
# when debugging is enabled
# used to hide the token when debugging is not enabled
debugrun() {
  if [ "${COVERITY_SUBMIT_DEBUG}" = "1" ]; then
    run "${@}"
    return $?
  else
    "${@}"
    return $?
  fi
}

scanit() {
  progress "Scanning using coverity"
  export PATH="${PATH}:${INSTALL_DIR}/${COVERITY_BUILD_VERSION}/bin/"
  covbuild="${COVERITY_BUILD_PATH}"
  [ -z "${covbuild}" ] && covbuild="$(which cov-build 2> /dev/null || command -v cov-build 2> /dev/null)"

  if [ -z "${covbuild}" ]; then
    fatal "Cannot find 'cov-build' binary in \$PATH. Export variable COVERITY_BUILD_PATH or set it in .coverity-scan.conf"
  elif [ ! -x "${covbuild}" ]; then
    fatal "The command '${covbuild}' is not executable. Export variable COVERITY_BUILD_PATH or set it in .coverity-scan.conf"
  fi

  version="$(grep "^#define PACKAGE_VERSION" config.h | cut -d '"' -f 2)"
  progress "Working on netdata version: ${version}"

  progress "Cleaning up old builds..."
  run make clean || echo >&2 "Nothing to clean"

  [ -d "cov-int" ] && rm -rf "cov-int"

  [ -f netdata-coverity-analysis.tgz ] && run rm netdata-coverity-analysis.tgz

  progress "Configuring netdata source..."

  run autoreconf -ivf
  run ./configure ${OTHER_OPTIONS}

  progress "Analyzing netdata..."
  run "${covbuild}" --dir cov-int make -j${cpus}

  echo >&2 "Compressing analysis..."
  run tar czvf netdata-coverity-analysis.tgz cov-int

  echo >&2 "Sending analysis to coverity for netdata version ${version} ..."
  COVERITY_SUBMIT_RESULT=$(debugrun curl --progress-bar \
    --form token="${token}" \
    --form email="${email}" \
    --form file=@netdata-coverity-analysis.tgz \
    --form version="${version}" \
    --form description="netdata, monitor everything, in real-time." \
    https://scan.coverity.com/builds?project="${repo}")

  echo "${COVERITY_SUBMIT_RESULT}" | grep -q -e 'Build successfully submitted' || echo >&2 "scan results were not pushed to coverity. Message was: ${COVERITY_SUBMIT_RESULT}"

  progress "Coverity scan completed"
}

installit() {
  ORIGINAL_DIR="${PWD}"
  TMP_DIR="$(mktemp -d /tmp/netdata-coverity-scan-XXXXX)"
  progress "Downloading coverity in ${TMP_DIR}..."
  cd "${TMP_DIR}"

  debugrun curl --remote-name --remote-header-name --show-error --location --data "token=${token}&project=${repo}" https://scan.coverity.com/download/linux64

  if [ -f "${COVERITY_BUILD_VERSION}.tar.gz" ]; then
    progress "Installing coverity..."
    cd "${INSTALL_DIR}"

    run sudo tar -z -x -f "${TMP_DIR}/${COVERITY_BUILD_VERSION}.tar.gz" || exit 1
    rm "${TMP_DIR}/${COVERITY_BUILD_VERSION}.tar.gz"
    export PATH=${PATH}:${INSTALL_DIR}/${COVERITY_BUILD_VERSION}/bin/
  else
    fatal "Failed to download coverity tool tarball!"
  fi

  # Validate the installation
  covbuild="$(which cov-build 2> /dev/null || command -v cov-build 2> /dev/null)"
  if [ -z "$covbuild" ]; then
    fatal "Failed to install coverity."
  fi

  # Clean temp directory
  [ -n "${TMP_DIR}" ] && rm -rf "${TMP_DIR}"

  progress "Coverity scan tools are installed."
  cd "$ORIGINAL_DIR"
  return 0
}

OTHER_OPTIONS="--disable-lto"
OTHER_OPTIONS+=" --with-zlib"
OTHER_OPTIONS+=" --with-math"
OTHER_OPTIONS+=" --enable-https"
OTHER_OPTIONS+=" --enable-jsonc"
OTHER_OPTIONS+=" --enable-plugin-nfacct"
OTHER_OPTIONS+=" --enable-plugin-freeipmi"
OTHER_OPTIONS+=" --enable-plugin-cups"
OTHER_OPTIONS+=" --enable-backend-prometheus-remote-write"
# TODO: enable these plugins too
#OTHER_OPTIONS+=" --enable-plugin-xenstat"
#OTHER_OPTIONS+=" --enable-backend-kinesis"
#OTHER_OPTIONS+=" --enable-backend-mongodb"

FOUND_OPTS="NO"
while [ -n "${1}" ]; do
  if [ "${1}" = "--with-install" ]; then
    progress "Running coverity install"
    installit
    shift 1
  elif [ -n "${1}" ]; then
    # Clear the default arguments, once you bump into the first argument
    if [ "${FOUND_OPTS}" = "NO" ]; then
      OTHER_OPTIONS="${1}"
      FOUND_OPTS="YES"
    else
      OTHER_OPTIONS+=" ${1}"
    fi

    shift 1
  else
    break
  fi
done

echo "Running coverity scan with extra options ${OTHER_OPTIONS}"
scanit "${OTHER_OPTIONS}"
