#!/usr/bin/env bash

set -e

build() {
  cp -r /code /tmp/code
  cd /tmp/code

  yum -y install rpm-build redhat-rpm-config yum-utils autoconf automake curl gcc git libmnl-devel libuuid-devel make pkgconfig zlib-devel

  autoreconf -ivf
  ./configure --enable-maintainer-mode
  make dist

  tgz="$(ls netdata-v*.tar.gz)"
  if [ ! -f "${tgz}" ]; then
    echo >&2 "Cannot find the generated tar.gz file '${tgz}'"
    exit 1
  fi

  srpm=$(rpmbuild -ts "${tgz}" | cut -d ' ' -f 2)
  if [ -z "${srpm}" ] || [ ! -f "${srpm}" ]; then
    echo >&2 "Cannot find the generated SRPM file '${srpm}'"
    exit 1
  fi

  rpmbuild --rebuild "${srpm}"

  cp /root/rpmbuild/RPMS/x86_64/netdata-* ./

  echo >&2 "All done!"
}

VERSION="7"
case "$1" in
  "centos7") VERSION="7";;
  "centos6") VERSION="6.9";;
esac

if [ "$IS_CONTAINER" != "" ]; then
  build
else
  cd "$(dirname "$0")/../../" || exit 1
  docker run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/code:Z" \
    --workdir /code \
    "centos:$VERSION" \
    ./contrib/rhel/build-netdata-rpm.sh
fi
