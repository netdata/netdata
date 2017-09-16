#!/usr/bin/env bash

. $(dirname "${0}")/../functions.sh "${@}" || exit 1

fetch "fping-4.0" "https://github.com/schweikert/fping/releases/download/v4.0/fping-4.0.tar.gz"

export CFLAGS="-static"

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

if [ ${NETDATA_BUILD_WITH_DEBUG} -eq 0 ]
then
    run strip ${NETDATA_INSTALL_PATH}/bin/fping
fi
