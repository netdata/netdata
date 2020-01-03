#!/usr/bin/env sh
#
# Installation script for the alpine host
# to prepare the static binary
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author: Paul Emm. Katsoulakis <paul@netdata.cloud>

# Packaging update
apk update

# Add required APK packages
apk add --no-cache \
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
    cmake \
    libtool \
    pkgconfig \
    util-linux-dev \
    openssl-dev \
    gnutls-dev \
    zlib-dev \
    libmnl-dev \
    libnetfilter_acct-dev \
    libuv-dev \
    lz4-dev \
    openssl-dev \
    snappy-dev \
    protobuf-dev \
    || exit 1

# snappy doesnt have static version in alpine, let's compile it
export SNAPPY_VER="1.1.7"
wget -O /snappy.tar.gz https://github.com/google/snappy/archive/${SNAPPY_VER}.tar.gz
cd /
tar -xf snappy.tar.gz
rm snappy.tar.gz
cd /snappy-${SNAPPY_VER}
mkdir build
cd build
cmake -DCMAKE_BUILD_SHARED_LIBS=true -DCMAKE_INSTALL_PREFIX:PATH=/usr -DCMAKE_INSTALL_LIBDIR=lib ../
make && make install

# Judy doesnt seem to be available on the repositories, download manually and install it
export JUDY_VER="1.0.5"
wget -O /judy.tar.gz http://downloads.sourceforge.net/project/judy/judy/Judy-${JUDY_VER}/Judy-${JUDY_VER}.tar.gz
cd /
tar -xf judy.tar.gz
rm judy.tar.gz
cd /judy-${JUDY_VER}
CFLAGS="-O2 -s" CXXFLAGS="-O2 -s" ./configure
make
make install;
