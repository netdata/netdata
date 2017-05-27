#!/usr/bin/env sh

if [ "$(uname -m)" != "x86_64" ]
	then
	echo >&2 "Static binary versions of netdata are available only for 64bit Intel/AMD CPUs (x86_64), but yours is: $(uname -m)."
	exit 1
fi

if [ "$(uname -s)" != "Linux" ]
	then
	echo >&2 "Static binary versions of netdata are available only for Linux, but this system is $(uname -s)"
	exit 1
fi

BASE='https://raw.githubusercontent.com/firehol/binary-packages/master'

echo >&2 "Checking the latest version of static build..."
LATEST="$(curl -Ss "${BASE}/netdata-latest.gz.run")"

if [ -z "${LATEST}" ]
	then
	echo >&2 "Cannot find the latest static binary version of netdata."
	exit 1
fi

echo >&2 "Downloading static netdata binary: ${LATEST}"
curl "${BASE}/${LATEST}" >"/tmp/${LATEST}"
if [ $? -ne 0 ]
	then
	echo >&2 "Failed to download the latest static binary version of netdata."
	exit 1
fi
chmod 755 "/tmp/${LATEST}"

echo >&2 "Executing the downloaded self-extracting archive"
sudo sh "/tmp/${LATEST}"

