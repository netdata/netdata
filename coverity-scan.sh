#!/usr/bin/env bash
# Coverity scan script
#
# To run this script you need to provide API token. This can be done either by:
#  - Putting token in ".coverity-token" file
#  - Assigning token value to COVERITY_SCAN_TOKEN environment variable
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Costa Tsaousis (costa@netdata.cloud)
# Author  : Pawel Krupa (paulfantom)
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)

cpus=$(grep -c ^processor </proc/cpuinfo)
[ -z "${cpus}" ] && cpus=1

token="${COVERITY_SCAN_TOKEN}"
([ -z "${token}" ] && [ -f .coverity-token ]) && token="$(<.coverity-token)"
if [ -z "${token}" ]; then
	echo >&2 "Save the coverity token to .coverity-token or export it as COVERITY_SCAN_TOKEN."
	exit 1
fi

export PATH=${PATH}:/opt/coverity/bin/
covbuild="$(which cov-build 2>/dev/null || command -v cov-build 2>/dev/null)"
([ -z "${covbuild}" ] && [ -f .coverity-build ]) && covbuild="$(<.coverity-build)"
if [ -z "${covbuild}" ]; then
	echo >&2 "Cannot find 'cov-build' binary in \$PATH."
	exit 1
elif [ ! -x "${covbuild}" ]; then
	echo >&2 "The command ${covbuild} is not executable. Save command the full filename of cov-build in .coverity-build"
	exit 1
fi

version="$(grep "^#define PACKAGE_VERSION" config.h | cut -d '"' -f 2)"
echo >&2 "Working on netdata version: ${version}"

echo >&2 "Cleaning up old builds..."
make clean || echo >&2 "Nothing to clean"

[ -d "cov-int" ] && rm -rf "cov-int"

[ -f netdata-coverity-analysis.tgz ] && rm netdata-coverity-analysis.tgz

autoreconf -ivf
./configure --enable-plugin-nfacct --enable-plugin-freeipmi
"${covbuild}" --dir cov-int make -j${cpus} || exit 1

echo >&2 "Compressing data..."
tar czvf netdata-coverity-analysis.tgz cov-int || exit 1

echo >&2 "Sending analysis for version ${version} ..."
COVERITY_SUBMIT_RESULT=$(curl --progress-bar --form token="${token}" \
  --form email=${COVERITY_SCAN_SUBMIT_MAIL} \
  --form file=@netdata-coverity-analysis.tgz \
  --form version="${version}" \
  --form description="netdata, real-time performance monitoring, done right." \
  https://scan.coverity.com/builds?project=${REPOSITORY})

echo ${COVERITY_SUBMIT_RESULT} | grep -q -e 'Build successfully submitted' || echo >&2 "scan results were not pushed to coverity. Message was: ${COVERITY_SUBMIT_RESULT}"

echo >&2 "Coverity scan mechanism completed"
