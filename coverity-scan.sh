#!/usr/bin/env bash

# To run this script you need to provide API token. This can be done either by:
#  - Putting token in ".coverity-token" file
#  - Assigning token value to COVERITY_SCAN_TOKEN environment variable
# Additionally script can install coverity tool on your computer. To do this just set environment variable INSTALL_COVERITY to "true"

cpus=$(grep -c ^processor </proc/cpuinfo)
[ -z "${cpus}" ] && cpus=1

token="${COVERITY_SCAN_TOKEN}"
([ -z "${token}" ] && [ -f .coverity-token ]) && token="$(<.coverity-token)"
if [ -z "${token}" ]; then
	echo >&2 "Save the coverity token to .coverity-token or export it as COVERITY_SCAN_TOKEN."
	exit 1
fi

covbuild="$(which cov-build 2>/dev/null || command -v cov-build 2>/dev/null)"
([ -z "${covbuild}" ] && [ -f .coverity-build ]) && covbuild="$(<.coverity-build)"
if [ -z "${covbuild}" ]; then
	echo "Cannot find 'cov-build' binary in \$PATH."
	if [ $INSTALL_COVERITY != "" ]; then
		echo "Installing coverity..."
		mkdir /tmp/coverity
        	curl -SL --data "token=${token}&project=netdata%2Fnetdata" https://scan.coverity.com/download/linux64 > /tmp/coverity_tool.tar.gz
        	tar -x-C /tmp/coverity/ -f /tmp/coverity_tool.tar.gz
	        sudo mv /tmp/coverity/cov-analysis-linux64-2017.07 /opt/coverity
        	export PATH=${PATH}:/opt/coverity/bin/
        else
		echo "Save command the full filename of cov-build in .coverity-build"
		exit 1
	fi
fi

if [ ! -x "${covbuild}" ]; then
	echo "The command ${covbuild} is not executable. Save command the full filename of cov-build in .coverity-build"
	exit 1
fi

version="$(grep "^#define PACKAGE_VERSION" config.h | cut -d '"' -f 2)"
echo >&2 "Working on netdata version: ${version}"

echo >&2 "Cleaning up old builds..."
make clean || exit 1

[ -d "cov-int" ] && rm -rf "cov-int"

[ -f netdata-coverity-analysis.tgz ] && rm netdata-coverity-analysis.tgz

autoreconf -ivf
./configure --enable-plugin-nfacct --enable-plugin-freeipmi
"${covbuild}" --dir cov-int make -j${cpus} || exit 1

echo >&2 "Compressing data..."
tar czvf netdata-coverity-analysis.tgz cov-int || exit 1

echo >&2 "Sending analysis for version ${version} ..."
curl --progress-bar --form token="${token}" \
  --form email=costa@tsaousis.gr \
  --form file=@netdata-coverity-analysis.tgz \
  --form version="${version}" \
  --form description="netdata, real-time performance monitoring, done right." \
  https://scan.coverity.com/builds?project=netdata%2Fnetdata
