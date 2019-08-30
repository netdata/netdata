#!/usr/bin/env bash
# Coverity scan script
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Costa Tsaousis (costa@netdata.cloud)
# Author  : Pawel Krupa (paulfantom)
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)

# To run manually, save configuration to .coverity-scan.conf like this:
#
# REPOSITORY="netdata/netdata"
# COVERITY_SCAN_SUBMIT_MAIL="you@example.com"
# COVERITY_SCAN_TOKEN="TOKEN taken from Coverity site"
# COVERITY_BUILD_PATH="/opt/cov-analysis-linux64-2019.03/bin/cov-build"
# DEBUG=1
#
# All these variables can also be exported before running this script.

source packaging/installer/functions.sh || exit 1

cpus=$(find_processors)
[ -z "${cpus}" ] && cpus=1

if [ -f ".coverity-scan.conf" ]
then
	source ".coverity-scan.conf" || exit 1
fi

repo="${REPOSITORY}"
if [ -z "${repo}" ]; then
	echo >&2 "export variable REPOSITORY or set it in .coverity-scan.conf"
	exit 1
fi
repo="${repo//\//%2F}"

email="${COVERITY_SCAN_SUBMIT_MAIL}"
if [ -z "${email}" ]; then
	echo >&2 "export variable COVERITY_SCAN_SUBMIT_MAIL or set it in .coverity-scan.conf"
	exit 1
fi

token="${COVERITY_SCAN_TOKEN}"
if [ -z "${token}" ]; then
	echo >&2 "export variable COVERITY_SCAN_TOKEN or set it in .coverity-scan.conf"
	exit 1
fi

export PATH=${PATH}:/opt/coverity/bin/
covbuild="${COVERITY_BUILD_PATH}"
[ -z "${covbuild}" ] && covbuild="$(which cov-build 2>/dev/null || command -v cov-build 2>/dev/null)"
if [ -z "${covbuild}" ]; then
	echo >&2 "Cannot find 'cov-build' binary in \$PATH. Export variable COVERITY_BUILD_PATH or set it in .coverity-scan.conf"
	exit 1
elif [ ! -x "${covbuild}" ]; then
	echo >&2 "The command ${covbuild} is not executable. Export variable COVERITY_BUILD_PATH or set it in .coverity-scan.conf"
	exit 1
fi

version="$(grep "^#define PACKAGE_VERSION" config.h | cut -d '"' -f 2)"
echo >&2 "Working on netdata version: ${version}"

echo >&2 "Cleaning up old builds..."
run make clean || echo >&2 "Nothing to clean"

[ -d "cov-int" ] && rm -rf "cov-int"

[ -f netdata-coverity-analysis.tgz ] && run rm netdata-coverity-analysis.tgz

run autoreconf -ivf
run ./configure --disable-lto \
	--enable-https \
	--enable-jsonc \
	--enable-plugin-nfacct \
	--enable-plugin-freeipmi \
	--enable-plugin-cups \
	--enable-backend-prometheus-remote-write \
	${NULL}

# TODO: enable these plugins too
#	--enable-plugin-xenstat \
#	--enable-backend-kinesis \
#	--enable-backend-mongodb \

run "${covbuild}" --dir cov-int make -j${cpus} || exit 1

echo >&2 "Compressing data..."
run tar czvf netdata-coverity-analysis.tgz cov-int || exit 1

debugrun() {
	# hide the token when DEBUG is not 1

	if [ "${DEBUG}" = "1" ]
	then
		run "${@}"
		return $?
	else
		"${@}"
		return $?
	fi
}

echo >&2 "Sending analysis for version ${version} ..."
COVERITY_SUBMIT_RESULT=$(debugrun curl --progress-bar \
  --form token="${token}" \
  --form email=${email} \
  --form file=@netdata-coverity-analysis.tgz \
  --form version="${version}" \
  --form description="netdata, monitor everything, in real-time." \
  https://scan.coverity.com/builds?project=${repo})

echo ${COVERITY_SUBMIT_RESULT} | grep -q -e 'Build successfully submitted' || echo >&2 "scan results were not pushed to coverity. Message was: ${COVERITY_SUBMIT_RESULT}"

echo >&2 "Coverity scan mechanism completed"
