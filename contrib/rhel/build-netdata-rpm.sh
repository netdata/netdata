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

typeset version="$(grep PACKAGE_VERSION < config.h | cut -d '"' -f 2)"
if [[ -z "${version}" ]]; then
	run_failed "Cannot find netdata version."
	exit 1
fi

if [[ "${version//-/}" != "$version" ]]; then
	# Remove all -* and _* suffixes to be as close as netdata release
	typeset versionfix="${version%%-*}"; versionfix="${versionfix%%_*}"
	# Append the current datetime fox a 'unique' build
	versionfix+="_$(date '+%m%d%H%M%S')"
	# And issue hints & details on why this failed, and how to fix it
	run_failed "Current version contains '-' which is forbidden by rpm. You must create a git annotated tag and rerun this script. Example:"
	run_failed "  git tag -a $versionfix -m 'Release by $(id -nu) on $(uname -n)' && $0"
	exit 1
fi


typeset tgz="netdata-${version}.tar.gz"
if [[ ! -f "${tgz}" ]]; then
	run_failed "Cannot find the generated tar.gz file '${tgz}'"
	exit 1
fi

typeset srpm="$(run rpmbuild -ts "${tgz}" | cut -d ' ' -f 2)"
if [[ -z "${srpm}" ]] || ! [[ -f "${srpm}" ]]; then
	run_failed "Cannot find the generated SRPM file '${srpm}'"
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

run_ok "All done! Packages created in '$(rpm -E '%_rpmdir/%_arch')'"
