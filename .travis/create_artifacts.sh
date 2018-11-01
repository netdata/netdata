#!/bin/bash
# shellcheck disable=SC2230

if [ ! -f .gitignore ]
then
  echo "Run as ./travis/$(basename "$0") from top level directory of git repository"
  exit 1
fi

# Make sure stdout is in blocking mode. If we don't, then conda create will barf during downloads.
# See https://github.com/travis-ci/travis-ci/issues/4704#issuecomment-348435959 for details.
python -c 'import os,sys,fcntl; flags = fcntl.fcntl(sys.stdout, fcntl.F_GETFL); fcntl.fcntl(sys.stdout, fcntl.F_SETFL, flags&~os.O_NONBLOCK);'
echo "--- Create tarball ---"
autoreconf -ivf
./configure
make dist
echo "--- Create self-extractor ---"
./makeself/build-x86_64-static.sh

echo "--- Create checksums ---"
GIT_TAG=$(git tag --points-at)
if [ "${GIT_TAG}" != "" ]; then
    ln -s netdata-latest.gz.run "netdata-${GIT_TAG}.gz.run"
    ln -s netdata-*.tar.gz "netdata-${GIT_TAG}.tar.gz"
    sha256sum -b "netdata-${GIT_TAG}.gz.run" "netdata-${GIT_TAG}.tar.gz" > "sha256sums.txt"
else
    sha256sum -b ./*.tar.gz ./*.gz.run > "sha256sums.txt"
fi

echo "checksums:"
cat sha256sums.txt


