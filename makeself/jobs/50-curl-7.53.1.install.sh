#!/usr/bin/env bash

exit 0

. $(dirname "${0}")/../functions.sh "${@}" || exit 1

fetch "curl-curl-7_53_1" "https://github.com/curl/curl/archive/curl-7_53_1.tar.gz"

export CFLAGS="-static -DCURL_STATICLIB"
export LDFLAGS="-static"
export PKG_CONFIG="pkg-config --static"
export curl_LDFLAGS="-all-static"

run ./buildconf

run ./configure \
	--prefix=${NETDATA_INSTALL_PATH} \
	--enable-optimize \
	--disable-shared \
	--enable-static \
	--enable-http \
	--enable-proxy \
	--enable-ipv6 \
	--enable-cookies \
	--disable-ldap \
	--disable-sspi \
	--without-librtmp \
	--disable-ftp \
	--disable-file \
	--disable-dict \
	--disable-telnet \
	--disable-tftp \
	--disable-rtsp \
	--disable-pop3 \
	--disable-imap \
	--disable-smtp \
	--disable-gopher \
	--disable-smb \
	--disable-ares \
	--without-libidn \
	${NULL}

run make clean
run make -j${PROCESSORS}
run make install
