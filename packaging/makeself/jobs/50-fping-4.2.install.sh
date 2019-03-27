#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

. $(dirname "${0}")/../functions.sh "${@}" || exit 1

fetch "fping-4.2" "https://github.com/schweikert/fping/releases/download/v4.2/fping-4.2.tar.gz"

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
run make -j${SYSTEM_CPUS}
run make install

if [ ${NETDATA_BUILD_WITH_DEBUG} -eq 0 ]
then
    run strip ${NETDATA_INSTALL_PATH}/bin/fping
fi
