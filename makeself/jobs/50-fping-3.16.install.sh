#!/usr/bin/env bash

. $(dirname "${0}")/../functions.sh "${@}" || exit 1

fetch "fping-3.16" "https://github.com/schweikert/fping/archive/3.16.tar.gz"

export CFLAGS="-static"

run ./autogen.sh

run ./configure \
	--prefix=${NETDATA_INSTALL_PATH} \
	--enable-ipv4 \
	--enable-ipv6 \
	${NULL}

cat >doc/Makefile <<EOF
all:
clean:
install:
EOF

run make clean
run make -j${PROCESSORS}
run make install

run strip ${NETDATA_INSTALL_PATH}/bin/fping
run strip ${NETDATA_INSTALL_PATH}/bin/fping6
