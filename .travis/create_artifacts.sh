#!/bin/bash
# shellcheck disable=SC2230

set -e

if [ ! -f .gitignore ]; then
	echo "Run as ./travis/$(basename "$0") from top level directory of git repository"
	exit 1
fi

# Everything from this directory will be uploaded to GCS
mkdir -p artifacts
BASENAME="netdata-$(git describe)"

# Make sure stdout is in blocking mode. If we don't, then conda create will barf during downloads.
# See https://github.com/travis-ci/travis-ci/issues/4704#issuecomment-348435959 for details.
python -c 'import os,sys,fcntl; flags = fcntl.fcntl(sys.stdout, fcntl.F_GETFL); fcntl.fcntl(sys.stdout, fcntl.F_SETFL, flags&~os.O_NONBLOCK);'
echo "--- Create tarball ---"
autoreconf -ivf
./configure --prefix=/usr --sysconfdir=/etc --localstatedir=/var --with-zlib --with-math --with-user=netdata CFLAGS=-O2 
make dist
mv "${BASENAME}.tar.gz" artifacts/

echo "--- Create self-extractor ---"
./packaging/makeself/build-x86_64-static.sh

# Needed fo GCS
echo "--- Copy artifacts to separate directory ---"
#shellcheck disable=SC2164
cd artifacts
ln -s "${BASENAME}.tar.gz" netdata-latest.tar.gz
ln -s "${BASENAME}.gz.run" netdata-latest.gz.run
sha256sum -b ./* >"sha256sums.txt"
echo "checksums:"
cat sha256sums.txt

