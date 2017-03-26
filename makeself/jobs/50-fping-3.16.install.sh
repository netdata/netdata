#!/usr/bin/env bash

. ${NETDATA_MAKESELF_PATH}/functions.sh "${@}" || exit 1

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

