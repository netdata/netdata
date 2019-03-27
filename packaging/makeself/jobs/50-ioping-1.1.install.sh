#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

. $(dirname "${0}")/../functions.sh "${@}" || exit 1

git clone "https://github.com/koct9i/ioping.git ioping-1.1"

export CFLAGS="-static"

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
    run strip ${NETDATA_INSTALL_PATH}/bin/ioping
fi
