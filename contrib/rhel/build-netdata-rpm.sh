#!/usr/bin/env bash

# docker run -it --rm centos:6.9 /bin/sh
# yum -y install rpm-build redhat-rpm-config yum-utils autoconf automake curl gcc git libmnl-devel libuuid-devel make pkgconfig zlib-devel

cd "$(dirname "$0")/../../" || exit 1
# shellcheck disable=SC1091
source "packaging/installer/functions.sh" || exit 1

set -e

run autoreconf -ivf
run ./configure --enable-maintainer-mode
run make dist

version=$(grep PACKAGE_VERSION < config.h | cut -d '"' -f 2)
if [ -z "${version}" ]
then
    echo >&2 "Cannot find netdata version."
    exit 1
fi

tgz="netdata-${version}.tar.gz"
if [ ! -f "${tgz}" ]
then
	echo >&2 "Cannot find the generated tar.gz file '${tgz}'"
	exit 1
fi

srpm=$(run rpmbuild -ts "${tgz}" | cut -d ' ' -f 2)
if [ -z "${srpm}" ] || [ ! -f "${srpm}" ]
then
	echo >&2 "Cannot find the generated SRPM file '${srpm}'"
	exit 1
fi

#if which yum-builddep 2>/dev/null
#then
#    run yum-builddep "${srpm}"
#elif which dnf 2>/dev/null
#then
#    [ "${UID}" = 0 ] && run dnf builddep "${srpm}"
#fi

run rpmbuild --rebuild "${srpm}"

echo >&2 "All done!"
