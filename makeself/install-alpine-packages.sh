#!/usr/bin/env sh
# SPDX-License-Identifier: GPL-3.0+

# this script should be running in alpine linux
# install the required packages
apk update
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
    libtool \
    pkgconfig \
    util-linux-dev \
    openssl-dev \
    gnutls-dev \
    zlib-dev \
    libmnl-dev \
    libnetfilter_acct-dev \
    || exit 1
