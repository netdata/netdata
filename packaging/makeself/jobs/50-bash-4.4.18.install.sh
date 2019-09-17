#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

. $(dirname "${0}")/../functions.sh "${@}" || exit 1

fetch "bash-4.4.18" "http://ftp.gnu.org/gnu/bash/bash-4.4.18.tar.gz"

run ./configure \
	--prefix=${NETDATA_INSTALL_PATH} \
	--without-bash-malloc \
	--enable-static-link \
	--enable-net-redirections \
	--enable-array-variables \
	--disable-profiling \
	--disable-nls \
#	--disable-rpath \
#	--enable-alias \
#	--enable-arith-for-command \
#	--enable-array-variables \
#	--enable-brace-expansion \
#	--enable-casemod-attributes \
#	--enable-casemod-expansions \
#	--enable-command-timing \
#	--enable-cond-command \
#	--enable-cond-regexp \
#	--enable-directory-stack \
#	--enable-dparen-arithmetic \
#	--enable-function-import \
#	--enable-glob-asciiranges-default \
#	--enable-help-builtin \
#	--enable-job-control \
#	--enable-net-redirections \
#	--enable-process-substitution \
#	--enable-progcomp \
#	--enable-prompt-string-decoding \
#	--enable-readline \
#	--enable-select \


run make clean
run make -j$(find_processors)

cat >examples/loadables/Makefile <<EOF
all:
clean:
install:
EOF

run make install

if [ ${NETDATA_BUILD_WITH_DEBUG} -eq 0 ]
then
    run strip ${NETDATA_INSTALL_PATH}/bin/bash
fi
