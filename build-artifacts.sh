#!/bin/sh

BASENAME="netdata-$(git describe)"

mkdir -p artifacts

autoreconf -ivf
./configure \
  --prefix=/usr \
  --sysconfdir=/etc \
  --localstatedir=/var \
  --libexecdir=/usr/libexec \
  --with-zlib \
  --with-math \
  --with-user=netdata \
  CFLAGS=-O2
make dist
mv "${BASENAME}.tar.gz" artifacts/

USER="" ./packaging/makeself/build-x86_64-static.sh

cp packaging/version artifacts/latest-version.txt

cd artifacts || exit 1
ln -s "${BASENAME}.tar.gz" netdata-latest.tar.gz
ln -s "${BASENAME}.gz.run" netdata-latest.gz.run
sha256sum -b ./* > "sha256sums.txt"
