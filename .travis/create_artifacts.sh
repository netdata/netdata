#!/usr/bin/env bash
#
# Artifacts creation script.
# This script generates two things:
#   1) The static binary that can run on all linux distros (built-in dependencies etc)
#   2) The distribution source tarbal
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author: Paul Emm. Katsoulakis <paul@netdata.cloud>
#
# shellcheck disable=SC2230

set -e

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup || echo "")
if [ -n "${CWD}" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/$(basename "$0") from top level directory of netdata git repository"
    exit 1
fi

if [ ! "${TRAVIS_REPO_SLUG}" == "netdata/netdata" ]; then
	echo "Beta mode on ${TRAVIS_REPO_SLUG}, not running anything here"
	exit 0
fi;


echo "--- Initialize git configuration ---"
git checkout "${1-master}"
git pull

if [ "${RELEASE_CHANNEL}" == stable ]; then
  echo "--- Set default release channel to stable ---"
  sed -i 's/^RELEASE_CHANNEL="nightly" *#/RELEASE_CHANNEL="stable" #/' \
    netdata-installer.sh \
    packaging/makeself/install-or-update.sh
fi

# Everything from this directory will be uploaded to GCS
mkdir -p artifacts
BASENAME="netdata-$(git describe)"

# Make sure stdout is in blocking mode. If we don't, then conda create will barf during downloads.
# See https://github.com/travis-ci/travis-ci/issues/4704#issuecomment-348435959 for details.
python -c 'import os,sys,fcntl; flags = fcntl.fcntl(sys.stdout, fcntl.F_GETFL); fcntl.fcntl(sys.stdout, fcntl.F_SETFL, flags&~os.O_NONBLOCK);'
echo "--- Create tarball ---"
autoreconf -ivf
./configure --prefix=/usr --sysconfdir=/etc --localstatedir=/var --libexecdir=/usr/libexec --with-zlib --with-math --with-user=netdata CFLAGS=-O2
make dist
mv "${BASENAME}.tar.gz" artifacts/

echo "--- Create self-extractor ---"
./packaging/makeself/build-x86_64-static.sh

# Needed for GCS
echo "--- Copy artifacts to separate directory ---"
#shellcheck disable=SC2164
cp packaging/version artifacts/latest-version.txt
cd artifacts
ln -s "${BASENAME}.tar.gz" netdata-latest.tar.gz
ln -s "${BASENAME}.gz.run" netdata-latest.gz.run
sha256sum -b ./* >"sha256sums.txt"

# TODO: Remove this after 1 successfuly nightly or until there are no v1.19.0-432 instances left
# XXX: See: https://github.com/netdata/netdata/issues/8056
sed -i -e '/netdata-v/d' sha256sums.txt

echo "checksums:"
cat sha256sums.txt
