#!/usr/bin/env sh

# this script should be running in alpine linux
# install the required packages
apk add \
    bash \
    wget \
    curl \
    ncurses \
    git \
    netcat-openbsd \
    alpine-sdk \
    autoconf \
    automake \
    gcc \
    make \
    pkgconfig \
    util-linux-dev \
    zlib-dev \
    libmnl-dev \
    libnetfilter_acct-dev \
    || exit 1

cd $(dirname "$0") || exit 1

# if we don't run inside the netdata repo
# download it and run from it
if [ ! -f ../netdata-installer.sh ]
then
    git clone https://github.com/firehol/netdata.git netdata.git || exit 1
    cd netdata.git/makeself || exit 1
    ./build.sh "$@"
    exit $?
fi

cat >&2 <<EOF

This program will create a self-extracting shell package containing
a statically linked netdata, able to run on any 64bit Linux system,
without any dependencies from the target system.

It can be used to have netdata running in no-time, or in cases the
target Linux system cannot compile netdata.

EOF

read "Press ENTER to continue > "

mkdir tmp || exit 1

./run-all-jobs.sh "$@"
exit $?
