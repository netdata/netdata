#!/usr/bin/env sh
#
# Installation script for the alpine host
# to prepare the static binary
#
# Copyright 2018-2025 Netdata Inc.
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later

apk update || exit 1
apk upgrade || exit 1

# Add required APK packages
apk add --no-cache -U \
  alpine-sdk \
  autoconf \
  automake \
  bash \
  binutils \
  cmake \
  curl \
  elfutils-dev \
  gcc \
  git \
  gnutls-dev \
  gzip \
  jq \
  libelf-static \
  libmnl-dev \
  libnetfilter_acct-dev \
  libtool \
  libuv-dev \
  libuv-static \
  lz4-dev \
  lz4-static \
  make \
  ncurses \
  netcat-openbsd \
  openssh \
  pkgconfig \
  protobuf-dev \
  snappy-dev \
  snappy-static \
  util-linux-dev \
  wget \
  xz \
  zlib-dev \
  zlib-static ||
  exit 1
