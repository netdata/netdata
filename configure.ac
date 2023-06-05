#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
# SPDX-License-Identifier: GPL-3.0-or-later
#
AC_PREREQ(2.60)

# We do not use m4_esyscmd_s to support older autoconf.
define([VERSION_STRING], m4_esyscmd([git describe 2>/dev/null | tr -d '\n']))
define([VERSION_FROM_FILE], m4_esyscmd([cat packaging/version | tr -d '\n']))
m4_ifval(VERSION_STRING, [], [define([VERSION_STRING], VERSION_FROM_FILE)])

AC_INIT([netdata], VERSION_STRING[])

AM_MAINTAINER_MODE([disable])
if test x"$USE_MAINTAINER_MODE" = xyes; then
AC_MSG_NOTICE(***************** MAINTAINER MODE *****************)
fi

PACKAGE_RPM_VERSION="VERSION_STRING"
AC_SUBST([PACKAGE_RPM_VERSION])

# -----------------------------------------------------------------------------
# autoconf initialization

AC_CONFIG_AUX_DIR([.])
AC_CONFIG_HEADERS([config.h])
AC_CONFIG_MACRO_DIR([build/m4])
AC_CONFIG_SRCDIR([daemon/main.c])
define([AUTOMATE_INIT_OPTIONS], [tar-pax subdir-objects])
m4_ifdef([AM_SILENT_RULES], [
    define([AUTOMATE_INIT_OPTIONS], [tar-pax silent-rules subdir-objects])
    ])
AM_INIT_AUTOMAKE(AUTOMATE_INIT_OPTIONS)
m4_ifdef([AM_SILENT_RULES], [
    AM_SILENT_RULES([yes])
    ])
AC_CANONICAL_HOST
AC_PROG_CC
AC_PROG_CC_C99
AM_PROG_CC_C_O
AC_PROG_CXX
AC_PROG_INSTALL
PKG_PROG_PKG_CONFIG
AC_USE_SYSTEM_EXTENSIONS

AC_LANG_PUSH([C])
AX_COMPILER_VENDOR
AC_LANG_POP([C])
if test "$ax_cv_c_compiler_vendor" = "clang"; then
  AC_CHECK_PROG([AR], [llvm-ar], [llvm-ar], [ar])
  AC_CHECK_PROG([RANLIB], [llvm-ranlib], [llvm-ranlib], [ranlib])
else
  AC_CHECK_TOOL([RANLIB], [ranlib])
  AC_CHECK_TOOL([AR], [ar])
fi

# -----------------------------------------------------------------------------
# configurable options

AC_ARG_ENABLE(
    [plugin-nfacct],
    [AS_HELP_STRING([--enable-plugin-nfacct], [enable nfacct plugin @<:@default autodetect@:>@])],
    ,
    [enable_plugin_nfacct="detect"]
)
AC_ARG_ENABLE(
    [plugin-freeipmi],
    [AS_HELP_STRING([--enable-plugin-freeipmi], [enable freeipmi plugin @<:@default autodetect@:>@])],
    ,
    [enable_plugin_freeipmi="detect"]
)
AC_ARG_ENABLE(
    [plugin-cups],
    [AS_HELP_STRING([--enable-plugin-cups], [enable cups plugin @<:@default autodetect@:>@])],
    ,
    [enable_plugin_cups="detect"]
)
AC_ARG_ENABLE(
    [plugin-xenstat],
    [AS_HELP_STRING([--enable-plugin-xenstat], [enable xenstat plugin @<:@default autodetect@:>@])],
    ,
    [enable_plugin_xenstat="detect"]
)
AC_ARG_ENABLE(
    [exporting-kinesis],
    [AS_HELP_STRING([--enable-exporting-kinesis], [enable kinesis exporting connector @<:@default autodetect@:>@])],
    ,
    [enable_exporting_kinesis="detect"]
)
AC_ARG_ENABLE(
    [exporting-pubsub],
    [AS_HELP_STRING([--enable-exporting-pubsub], [enable pubsub exporting connector @<:@default autodetect@:>@])],
    ,
    [enable_exporting_pubsub="detect"]
)
AC_ARG_ENABLE(
    [exporting-prometheus-remote-write],
    [AS_HELP_STRING([--enable-exporting-prometheus-remote-write], [enable prometheus remote write exporting connector @<:@default autodetect@:>@])],
    ,
    [enable_exporting_prometheus_remote_write="detect"]
)
AC_ARG_ENABLE(
    [exporting-mongodb],
    [AS_HELP_STRING([--enable-exporting-mongodb], [enable mongodb exporting @<:@default autodetect@:>@])],
    ,
    [enable_exporting_mongodb="detect"]
)
AC_ARG_ENABLE(
    [pedantic],
    [AS_HELP_STRING([--enable-pedantic], [enable pedantic compiler warnings @<:@default disabled@:>@])],
    ,
    [enable_pedantic="no"]
)
AC_ARG_ENABLE(
    [accept4],
    [AS_HELP_STRING([--disable-accept4], [System does not have accept4 @<:@default autodetect@:>@])],
    ,
    [enable_accept4="detect"]
)
AC_ARG_WITH(
    [webdir],
    [AS_HELP_STRING([--with-webdir], [location of webdir @<:@PKGDATADIR/web@:>@])],
    [webdir="${withval}"],
    [webdir="\$(pkgdatadir)/web"]
)
AC_ARG_WITH(
    [libcap],
    [AS_HELP_STRING([--with-libcap], [build with libcap @<:@default autodetect@:>@])],
    ,
    [with_libcap="detect"]
)
AC_ARG_WITH(
    [math],
    [AS_HELP_STRING([--without-math], [build without math @<:@default enabled@:>@])],
    ,
    [with_math="yes"]
)
AC_ARG_WITH(
    [user],
    [AS_HELP_STRING([--with-user], [use this user to drop privilege @<:@default nobody@:>@])],
    ,
    [with_user="nobody"]
)
AC_ARG_ENABLE(
    [x86-sse],
    [AS_HELP_STRING([--disable-x86-sse], [SSE/SS2 optimizations on x86 @<:@default enabled@:>@])],
    ,
    [enable_x86_sse="yes"]
)
AC_ARG_ENABLE(
    [lto],
    [AS_HELP_STRING([--enable-lto], [Link Time Optimizations @<:@default disabled@:>@])],
    ,
    [enable_lto="no"]
)
AC_ARG_ENABLE(
    [https],
    [AS_HELP_STRING([--enable-https], [Enable SSL support @<:@default autodetect@:>@])],
    ,
    [enable_https="detect"]
)
AC_ARG_ENABLE(
    [compression],
    [AS_HELP_STRING([--enable-compression], [Enable LZ4 compression support @<:@default autodetect@:>@])],
    ,
    [enable_compression="detect"]
)
AC_ARG_ENABLE(
    [dbengine],
    [AS_HELP_STRING([--disable-dbengine], [disable netdata dbengine @<:@default autodetect@:>@])],
    ,
    [enable_dbengine="detect"]
)
AC_ARG_ENABLE(
    [jsonc],
    [AS_HELP_STRING([--enable-jsonc], [Enable JSON-C support @<:@default autodetect@:>@])],
    ,
    [enable_jsonc="detect"]
)
AC_ARG_ENABLE(
    [ebpf],
    [AS_HELP_STRING([--disable-ebpf], [Disable eBPF support @<:@default autodetect@:>@])],
    ,
    [enable_ebpf="detect"]
)
AC_ARG_WITH(
    [bundled-lws],
    [AS_HELP_STRING([--with-bundled-lws], [Use the bundled version of libwebsockets library @<:@default use system library@:>@])],
    [with_bundled_lws="yes"], [with_bundled_lws="no"]
)
AC_ARG_WITH(
    [bundled-protobuf],
    [AS_HELP_STRING([--with-bundled-protobuf],
                    [Uses the bundled version of Google Protocol Buffers @<:@default bundled if present@:>@])],
    [with_bundled_protobuf="$withval"],
    [with_bundled_protobuf="detect"]
)
AC_ARG_ENABLE(
    [ml],
    [AS_HELP_STRING([--enable-ml], [Enable anomaly detection @<:@default autodetect@:>@])],
    ,
    [enable_ml="detect"]
)
AC_ARG_ENABLE(
    [aclk_ssl_debug],
    [AS_HELP_STRING([--enable-aclk-ssl-debug], [Enables possibility for SSL key logging @<:@default no@:>@])],
    [aclk_ssl_debug="yes"],
    [aclk_ssl_debug="no"]
)
AC_ARG_ENABLE(
    [httpd],
    [AS_HELP_STRING([--disable-httpd], [Disable webserver (h2o based) @<:@default autodetect@:>@])],
    ,
    [enable_httpd="detect"]
)

# -----------------------------------------------------------------------------
# Enforce building with C99, bail early if we can't.
test "${ac_cv_prog_cc_c99}" = "no" && AC_MSG_ERROR([Netdata requires a compiler that supports C99 to build])

# -----------------------------------------------------------------------------
# Check if cloud is enabled and if the functionality is available

AC_ARG_ENABLE(
    [cloud],
    [AS_HELP_STRING([--disable-cloud],
                    [Disables all cloud functionality])],
    [ enable_cloud="$enableval" ],
    [ enable_cloud="detect" ]
)

if test "${enable_cloud}" = "no"; then
    AC_DEFINE([DISABLE_CLOUD], [1], [disable netdata cloud functionality])
fi

# -----------------------------------------------------------------------------
# C++ version check

# Check for C++17 support (optional)
# AX_CXX_COMPILE_STDCXX(17, noext, optional)

if test "x$HAVE_CXX17" != "x1"; then
  # Check for C++11 support (optional)
  AX_CXX_COMPILE_STDCXX(11, noext, optional)
fi

AC_MSG_CHECKING([c++ standard to use])
if test "x$HAVE_CXX17" = "x1"; then
  have_cxx17="yes"
  have_cxx11="yes"
  CPP_STD_FLAG="-std=c++17"
  cpp_std_to_use="c++17"
  AM_CONDITIONAL([HAVE_CXX17], [true])
  AM_CONDITIONAL([HAVE_CXX11], [true])
elif test "x$HAVE_CXX11" = "x1"; then
  have_cxx17="no"
  have_cxx11="yes"
  CPP_STD_FLAG="-std=c++11"
  cpp_std_to_use="c++11"
  AM_CONDITIONAL([HAVE_CXX17], [false])
  AM_CONDITIONAL([HAVE_CXX11], [true])
else
  have_cxx17="no"
  have_cxx11="no"
  CPP_STD_FLAG=""
  cpp_std_to_use="no c++"
  AM_CONDITIONAL([HAVE_CXX17], [false])
  AM_CONDITIONAL([HAVE_CXX11], [false])
fi

# PPC64LE needs -std=gnu++11 in order to build dlib. However, the rest of
# the agent's components use and have been tested only with -std=c++11.
# Skip ML compilation on that CPU until we reorganize and test the C++ flags.
if test "${host_cpu}" = "powerpc64le"; then
  have_cxx17="no"
  have_cxx11="no"
  CPP_STD_FLAG=""
  cpp_std_to_use="no c++ on powerpc64le"
  AM_CONDITIONAL([HAVE_CXX17], [false])
  AM_CONDITIONAL([HAVE_CXX11], [false])
fi

AC_MSG_RESULT([${cpp_std_to_use}])

# -----------------------------------------------------------------------------
# netdata required checks

# fails on centos6
#AX_CHECK_ENABLE_DEBUG()

AX_GCC_FUNC_ATTRIBUTE([returns_nonnull])
AX_GCC_FUNC_ATTRIBUTE([malloc])
AX_GCC_FUNC_ATTRIBUTE([noreturn])
AX_GCC_FUNC_ATTRIBUTE([noinline])
AX_GCC_FUNC_ATTRIBUTE([format])
AX_GCC_FUNC_ATTRIBUTE([warn_unused_result])

AC_CHECK_TYPES([struct timespec, clockid_t], [], [], [[#include <time.h>]])
AC_SEARCH_LIBS([clock_gettime], [rt posix4])
AC_CHECK_FUNCS([clock_gettime])
AC_CHECK_FUNCS([sched_setscheduler sched_getscheduler sched_getparam sched_get_priority_min sched_get_priority_max getpriority setpriority nice])
AC_CHECK_FUNCS([recvmmsg])
AC_CHECK_FUNCS([close_range])

AC_TYPE_INT8_T
AC_TYPE_INT16_T
AC_TYPE_INT32_T
AC_TYPE_INT64_T
AC_TYPE_UINT8_T
AC_TYPE_UINT16_T
AC_TYPE_UINT32_T
AC_TYPE_UINT64_T
AC_C_INLINE
AC_FUNC_STRERROR_R
AC_C__GENERIC
# AC_C_STMT_EXPR
AC_CANONICAL_HOST
AC_HEADER_MAJOR
AC_HEADER_RESOLV

AC_CHECK_HEADERS_ONCE([sys/prctl.h])
AC_CHECK_HEADERS_ONCE([sys/vfs.h])
AC_CHECK_HEADERS_ONCE([sys/statfs.h])
AC_CHECK_HEADERS_ONCE([linux/magic.h])
AC_CHECK_HEADERS_ONCE([sys/statvfs.h])
AC_CHECK_HEADERS_ONCE([sys/mount.h])

if test "${enable_accept4}" != "no"; then
    AC_CHECK_FUNCS_ONCE(accept4)
fi

AC_CHECK_FUNCS_ONCE(malloc_usable_size)

# -----------------------------------------------------------------------------
# operating system detection

originalCFLAGS="${CFLAGS}"
AC_MSG_CHECKING([operating system])
case "$host_os" in
freebsd*)
    build_target=freebsd
    OPTIONAL_OS_DEP_CFLAGS="-I/usr/local/include"
    CFLAGS="${CFLAGS} ${OPTIONAL_OS_DEP_CFLAGS}"
    LDFLAGS="${LDFLAGS} -L/usr/local/lib"
    ;;
darwin*)
    build_target=macos
    LDFLAGS="${LDFLAGS} -framework CoreFoundation -framework IOKit"
    # -----------------------------------------------------------------------------
    # Pull in OpenSSL properly if on macOS
    if brew --prefix > /dev/null 2>&1; then
      if brew --prefix --installed openssl > /dev/null 2>&1; then
        HOMEBREW_OPENSSL_PREFIX=$(brew --prefix --installed openssl)
      elif brew --prefix --installed openssl@3 > /dev/null 2>&1; then
        HOMEBREW_OPENSSL_PREFIX=$(brew --prefix --installed openssl@3)
      elif brew --prefix --installed openssl@1.1 > /dev/null 2>&1; then
        HOMEBREW_OPENSSL_PREFIX=$(brew --prefix --installed openssl@1.1)
      fi
      if test -n "${HOMEBREW_OPENSSL_PREFIX}"; then
        OPTIONAL_OS_DEP_CFLAGS="-I${HOMEBREW_OPENSSL_PREFIX}/include"
        CFLAGS="${CFLAGS} ${OPTIONAL_OS_DEP_CFLAGS}"
        LDFLAGS="${LDFLAGS} -L${HOMEBREW_OPENSSL_PREFIX}/lib"
      fi
      HOMEBREW_PREFIX=$(brew --prefix)
      OPTIONAL_OS_DEP_CFLAGS="${OPTIONAL_OS_DEP_CFLAGS} -I${HOMEBREW_PREFIX}/include"
      CFLAGS="${CFLAGS} -I${HOMEBREW_PREFIX}/include"
      LDFLAGS="${LDFLAGS} -L${HOMEBREW_PREFIX}/lib"
    fi
    ;;
*)
    build_target=linux
    ;;
esac

AM_CONDITIONAL([FREEBSD], [test "${build_target}" = "freebsd"])
AM_CONDITIONAL([MACOS], [test "${build_target}" = "macos"])
AM_CONDITIONAL([LINUX], [test "${build_target}" = "linux"])
AC_MSG_RESULT([Host OS: ${build_target}])

# -----------------------------------------------------------------------------
# backtrace

AC_SEARCH_LIBS([backtrace], [execinfo], [AC_DEFINE([HAVE_BACKTRACE], [1], [backtrace availability])])

# -----------------------------------------------------------------------------
# pthreads

ACX_PTHREAD(, [AC_MSG_ERROR([Cannot initialize pthread environment])])
LIBS="${PTHREAD_LIBS} ${LIBS}"
CFLAGS="${CFLAGS} ${PTHREAD_CFLAGS}"
CC="${PTHREAD_CC}"

AC_CHECK_LIB(
[pthread],
[pthread_getname_np],
[AC_DEFINE([HAVE_PTHREAD_GETNAME_NP], [1], [Is set if pthread_getname_np is available])]
)

# -----------------------------------------------------------------------------
# libdatachannel

AC_CHECK_LIB([datachannel], [rtcCreatePeerConnection],
             [LIBDATACHANNEL_FOUND=yes],
             [LIBDATACHANNEL_FOUND=no])

if test "x$LIBDATACHANNEL_FOUND" = "xyes"; then
    AC_DEFINE([HAVE_LIBDATACHANNEL], [1], [libdatachannel usability])
    OPTIONAL_DATACHANNEL_LIBS="-ldatachannel"
fi

# -----------------------------------------------------------------------------
# libm

AC_ARG_VAR([MATH_CFLAGS], [C compiler flags for math])
AC_ARG_VAR([MATH_LIBS], [linker flags for math])
if test -z "${MATH_LIBS}"; then
    AC_CHECK_LIB(
        [m],
        [sin],
        [MATH_LIBS="-lm"]
    )
fi
test "${with_math}" = "yes" -a -z "${MATH_LIBS}" && AC_MSG_ERROR([math required but not found])

AC_MSG_CHECKING([if libm should be used])
if test "${with_math}" != "no" -a ! -z "${MATH_LIBS}"; then
    with_math="yes"
    AC_DEFINE([STORAGE_WITH_MATH], [1], [math usability])
    OPTIONAL_MATH_CFLAGS="${MATH_CFLAGS}"
    OPTIONAL_MATH_LIBS="${MATH_LIBS}"
else
    with_math="no"
fi
AC_MSG_RESULT([${with_math}])

AC_CHECK_FUNCS([isfinite])
AC_CHECK_FUNCS([finite])

# -----------------------------------------------------------------------------
# libuv multi-platform support library with a focus on asynchronous I/O
# TODO: check version, uv_fs_scandir_next only available in version >= 1.0

AC_CHECK_LIB(
    [uv],
    [uv_fs_scandir_next],
    [UV_LIBS="-luv"]
)
test -z "${UV_LIBS}" && \
    AC_MSG_ERROR([libuv required but not found. Try installing 'libuv1-dev' or 'libuv-devel'.])
OPTIONAL_UV_CFLAGS="${UV_CFLAGS}"
OPTIONAL_UV_LIBS="${UV_LIBS}"


# -----------------------------------------------------------------------------
# lz4 Extremely Fast Compression algorithm

AC_CHECK_LIB(
    [lz4],
    [LZ4_initStream],
    [LZ4_LIBS_FAST="-llz4"]
)

AC_CHECK_LIB(
    [lz4],
    [LZ4_compress_default],
    [LZ4_LIBS="-llz4"]
)

# -----------------------------------------------------------------------------
# zlib

PKG_CHECK_MODULES(
    [ZLIB],
    [zlib],
    [have_zlib=yes],
    [have_zlib=no]
)
test "${have_zlib}" != "yes" && AC_MSG_ERROR([zlib required but not found. Try installing 'zlib1g-dev' or 'zlib-devel'.])

# -----------------------------------------------------------------------------
# libuuid

PKG_CHECK_MODULES(
    [UUID],
    [uuid],
    [have_uuid=yes],
    [AC_MSG_ERROR([libuuid required but not found. Try installing 'uuid-dev' or 'libuuid-devel'.])]
)
OPTIONAL_UUID_CFLAGS="${UUID_CFLAGS}"
OPTIONAL_UUID_LIBS="${UUID_LIBS}"

# -----------------------------------------------------------------------------
# OpenSSL Cryptography and SSL/TLS Toolkit

AC_CHECK_LIB(
    [crypto],
    [SHA256_Init],
    [SSL_LIBS="-lcrypto -lssl"]
)

AC_CHECK_LIB(
    [crypto],
    [X509_VERIFY_PARAM_set1_host],
    [ssl_host_validation="yes"],
    [ssl_host_validation="no"]
)

test -z "${SSL_LIBS}" || \
    AC_DEFINE([HAVE_CRYPTO], [1], [libcrypto availability])

if test "${ssl_host_validation}" = "no"; then
    AC_DEFINE([HAVE_X509_VERIFY_PARAM_set1_host], [0], [ssl host validation])
    AC_MSG_WARN([DISABLING SSL HOSTNAME VALIDATION BECAUSE IT IS NOT AVAILABLE ON THIS SYSTEM.])
else
    AC_DEFINE([HAVE_X509_VERIFY_PARAM_set1_host], [1], [ssl host validation])
fi

# -----------------------------------------------------------------------------
# JSON-C library

PKG_CHECK_MODULES([JSON],[json-c],AC_CHECK_LIB(
    [json-c],
    [json_object_get_type],
    [JSONC_LIBS="-ljson-c"]),AC_CHECK_LIB(
        [json],
        [json_object_get_type],
        [JSONC_LIBS="-ljson"])
    )

OPTIONAL_JSONC_LIBS="${JSONC_LIBS}"

# -----------------------------------------------------------------------------
# YAML library

AC_CHECK_LIB(
    [yaml],
    [yaml_parser_initialize],
    [YAML_LIBS="-lyaml"]
)

OPTIONAL_YAML_LIBS="${YAML_LIBS}"

# -----------------------------------------------------------------------------
# DB engine and HTTPS
test "${enable_dbengine}" = "yes" -a -z "${LZ4_LIBS}" && \
    AC_MSG_ERROR([liblz4 required but not found. Try installing 'liblz4-dev' or 'lz4-devel'.])

AC_C_BIGENDIAN([],
    [LIBJUDY_CFLAGS="-DJU_LITTLE_ENDIAN"],
    [AC_MSG_ERROR([Could not find out system endiannnes])])

AC_CHECK_SIZEOF(void *)
if test "$ac_cv_sizeof_void_p" = 8; then 
    AC_MSG_RESULT(Detected 64-bit Build Environment)
    	LIBJUDY_CFLAGS="$LIBJUDY_CFLAGS -DJU_64BIT"
else 
    AC_MSG_RESULT(Detected 32-bit Build Environment)
        LIBJUDY_CFLAGS="$LIBJUDY_CFLAGS -UJU_64BIT"
fi

AC_SUBST([LIBJUDY_CFLAGS])

JUDY_CFLAGS="-I \$(abs_top_srcdir)/libnetdata/libjudy/src"

test "${enable_https}" = "yes" -a -z "${SSL_LIBS}" && \
    AC_MSG_ERROR([OpenSSL required for HTTPS but not found. Try installing 'libssl-dev' or 'openssl-devel'.])

test "${enable_dbengine}" = "yes" -a -z "${SSL_LIBS}" && \
    AC_MSG_ERROR([OpenSSL required for DBENGINE but not found. Try installing 'libssl-dev' or 'openssl-devel'.])

AC_MSG_CHECKING([if netdata dbengine should be used])
if test "${enable_dbengine}" != "no" -a "${UV_LIBS}" -a "${LZ4_LIBS}" -a "${SSL_LIBS}"; then
    enable_dbengine="yes"
    AC_DEFINE([ENABLE_DBENGINE], [1], [netdata dbengine usability])
    OPTIONAL_LZ4_CFLAGS="${LZ4_CFLAGS}"
    OPTIONAL_LZ4_LIBS="${LZ4_LIBS}"
    OPTIONAL_SSL_CFLAGS="${SSL_CFLAGS}"
    OPTIONAL_SSL_LIBS="${SSL_LIBS}"
else
    enable_dbengine="no"
fi
AC_MSG_RESULT([${enable_dbengine}])
AM_CONDITIONAL([ENABLE_DBENGINE], [test "${enable_dbengine}" = "yes"])

AC_MSG_CHECKING([if netdata https should be used])
if test "${enable_https}" != "no" -a "${SSL_LIBS}"; then
    enable_https="yes"
    AC_DEFINE([ENABLE_HTTPS], [1], [netdata HTTPS usability])
    OPTIONAL_SSL_CFLAGS="${SSL_CFLAGS}"
    OPTIONAL_SSL_LIBS="${SSL_LIBS}"
else
    enable_https="no"
fi
AC_MSG_RESULT([${enable_https}])
AM_CONDITIONAL([ENABLE_HTTPS], [test "${enable_https}" = "yes"])

AC_MSG_CHECKING([if netdata compression should be used])
if test "${enable_compression}" != "no"; then
    if test "${LZ4_LIBS_FAST}"; then
        LIBS_BKP="${LIBS}"
        LIBS="${LZ4_LIBS_FAST}"
        AC_TRY_LINK(
            [ #include <lz4.h> ],
            [
                LZ4_stream_t* stream = LZ4_initStream(NULL, 0);
            ],
            [ enable_compression="yes"],
            [ enable_compression="no" ]
        )
        LIBS="${LIBS_BKP}"
        if test "${enable_compression}" == "yes"; then
            OPTIONAL_LZ4_LIBS="${LZ4_LIBS_FAST}"
            AC_DEFINE([ENABLE_COMPRESSION], [1], [netdata compression usability])
        fi
    else
        if test "${enable_compression}" == "yes"; then
            AC_MSG_ERROR([liblz4 with version >= 1.9.0 required to enable_compression. Try installing the required version of 'liblz4-dev' or 'liblz4-devel'.])
            enable_compression="no"
        fi
        enable_compression="no"
    fi
else
    enable_compression="no"
fi
AC_MSG_RESULT([${enable_compression}])
AM_CONDITIONAL([ENABLE_COMPRESSION], [test "${enable_compression}" = "yes"])

# -----------------------------------------------------------------------------
# JSON-C

if test "${enable_jsonc}" != "no" -a -z "${JSONC_LIBS}"; then
    # Try and detect manual static build presence (from netdata-installer.sh)
    AC_MSG_CHECKING([if statically built json-c is present])
    HAVE_libjson_c_a="no"
    if test -f "externaldeps/jsonc/libjson-c.a"; then
        LIBS_BKP="${LIBS}"
        LIBS="externaldeps/jsonc/libjson-c.a"
        AC_LINK_IFELSE([AC_LANG_SOURCE([[#include "externaldeps/jsonc/json-c/json.h"
                                         int main (int argc, char **argv) {
                                             struct json_object *jobj;
                                             char *str = "{ \"msg-type\": \"random\" }";
                                             jobj = json_tokener_parse(str);
                                             json_object_get_type(jobj);
                                         }]])],
                                        [HAVE_libjson_c_a="yes"],
                                        [HAVE_libjson_c_a="no"])
        LIBS="${LIBS_BKP}"
    fi

	if test "${HAVE_libjson_c_a}" = "yes"; then
		AC_DEFINE([LINK_STATIC_JSONC], [1], [static json-c should be used])
		JSONC_LIBS="static"
		OPTIONAL_JSONC_STATIC_CFLAGS="-I \$(abs_top_srcdir)/externaldeps/jsonc"
	fi
	AC_MSG_RESULT([${HAVE_libjson_c_a}])
fi
AM_CONDITIONAL([LINK_STATIC_JSONC], [test "${JSONC_LIBS}" = "static"])

test "${enable_jsonc}" = "yes" -a -z "${JSONC_LIBS}" && \
    AC_MSG_ERROR([JSON-C required but not found. Try installing 'libjson-c-dev' or 'json-c'.])

AC_MSG_CHECKING([if json-c should be used])
if test "${enable_jsonc}" != "no" -a "${JSONC_LIBS}"; then
	enable_jsonc="yes"
	AC_DEFINE([ENABLE_JSONC], [1], [netdata json-c usability])
else
	enable_jsonc="no"
fi
AC_MSG_RESULT([${enable_jsonc}])
AM_CONDITIONAL([ENABLE_JSONC], [test "${enable_jsonc}" = "yes"])

# -----------------------------------------------------------------------------
# YAML

if test -z "${YAML_LIBS}"; then
    # Try and detect manual static build presence (from netdata-installer.sh)
    AC_MSG_CHECKING([if statically built libyaml is present])
    HAVE_libyaml_a="no"
    if test -f "externaldeps/libyaml/libyaml.a"; then
        LIBS_BKP="${LIBS}"
        LIBS="externaldeps/libyaml/libyaml.a"
        AC_LINK_IFELSE([AC_LANG_SOURCE([[#include "externaldeps/libyaml/yaml.h"
                                         int main (int argc, char **argv) {
                                             yaml_parser_t parser;
                                             yaml_parser_initialize(&parser);
                                         }]])],
                                        [HAVE_libyaml_a="yes"],
                                        [HAVE_libyaml_a="no"])
        LIBS="${LIBS_BKP}"
    fi

	if test "${HAVE_libyaml_a}" = "yes"; then
		AC_DEFINE([LINK_STATIC_YAML], [1], [static yaml should be used])
		YAML_LIBS="static"
		OPTIONAL_YAML_STATIC_CFLAGS="-I \$(abs_top_srcdir)/externaldeps/libyaml"
	fi
	AC_MSG_RESULT([${HAVE_libyaml_a}])
fi
AM_CONDITIONAL([LINK_STATIC_YAML], [test "${YAML_LIBS}" = "static"])

test -z "${YAML_LIBS}" && \
    AC_MSG_ERROR([LIBYAML required but not found. Try installing 'libyaml-dev'.])

# -----------------------------------------------------------------------------
# compiler options

AC_ARG_VAR([SSE_CANDIDATE], [C compiler flags for SSE])
AS_CASE([$host_cpu],
    [i?86], [SSE_CANDIDATE="yes"]
)
AC_SUBST([SSE_CANDIDATE])
if test "${SSE_CANDIDATE}" = "yes" -a "${enable_x86_sse}" = "yes"; then
    opt="-msse2 -mfpmath=sse"
    AX_CHECK_COMPILE_FLAG(${opt}, [CFLAGS="${CFLAGS} ${opt}"], [])
fi

if test "${GCC}" = "yes"; then
    AC_DEFINE_UNQUOTED([likely(x)], [__builtin_expect(!!(x), 1)], [gcc branch optimization])
    AC_DEFINE_UNQUOTED([unlikely(x)], [__builtin_expect(!!(x), 0)], [gcc branch optimization])
else
    AC_DEFINE_UNQUOTED([likely(x)], [(x)], [gcc branch optimization])
    AC_DEFINE_UNQUOTED([unlikely(x)], [(x)], [gcc branch optimization])
fi

if test "${GCC}" = "yes"; then
    AC_DEFINE([__always_unused], [__attribute__((unused))], [gcc unused attribute])
    AC_DEFINE([__maybe_unused], [__attribute__((unused))], [gcc unused attribute])
else
    AC_DEFINE([__always_unused], [], [dummy unused attribute])
    AC_DEFINE([__maybe_unused], [], [dummy unused attribute])
fi

if test "${enable_pedantic}" = "yes"; then
    enable_strict="yes"
    CFLAGS="${CFLAGS} -pedantic -Wall -Wextra -Wno-long-long"
fi


# -----------------------------------------------------------------------------
# memory allocation library

AC_MSG_CHECKING([for memory allocator])
TS_CHECK_JEMALLOC
if test "$has_jemalloc" = "1"; then
    AC_DEFINE([ENABLE_JEMALLOC], [1], [compile and link with jemalloc])
    AC_MSG_RESULT([jemalloc])
else
    TS_CHECK_TCMALLOC
    if test "$has_tcmalloc" = "1"; then
        AC_DEFINE([ENABLE_TCMALLOC], [1], [compile and link with tcmalloc])
        AC_MSG_RESULT([tcmalloc])
    else
        AC_MSG_RESULT([system])
        AC_C_MALLOPT
        AC_C_MALLINFO
    fi
fi


# -----------------------------------------------------------------------------
# libcap

PKG_CHECK_MODULES(
    [LIBCAP],
    [libcap],
    [AC_CHECK_LIB([cap], [cap_get_proc, cap_set_proc],
        [AC_CHECK_HEADER(
            [sys/capability.h],
            [have_libcap=yes],
            [have_libcap=no]
        )],
        [have_libcap=no]
    )],
    [have_libcap=no]
)
test "${with_libcap}" = "yes" -a "${have_libcap}" != "yes" && AC_MSG_ERROR([libcap required but not found.])

AC_MSG_CHECKING([if libcap should be used])
if test "${with_libcap}" != "no" -a "${have_libcap}" = "yes"; then
    with_libcap="yes"
    AC_DEFINE([HAVE_CAPABILITY], [1], [libcap usability])
    OPTIONAL_LIBCAP_CFLAGS="${LIBCAP_CFLAGS}"
    OPTIONAL_LIBCAP_LIBS="${LIBCAP_LIBS}"
else
    with_libcap="no"
fi
AC_MSG_RESULT([${with_libcap}])
AM_CONDITIONAL([ENABLE_CAPABILITY], [test "${with_libcap}" = "yes"])

# -----------------------------------------------------------------------------
# HTTPD and h2o related

can_build_httpd="no"
if test "${enable_httpd}" != "no"; then
    can_build_httpd="yes"
    AC_MSG_CHECKING([can build HTTPD])
    if test -z "${UV_LIBS}"; then
        can_build_httpd="no"
    fi
    if test -n "${SSL_LIBS}"; then
        OPTIONAL_SSL_CFLAGS="${SSL_CFLAGS}"
        OPTIONAL_SSL_LIBS="${SSL_LIBS}"
    else
        can_build_httpd="no"
    fi
    if test "${with_zlib}" != "yes"; then
        can_build_httpd="no"
    fi
    AC_MSG_RESULT([${can_build_httpd}])

    if test "${can_build_httpd}" = "no" -a "${enable_httpd}" = "yes"; then
        AC_MSG_ERROR([HTTPD was requested but it cannot be built])
    fi

    if test "${can_build_httpd}" = "yes"; then
        AC_DEFINE([ENABLE_HTTPD], [1], [HTTPD (h2o based web server)])
        HTTPD_CFLAGS="-I\$(abs_top_srcdir)/httpd/h2o/include -I\$(abs_top_srcdir)/httpd/h2o/deps/picotls/include -I\$(abs_top_srcdir)/httpd/h2o/deps/quicly/include -DH2O_USE_LIBUV=0"
    fi
fi
AM_CONDITIONAL([ENABLE_HTTPD], [test "${can_build_httpd}" = "yes"])

# -----------------------------------------------------------------------------
# ACLK

bundled_proto_avail="no"
if test "${with_bundled_protobuf}" != "no"; then
    AC_MSG_CHECKING([is bundled protobuf available])
    if test -f "externaldeps/protobuf/src/protoc"; then
        bundled_proto_avail="yes"
    fi
    AC_MSG_RESULT([${bundled_proto_avail}])
    if test "${with_bundled_protobuf}" == "yes" -a "${bundled_proto_avail}" != "yes"; then
        AC_MSG_ERROR([Bundled protobuf requested using --with-bundled-protobuf but it cannot be used/found])
    fi
    if test "${with_bundled_protobuf}" == "detect" -a "${bundled_proto_avail}" == "yes"; then
        with_bundled_protobuf="yes"
    fi
fi

AM_CONDITIONAL([MQTT_WSS_DEBUG], [test "${aclk_ssl_debug}" = "yes"])
if test "${aclk_ssl_debug}" = "yes"; then
    AC_DEFINE([MQTT_WSS_DEBUG], [1], [ACLK SSL allow debugging])
fi

if test "${with_bundled_protobuf}" != "yes"; then
PKG_CHECK_MODULES(
    [PROTOBUF],
    [protobuf >= 3],
    [have_libprotobuf=yes],
    [have_libprotobuf=no]
)

AC_PATH_PROG([PROTOC], [protoc], [no])
AS_IF(
    [test x"${PROTOC}" == x"no"],
    [have_protoc=no],
    [have_protoc=yes]
)
else
    AC_MSG_NOTICE([using bundled protobuf])
    AC_DEFINE([BUNDLED_PROTOBUF], [1], [Using a bundled copy of protobuf])
    PROTOC="\$(abs_top_srcdir)/externaldeps/protobuf/src/protoc"
    PROTOBUF_CFLAGS="-I \$(abs_top_srcdir)/externaldeps/protobuf/src"
    PROTOBUF_LIBS="\$(abs_top_srcdir)/externaldeps/protobuf/src/.libs/libprotobuf.a"
    have_libprotobuf="yes"
    have_protoc="yes"
fi

AC_PATH_PROG([CXX_BINARY], [${CXX}], [no])
AS_IF(
    [test x"${CXX_BINARY}" == x"no"],
    [have_CXX_compiler=no],
    [have_CXX_compiler=yes]
)

if test "${have_libprotobuf}" == "yes" && test "${have_CXX_compiler}" == "yes"; then
    AC_DEFINE([HAVE_PROTOBUF], [1], [Protobuf is available])
fi

AC_MSG_CHECKING([if Cloud functionality should be enabled])
AC_MSG_RESULT([${enable_cloud}])

if test "$enable_cloud" != "no"; then
    AC_MSG_NOTICE([Checking if ACLK can be built])
    can_enable_ng="yes"
    AC_MSG_CHECKING([if git submodules present for ACLK])
    if test -f "mqtt_websockets/src/mqtt_wss_client.c"; then
        AC_MSG_RESULT([yes])
    else
        AC_MSG_RESULT([no])
        can_enable_ng="no"
    fi
    AC_MSG_CHECKING([if SSL available for ACLK])
    if test -n "${SSL_LIBS}"; then
        AC_MSG_RESULT([yes])
        OPTIONAL_SSL_CFLAGS="${SSL_CFLAGS}"
        OPTIONAL_SSL_LIBS="${SSL_LIBS}"
    else
        AC_MSG_RESULT([no])
    fi
    AC_MSG_CHECKING([if JSON-C available for ACLK])
    if test "$enable_jsonc" != "yes"; then
        AC_MSG_RESULT([no])
        can_enable_ng="no"
    else
        AC_MSG_RESULT([yes])
    fi

    AC_MSG_CHECKING([if protobuf available for ACLK New Cloud Protocol])
    if test "${have_libprotobuf}" != "yes"; then
        AC_MSG_RESULT([no])
        can_enable_ng="no"
    else
        AC_MSG_RESULT([yes])
    fi
    AC_MSG_CHECKING([if protoc available for ACLK New Cloud Protocol])
    if test "${have_protoc}" != "yes"; then
        AC_MSG_RESULT([no])
        can_enable_ng="no"
    else
        AC_MSG_RESULT([yes])
    fi
    AC_MSG_CHECKING([if C++ compiler available for ACLK New Cloud Protocol])
    if test "${have_CXX_compiler}" != "yes"; then
        AC_MSG_RESULT([no])
        can_enable_ng="no"
    else
        AC_MSG_RESULT([yes])
    fi

    AC_MSG_CHECKING([ACLK Next Generation can be built])
    AC_MSG_RESULT([${can_enable_ng}])
    if test "$can_enable_ng" = "no" -a "$enable_cloud" = "yes"; then
        AC_MSG_ERROR([You have requested --with-cloud but ACLK can't be built. See reasons in lines above])
    fi
    if test "$can_enable_ng" = "yes"; then
        enable_aclk="yes"
        AC_DEFINE([ENABLE_ACLK], [1], [netdata ACLK])
        OPTIONAL_ACLK_CFLAGS="-I \$(abs_top_srcdir)/mqtt_websockets/src/include -I \$(abs_top_srcdir)/mqtt_websockets/c-rbuf/include -I \$(abs_top_srcdir)/aclk/aclk-schemas"
        OPTIONAL_PROTOBUF_CFLAGS="${PROTOBUF_CFLAGS}"
        OPTIONAL_PROTOBUF_LIBS="${PROTOBUF_LIBS}"
    fi
fi

if test "$enable_cloud" = "yes" -a "$enable_aclk" != "yes"; then
    AC_MSG_ERROR([ACLK can't be built but --enable-cloud was requested])
fi

AC_SUBST([enable_cloud])
AC_SUBST([enable_aclk])
AM_CONDITIONAL([ENABLE_ACLK], [test "${enable_aclk}" = "yes"])

# -----------------------------------------------------------------------------
# apps.plugin

AC_MSG_CHECKING([if apps.plugin should be enabled])
if test "${build_target}" != "macos"; then
    AC_DEFINE([ENABLE_APPS_PLUGIN], [1], [apps.plugin])
    enable_plugin_apps="yes"
else
    enable_plugin_apps="no"
fi
AC_MSG_RESULT([${enable_plugin_apps}])
AM_CONDITIONAL([ENABLE_PLUGIN_APPS], [test "${enable_plugin_apps}" = "yes"])


# -----------------------------------------------------------------------------
# freeipmi.plugin - libipmimonitoring

PKG_CHECK_MODULES(
    [IPMIMONITORING],
    [libipmimonitoring],
    [AC_CHECK_LIB([ipmimonitoring], [
        ipmi_monitoring_sensor_readings_by_record_id,
        ipmi_monitoring_sensor_readings_by_sensor_type,
        ipmi_monitoring_sensor_read_sensor_number,
        ipmi_monitoring_sensor_read_sensor_name,
        ipmi_monitoring_sensor_read_sensor_state,
        ipmi_monitoring_sensor_read_sensor_units,
        ipmi_monitoring_sensor_iterator_next,
        ipmi_monitoring_ctx_sensor_config_file,
        ipmi_monitoring_ctx_sdr_cache_directory,
        ipmi_monitoring_ctx_errormsg,
        ipmi_monitoring_ctx_create
    ],
        [AC_CHECK_HEADER(
            [ipmi_monitoring.h],
            [AC_CHECK_HEADER(
                [ipmi_monitoring_bitmasks.h],
                [have_ipmimonitoring=yes],
                [have_ipmimonitoring=no]
            )],
            [have_ipmimonitoring=no]
        )],
        [have_ipmimonitoring=no]
    )],
    [have_ipmimonitoring=no]
)
test "${enable_plugin_freeipmi}" = "yes" -a "${have_ipmimonitoring}" != "yes" && \
    AC_MSG_ERROR([ipmimonitoring required but not found. Try installing 'libipmimonitoring-dev' or 'libipmimonitoring-devel'])

AC_MSG_CHECKING([if freeipmi.plugin should be enabled])
if test "${enable_plugin_freeipmi}" != "no" -a "${have_ipmimonitoring}" = "yes"; then
    enable_plugin_freeipmi="yes"
    AC_DEFINE([HAVE_FREEIPMI], [1], [ipmimonitoring usability])
    OPTIONAL_IPMIMONITORING_CFLAGS="${IPMIMONITORING_CFLAGS}"
    OPTIONAL_IPMIMONITORING_LIBS="${IPMIMONITORING_LIBS}"
else
    enable_plugin_freeipmi="no"
fi
AC_MSG_RESULT([${enable_plugin_freeipmi}])
AM_CONDITIONAL([ENABLE_PLUGIN_FREEIPMI], [test "${enable_plugin_freeipmi}" = "yes"])


# -----------------------------------------------------------------------------
# cups.plugin - libcups

# Only check most recently added method of cups
AC_CHECK_LIB([cups], [httpConnect2],
    [AC_CHECK_HEADER(
        [cups/cups.h],
        [have_cups=yes],
        [have_cups=no]
    )],
    [have_cups=no]
)

test "${enable_plugin_cups}" = "yes" -a "${have_cups}" != "yes" && \
    AC_MSG_ERROR([cups required but not found. Try installing 'cups'])

AC_ARG_WITH([cups-config],
    [AS_HELP_STRING([--with-cups-config=path], [Specify path to cups-config executable.])],
    [with_cups_config="$withval"],
    [with_cups_config=system]
    )

AS_IF([test "x$with_cups_config" != "xsystem"], [
	CUPSCONFIG=$with_cups_config
], [
	AC_PATH_TOOL(CUPSCONFIG, [cups-config])
	AS_IF([test -z "$CUPSCONFIG"], [
		have_cups=no
	])
])

AC_MSG_CHECKING([if cups.plugin should be enabled])
if test "${enable_plugin_cups}" != "no" -a "${have_cups}" = "yes"; then
    enable_plugin_cups="yes"
    AC_DEFINE([HAVE_CUPS], [1], [cups usability])

    CUPS_CFLAGS="${CUPS_CFLAGS} `$CUPSCONFIG --cflags`"
    CUPS_LIBS="${CUPS_LIBS} `$CUPSCONFIG --libs`"

    OPTIONAL_CUPS_CFLAGS="${CUPS_CFLAGS}"
    OPTIONAL_CUPS_LIBS="${CUPS_LIBS}"
else
    enable_plugin_cups="no"
fi
AC_MSG_RESULT([${enable_plugin_cups}])
AM_CONDITIONAL([ENABLE_PLUGIN_CUPS], [test "${enable_plugin_cups}" = "yes"])


# -----------------------------------------------------------------------------
# nfacct.plugin - libmnl, libnetfilter_acct

AC_CHECK_HEADER(
    [linux/netfilter/nfnetlink_conntrack.h],
    [AC_CHECK_DECL(
        [CTA_STATS_MAX],
        [have_nfnetlink_conntrack=yes],
        [have_nfnetlink_conntrack=no],
        [#include <linux/netfilter/nfnetlink_conntrack.h>]
    )],
    [have_nfnetlink_conntrack=no]
)

LIBS_BAK="${LIBS}"
LIBS="${LIBS} -lmnl"

PKG_CHECK_MODULES(
    [LIBMNL],
    [libmnl],
    [AC_CHECK_LIB(
        [mnl],
        [mnl_socket_open],
        [have_libmnl=yes],
        [have_libmnl=no]
    )],
    [have_libmnl=no]
)


LIBS="${LIBS}"

PKG_CHECK_MODULES(
    [NFACCT],
    [libnetfilter_acct],
    AC_SEARCH_LIBS(
        [nfacct_alloc],
        [netfilter_acct libnetfilter_acct],
        [have_libnetfilter_acct=yes],
        [have_libnetfilter_acct=no],
        [${LIBS} -L/libnetfilter-acct-static/lib/libnetfilter_acct.a]
    )],
    [have_libnetfilter_acct=no]
)

LIBS="${LIBS_BAK}"

test "${enable_plugin_nfacct}" = "yes" -a "${have_nfnetlink_conntrack}" != "yes" && \
    AC_MSG_ERROR([nfnetlink_conntrack.h required but not found or too old])

test "${enable_plugin_nfacct}" = "yes" -a "${have_libnetfilter_acct}" != "yes" && \
    AC_MSG_ERROR([netfilter_acct required but not found])

test "${enable_plugin_nfacct}" = "yes" -a "${have_libmnl}" != "yes" && \
    AC_MSG_ERROR([libmnl required but not found. Try installing 'libmnl-dev' or 'libmnl-devel'])

AC_MSG_CHECKING([if nfacct.plugin should be enabled])
if test "${enable_plugin_nfacct}" != "no" -a "${have_libnetfilter_acct}" = "yes" \
                                          -a "${have_libmnl}" = "yes" \
                                          -a "${have_nfnetlink_conntrack}" = "yes"; then
    enable_plugin_nfacct="yes"
    AC_DEFINE([HAVE_NFACCT], [1], [netfilter accounting usability])
    OPTIONAL_NFACCT_CFLAGS="${NFACCT_CFLAGS} ${LIBMNL_CFLAGS}"
    OPTIONAL_NFACCT_LIBS="${NFACCT_LIBS} ${LIBMNL_LIBS}"
else
    enable_plugin_nfacct="no"
fi
AC_MSG_RESULT([${enable_plugin_nfacct}])
AM_CONDITIONAL([ENABLE_PLUGIN_NFACCT], [test "${enable_plugin_nfacct}" = "yes"])


# -----------------------------------------------------------------------------
# xenstat.plugin - libxenstat

PKG_CHECK_MODULES(
    [YAJL],
    [yajl],
    [AC_CHECK_LIB(
        [yajl],
        [yajl_tree_get],
        [have_libyajl=yes],
        [have_libyajl=no]
    )],
    [have_libyajl=no]
)

AC_CHECK_LIB(
    [xenstat],
    [xenstat_init],
    [AC_CHECK_HEADER(
        [xenstat.h],
        [have_libxenstat=yes],
        [have_libxenstat=no]
    )],
    [have_libxenstat=no],
    [-lyajl]
)

PKG_CHECK_MODULES(
    [XENLIGHT],
    [xenlight],
    [AC_CHECK_LIB(
        [xenlight],
        [libxl_domain_info],
        [AC_CHECK_HEADER(
            [libxl.h],
            [have_libxenlight=yes],
            [have_libxenlight=no]
        )],
        [have_libxenlight=no]
    )],
    [have_libxenlight=no]
)

test "${enable_plugin_xenstat}" = "yes" -a "${have_libxenstat}" != "yes" && \
    AC_MSG_ERROR([libxenstat required but not found. try installing 'xen-dom0-libs-devel'])

test "${enable_plugin_xenstat}" = "yes" -a "${have_libxenlight}" != "yes" && \
    AC_MSG_ERROR([libxenlight required but not found. try installing 'xen-dom0-libs-devel'])

test "${enable_plugin_xenstat}" = "yes" -a "${have_libyajl}" != "yes" && \
    AC_MSG_ERROR([libyajl required but not found. Try installing 'yajl-devel'])

AC_MSG_CHECKING([if xenstat.plugin should be enabled])
if test "${enable_plugin_xenstat}" != "no" -a "${have_libxenstat}" = "yes" -a "${have_libxenlight}" = "yes" -a "${have_libyajl}" = "yes"; then
    enable_plugin_xenstat="yes"
    AC_DEFINE([HAVE_LIBXENSTAT], [1], [libxenstat usability])
    OPTIONAL_XENSTAT_CFLAGS="${XENLIGHT_CFLAGS} ${YAJL_CFLAGS}"
    OPTIONAL_XENSTAT_LIBS="-lxenstat ${XENLIGHT_LIBS} ${YAJL_LIBS}"
else
    enable_plugin_xenstat="no"
fi
AC_MSG_RESULT([${enable_plugin_xenstat}])
AM_CONDITIONAL([ENABLE_PLUGIN_XENSTAT], [test "${enable_plugin_xenstat}" = "yes"])

if test "${enable_plugin_xenstat}" == "yes"; then
    AC_MSG_CHECKING([for xenstat_vbd_error in -lxenstat])
    AC_TRY_LINK(
        [ #include <xenstat.h> ],
        [
            xenstat_vbd * vbd;
            int out = xenstat_vbd_error(vbd);
        ],
        [
            have_xenstat_vbd_error=yes
            AC_DEFINE([HAVE_XENSTAT_VBD_ERROR], [1], [xenstat_vbd_error usability])
        ],
        [ have_xenstat_vbd_error=no ]
    )
    AC_MSG_RESULT([${have_xenstat_vbd_error}])
fi

# -----------------------------------------------------------------------------
# perf.plugin

AC_CHECK_HEADER(
    [linux/perf_event.h],
    [AC_CHECK_DECL(
        [PERF_COUNT_HW_REF_CPU_CYCLES],
        [have_perf_event=yes],
        [have_perf_event=no],
        [#include <linux/perf_event.h>]
    )],
    [have_perf_event=no]
)

AC_MSG_CHECKING([if perf.plugin should be enabled])
if test "${build_target}" == "linux" -a "${have_perf_event}" = "yes"; then
    AC_DEFINE([ENABLE_PERF_PLUGIN], [1], [perf.plugin])
    enable_plugin_perf="yes"
else
    enable_plugin_perf="no"
fi
AC_MSG_RESULT([${enable_plugin_perf}])
AM_CONDITIONAL([ENABLE_PLUGIN_PERF], [test "${enable_plugin_perf}" = "yes"])

# -----------------------------------------------------------------------------
# gtest/gmock

AC_MSG_CHECKING([if gtest and gmock can be found])

PKG_CHECK_MODULES([GTEST], [gtest], [have_gtest=yes], [have_gtest=no])
PKG_CHECK_MODULES([GMOCK], [gmock], [have_gmock=yes], [have_gmock=no])

if test "${have_gtest}" = "yes" -a "${have_gmock}" = "yes"; then
    OPTIONAL_GTEST_CFLAGS="${GTEST_CFLAGS} ${GMOCK_CFLAGS}"
    OPTIONAL_GTEST_LIBS="${GTEST_LIBS} ${GMOCK_LIBS}"
    have_gtest="yes"
else
    have_gtest="no"
fi

# -----------------------------------------------------------------------------
# ml - anomaly detection

# Check if uuid is available. Fail if ML was explicitly requested.
if test "${enable_ml}" = "yes" -a "${have_uuid}" != "yes"; then
    AC_MSG_ERROR([You have explicitly requested --enable-ml functionality but libuuid can not be found.])
fi

# Check if submodules have not been fetched. Fail if ML was explicitly requested.
AC_MSG_CHECKING([if git submodules are present for machine learning functionality])
if test -f "ml/dlib/dlib/all/source.cpp"; then
    AC_MSG_RESULT([yes])
    have_ml_submodules="yes"
else
    AC_MSG_RESULT([no])
    have_ml_submodules="no"
fi

if test "${enable_ml}" = "yes" -a "${have_ml_submodules}" = "no"; then
    AC_MSG_ERROR([You have explicitly requested --enable-ml functionality but it cannot be built because the required git submodules are missing.])
fi

if test "${enable_ml}" = "yes" -a "${have_cxx11}" = "no"; then
    AC_MSG_ERROR([You have explicitly requested --enable-ml functionality but it cannot be built without a C++11 toolchain.])
fi

# Decide if we should build ML
if test "${enable_ml}" != "no" -a "${have_ml_submodules}" = "yes" -a "${have_cxx11}" = "yes" -a "${have_uuid}" = "yes"; then
    build_ml="yes"
else
    build_ml="no"
fi

AM_CONDITIONAL([ENABLE_ML], [test "${build_ml}" = "yes"])
if test "${build_ml}" = "yes"; then
    AC_DEFINE([ENABLE_ML], [1], [anomaly detection usability])
    OPTIONAL_ML_CFLAGS="-DDLIB_NO_GUI_SUPPORT -I \$(abs_top_srcdir)/ml/dlib"
    OPTIONAL_ML_LIBS=""
fi


# -----------------------------------------------------------------------------
# debugfs.plugin

if test "${build_target}" = "linux"; then
    AC_DEFINE([ENABLE_DEBUGFS_PLUGIN], [1], [debugfs.plugin])
    enable_plugin_debugfs="yes"
else
    enable_plugin_debugfs="no"
fi

AC_MSG_CHECKING([if debugfs.plugin should be enabled])
AC_MSG_RESULT([${enable_plugin_debugfs}])
AM_CONDITIONAL([ENABLE_PLUGIN_DEBUGFS], [test "${enable_plugin_debugfs}" = "yes"])


# -----------------------------------------------------------------------------
# ebpf.plugin

if test "${build_target}" = "linux" -a "${enable_ebpf}" != "no"; then
    PKG_CHECK_MODULES(
        [LIBELF],
        [libelf],
        [have_libelf=yes],
        [have_libelf=no]
    )

    AC_CHECK_TYPE(
        [struct bpf_prog_info],
        [have_bpf=yes],
        [have_bpf=no],
        [#include <linux/bpf.h>]
    )

    AC_CHECK_FILE(
        externaldeps/libbpf/libbpf.a,
        [have_libbpf=yes],
        [have_libbpf=no]
    )

    if test "${have_libelf}" = "yes" -a \
            "${have_bpf}" = "yes" -a \
            "${have_libbpf}" = "yes"; then
        OPTIONAL_BPF_CFLAGS="${LIBELF_CFLAGS} -I \$(abs_top_srcdir)/externaldeps/libbpf/include -I \$(abs_top_srcdir)/externaldeps/libbpf/include/uapi"
        OPTIONAL_BPF_LIBS="\$(abs_top_srcdir)/externaldeps/libbpf/libbpf.a ${LIBELF_LIBS}"
        AC_DEFINE([HAVE_LIBBPF], [1], [libbpf usability])
        enable_ebpf="yes"
    else
        enable_ebpf="no"
    fi
else
    enable_ebpf="no"
fi
AC_MSG_CHECKING([if ebpf.plugin should be enabled])
AC_MSG_RESULT([${enable_ebpf}])
AM_CONDITIONAL([ENABLE_PLUGIN_EBPF], [test "${enable_ebpf}" = "yes"])

# -----------------------------------------------------------------------------
# slabinfo.plugin

AC_MSG_CHECKING([if slabinfo.plugin should be enabled])
if test "${build_target}" == "linux"; then
    AC_DEFINE([ENABLE_SLABINFO], [1], [slabinfo plugin])
    enable_plugin_slabinfo="yes"
else
    enable_plugin_slabinfo="no"
fi
AC_MSG_RESULT([${enable_plugin_slabinfo}])
AM_CONDITIONAL([ENABLE_PLUGIN_SLABINFO], [test "${enable_plugin_slabinfo}" = "yes"])


# -----------------------------------------------------------------------------
# AWS Kinesis exporting connector - libaws-cpp-sdk-kinesis, libaws-cpp-sdk-core, libssl, libcrypto, libcurl

PKG_CHECK_MODULES(
    [LIBCRYPTO],
    [libcrypto],
    [AC_CHECK_LIB(
        [crypto],
        [CRYPTO_new_ex_data],
        [have_libcrypto=yes],
        [have_libcrypto=no]
    )],
    [have_libcrypto=no]
)

PKG_CHECK_MODULES(
    [LIBSSL],
    [libssl],
    [AC_CHECK_LIB(
        [ssl],
        [SSL_connect],
        [have_libssl=yes],
        [have_libssl=no]
    )],
    [have_libssl=no]
)

PKG_CHECK_MODULES(
    [LIBCURL],
    [libcurl],
    [AC_CHECK_LIB(
        [curl],
        [curl_easy_init],
        [have_libcurl=yes],
        [have_libcurl=no]
    )],
    [have_libcurl=no]
)

PKG_CHECK_MODULES(
    [AWS_CPP_SDK_CORE],
    [aws-cpp-sdk-core],
    [have_libaws_cpp_sdk_core=yes],
    [have_libaws_cpp_sdk_core=no]
)

PKG_CHECK_MODULES(
    [AWS_CPP_SDK_KINESIS],
    [aws-cpp-sdk-kinesis],
    [have_libaws_cpp_sdk_kinesis=yes],
    [have_libaws_cpp_sdk_kinesis=no]
)

test "${enable_exporting_kinesis}" = "yes" -a "${have_libaws_cpp_sdk_kinesis}" != "yes" && \
    AC_MSG_ERROR([libaws-cpp-sdk-kinesis required but not found. try installing AWS C++ SDK])

test "${enable_exporting_kinesis}" = "yes" -a "${have_libaws_cpp_sdk_core}" != "yes" && \
    AC_MSG_ERROR([libaws-cpp-sdk-core required but not found. try installing AWS C++ SDK])

test "${enable_exporting_kinesis}" = "yes" -a "${have_libcurl}" != "yes" && \
    AC_MSG_ERROR([libcurl required but not found])

test "${enable_exporting_kinesis}" = "yes" -a "${have_libssl}" != "yes" && \
    AC_MSG_ERROR([libssl required but not found])

test "${enable_exporting_kinesis}" = "yes" -a "${have_libcrypto}" != "yes" && \
    AC_MSG_ERROR([libcrypto required but not found])

AC_MSG_CHECKING([if kinesis exporting connector should be enabled])
if test "${enable_exporting_kinesis}" != "no" -a "${have_libaws_cpp_sdk_kinesis}" = "yes" \
                                            -a "${have_libaws_cpp_sdk_core}" = "yes" \
                                            -a "${have_libcurl}" = "yes" \
                                            -a "${have_libssl}" = "yes" \
                                            -a "${have_libcrypto}" = "yes"; then
    enable_exporting_kinesis="yes"
    AC_DEFINE([HAVE_KINESIS], [1], [libaws-cpp-sdk-kinesis usability])
    OPTIONAL_KINESIS_CFLAGS="${LIBCRYPTO_CFLAGS} ${LIBSSL_CFLAGS} ${LIBCURL_CFLAGS}"
    OPTIONAL_KINESIS_CXXFLAGS="${AWS_CPP_SDK_KINESIS_CFLAGS} ${AWS_CPP_SDK_CORE_CFLAGS}"
    OPTIONAL_KINESIS_LIBS="${AWS_CPP_SDK_KINESIS_LIBS} ${AWS_CPP_SDK_CORE_LIBS} \
                           ${LIBCRYPTO_LIBS} ${LIBSSL_LIBS} ${LIBCURL_LIBS}"
else
    enable_exporting_kinesis="no"
fi

AC_MSG_RESULT([${enable_exporting_kinesis}])
AM_CONDITIONAL([ENABLE_EXPORTING_KINESIS], [test "${enable_exporting_kinesis}" = "yes"])


# -----------------------------------------------------------------------------
# Pub/Sub exporting connector - googleapis

PKG_CHECK_MODULES(
    [GRPC],
    [grpc],
    [have_libgrpc=yes],
    [have_libgrpc=no]
)

if test "${enable_exporting_pubsub}" != "no"; then
    PKG_CHECK_MODULES(
        [PUBSUB],
        [google_cloud_cpp_pubsub_protos],
        [have_pubsub_protos=yes],
        [have_pubsub_protos=no]
    )
fi

AC_PATH_PROG([CXX_BINARY], [${CXX}], [no])
AS_IF(
    [test x"${CXX_BINARY}" == x"no"],
    [have_CXX_compiler=no],
    [have_CXX_compiler=yes]
)

test "${enable_pubsub}" = "yes" -a "${have_grpc}" != "yes" && \
    AC_MSG_ERROR([libgrpc required but not found. try installing grpc])

test "${enable_pubsub}" = "yes" -a "${have_pubsub_protos}" != "yes" && \
    AC_MSG_ERROR([libgoogleapis_cpp_pubsub_protos required but not found. try installing googleapis])

test "${enable_exporting_prometheus_remote_write}" = "yes" -a "${have_CXX_compiler}" != "yes" && \
    AC_MSG_ERROR([C++ compiler required but not found. try installing g++])

AC_MSG_CHECKING([if pubsub exporting connector should be enabled])
if test "${enable_exporting_pubsub}" != "no" -a "${have_pubsub_protos}" = "yes" -a "${have_CXX_compiler}" = "yes"; then
    enable_exporting_pubsub="yes"
    AC_DEFINE([ENABLE_EXPORTING_PUBSUB], [1], [Pub/Sub API usability])
    OPTIONAL_PUBSUB_CFLAGS="${GRPC_CFLAGS} ${PUBSUB_CFLAGS}"
    OPTIONAL_PUBSUB_LIBS="${GRPC_LIBS} ${PUBSUB_LIBS}"
else
    enable_pubsub="no"
fi

AC_MSG_RESULT([${enable_exporting_pubsub}])
AM_CONDITIONAL([ENABLE_EXPORTING_PUBSUB], [test "${enable_exporting_pubsub}" = "yes"])


# -----------------------------------------------------------------------------
# Prometheus remote write exporting connector - libprotobuf, libsnappy, protoc

AC_MSG_CHECKING([for snappy::RawCompress in -lsnappy])

    AC_LANG_SAVE
    AC_LANG_CPLUSPLUS
    save_LIBS="${LIBS}"
    LIBS="-lsnappy"
    save_CXXFLAGS="${CXXFLAGS}"
    CXXFLAGS="${CXXFLAGS} ${CPP_STD_FLAG}"

    AC_TRY_LINK(
        [
            #include <stdlib.h>
            #include <snappy.h>
        ],
        [
            const char *input = "test";
            size_t compressed_length;
            char *buffer = (char *)malloc(5 * sizeof(char));
            snappy::RawCompress(input, 4, buffer, &compressed_length);
            free(buffer);
        ],
        [
            have_libsnappy=yes
            SNAPPY_CFLAGS=""
            SNAPPY_LIBS="-lsnappy"
        ],
        [have_libsnappy=no]
    )

    LIBS="${save_LIBS}"
    CXXFLAGS="${save_CXXFLAGS}"
    AC_LANG_RESTORE

AC_MSG_RESULT([${have_libsnappy}])

test "${enable_exporting_prometheus_remote_write}" = "yes" -a "${have_libprotobuf}" != "yes" && \
    AC_MSG_ERROR([libprotobuf required but not found. try installing protobuf])

test "${enable_exporting_prometheus_remote_write}" = "yes" -a "${have_libsnappy}" != "yes" && \
    AC_MSG_ERROR([libsnappy required but not found. try installing snappy])

test "${enable_exporting_prometheus_remote_write}" = "yes" -a "${have_protoc}" != "yes" && \
    AC_MSG_ERROR([protoc compiler required but not found. try installing protobuf])

test "${enable_exporting_prometheus_remote_write}" = "yes" -a "${have_CXX_compiler}" != "yes" && \
    AC_MSG_ERROR([C++ compiler required but not found. try installing g++])

AC_MSG_CHECKING([if prometheus remote write exporting connector should be enabled])
if test "${enable_exporting_prometheus_remote_write}" != "no" -a "${have_libprotobuf}" = "yes" -a "${have_libsnappy}" = "yes" \
                                                           -a "${have_protoc}" = "yes" -a "${have_CXX_compiler}" = "yes"; then
    enable_exporting_prometheus_remote_write="yes"
    AC_DEFINE([ENABLE_PROMETHEUS_REMOTE_WRITE], [1], [Prometheus remote write API usability])
    OPTIONAL_PROMETHEUS_REMOTE_WRITE_CFLAGS="${SNAPPY_CFLAGS} -I \$(abs_top_srcdir)/exporting/prometheus/remote_write"
    OPTIONAL_PROMETHEUS_REMOTE_WRITE_LIBS="${SNAPPY_LIBS}"
    OPTIONAL_PROTOBUF_CFLAGS="${PROTOBUF_CFLAGS}"
    OPTIONAL_PROTOBUF_LIBS="${PROTOBUF_LIBS}"
else
    enable_exporting_prometheus_remote_write="no"
fi

AC_MSG_RESULT([${enable_exporting_prometheus_remote_write}])
AM_CONDITIONAL([ENABLE_EXPORTING_PROMETHEUS_REMOTE_WRITE], [test "${enable_exporting_prometheus_remote_write}" = "yes"])


# -----------------------------------------------------------------------------
# MongoDB exporting connector - libmongoc

PKG_CHECK_MODULES(
    [LIBMONGOC],
    [libmongoc-1.0 >= 1.7],
    [have_libmongoc=yes],
    [have_libmongoc=no]
)

test "${enable_exporting_mongodb}" = "yes" -a "${have_libmongoc}" != "yes" && \
    AC_MSG_ERROR([libmongoc required but not found. Try installing `mongoc`.])

AC_MSG_CHECKING([if mongodb exporting connector should be enabled])
if test "${enable_exporting_mongodb}" != "no" -a "${have_libmongoc}" = "yes"; then
    enable_exporting_mongodb="yes"
    AC_DEFINE([HAVE_MONGOC], [1], [libmongoc usability])
    OPTIONAL_MONGOC_CFLAGS="${LIBMONGOC_CFLAGS}"
    OPTIONAL_MONGOC_LIBS="${LIBMONGOC_LIBS}"
else
    enable_exporting_mongodb="no"
fi

AC_MSG_RESULT([${enable_exporting_mongodb}])
AM_CONDITIONAL([ENABLE_EXPORTING_MONGODB], [test "${enable_exporting_mongodb}" = "yes"])


# -----------------------------------------------------------------------------
# check for setns() - cgroup-network

AC_CHECK_FUNC([setns])
AC_MSG_CHECKING([if cgroup-network can be enabled])
if test "$ac_cv_func_setns" = "yes" ; then
    have_setns="yes"
    AC_DEFINE([HAVE_SETNS], [1], [Define 1 if you have setns() function])
else
    have_setns="no"
fi
AC_MSG_RESULT([${have_setns}])
AM_CONDITIONAL([ENABLE_PLUGIN_CGROUP_NETWORK], [test "${have_setns}" = "yes"])


# -----------------------------------------------------------------------------
# Link-Time-Optimization

if test "${enable_lto}" != "no"; then
    opt="-flto"
    AX_CHECK_COMPILE_FLAG(${opt}, [have_lto=yes], [have_lto=no])
fi
if test "${have_lto}" = "yes"; then
    oCFLAGS="${CFLAGS}"
    CFLAGS="${CFLAGS} -flto"
    ac_cv_c_lto_cross_compile="${enable_lto}"
    test "${ac_cv_c_lto_cross_compile}" != "yes" && ac_cv_c_lto_cross_compile="no"
    AC_C_LTO
    CFLAGS="${oCFLAGS}"
    test "${ac_cv_c_lto}" != "yes" && have_lto="no"
fi
test "${enable_lto}" = "yes" -a "${have_lto}" != "yes" && \
    AC_MSG_ERROR([LTO is required but is not available.])
AC_MSG_CHECKING([if LTO should be enabled])
if test "${enable_lto}" != "no" -a "${have_lto}" = "yes"; then
    enable_lto="yes"
    OPTIONAL_LTO_CFLAGS="-flto"
else
    enable_lto="no"
fi
AC_MSG_RESULT([${enable_lto}])


# -----------------------------------------------------------------------------

# Try to unconditionally link with -latomic. If the compiler can satisfy
# all the atomic ops with builtins then, the library will be left unused.
# Otherwise, some ops will be covered by the compiler's intrinsics and some
# will be picked up by the linker from -latomic. In the later case, if
# -latomic is not available there will be a build failure, which would
# have happened either way before this change.
AC_LANG_PUSH([C++])

AC_MSG_CHECKING(whether we can use -latomic)
OLD_LIBS="${LIBS}"
LIBS="-latomic"
AC_LINK_IFELSE([AC_LANG_SOURCE([[
  #include <atomic>
  #include <cstdint>
  std::atomic<std::int64_t> v;
  int main() {
    return v;
  }
]])], CAN_USE_LIBATOMIC=yes, CAN_USE_LIBATOMIC=no)
LIBS="${OLD_LIBS}"
AC_MSG_RESULT($CAN_USE_LIBATOMIC)

if test "x$CAN_USE_LIBATOMIC" = xyes; then
    OPTIONAL_ATOMIC_LIBS="-latomic"
fi
AC_SUBST([OPTIONAL_ATOMIC_LIBS])

AC_LANG_POP([C++])

# -----------------------------------------------------------------------------

AC_MSG_CHECKING(whether we can use dlsym)
OLD_LIBS="${LIBS}"
LIBS="-ldl"
AC_LINK_IFELSE([AC_LANG_SOURCE([[
  #include <dlfcn.h>
  static void *(*libc_malloc)(size_t);
  int main() {
    libc_malloc = dlsym(RTLD_NEXT, "malloc");
  }
]])], CAN_USE_DLSYM=yes, CAN_USE_DLSYM=no)
LIBS="${OLD_LIBS}"
AC_MSG_RESULT($CAN_USE_DLSYM)

if test "x$CAN_USE_DLSYM" = xyes; then
    AC_DEFINE([HAVE_DLSYM], [1], [dlsym usability])
    OPTIONAL_DL_LIBS="-ldl"
fi
AC_SUBST([OPTIONAL_DL_LIBS])

# -----------------------------------------------------------------------------


AC_DEFINE_UNQUOTED([NETDATA_USER], ["${with_user}"], [use this user to drop privileged])

varlibdir="${localstatedir}/lib/netdata"
registrydir="${localstatedir}/lib/netdata/registry"
cachedir="${localstatedir}/cache/netdata"
chartsdir="${libexecdir}/netdata/charts.d"
pythondir="${libexecdir}/netdata/python.d"
configdir="${sysconfdir}/netdata"
libconfigdir="${libdir}/netdata/conf.d"
logdir="${localstatedir}/log/netdata"
pluginsdir="${libexecdir}/netdata/plugins.d"
netdata_user="${with_user}"
libsysdir="${libdir}/netdata/system"

AC_SUBST([varlibdir])
AC_SUBST([registrydir])
AC_SUBST([cachedir])
AC_SUBST([chartsdir])
AC_SUBST([pythondir])
AC_SUBST([configdir])
AC_SUBST([libconfigdir])
AC_SUBST([logdir])
AC_SUBST([pluginsdir])
AC_SUBST([webdir])
AC_SUBST([netdata_user])
AC_SUBST([libsysdir])

CFLAGS="${originalCFLAGS} ${OPTIONAL_LTO_CFLAGS} ${OPTIONAL_PROTOBUF_CFLAGS} ${OPTIONAL_MATH_CFLAGS} ${OPTIONAL_NFACCT_CFLAGS} \
    ${ZLIB_CFLAGS} ${OPTIONAL_UUID_CFLAGS} \
    ${OPTIONAL_LIBCAP_CFLAGS} ${OPTIONAL_IPMIMONITORING_CFLAGS} ${OPTIONAL_CUPS_CFLAGS} ${OPTIONAL_XENSTAT_FLAGS} \
    ${OPTIONAL_KINESIS_CFLAGS} ${OPTIONAL_PUBSUB_CFLAGS} ${OPTIONAL_PROMETHEUS_REMOTE_WRITE_CFLAGS} \
    ${OPTIONAL_MONGOC_CFLAGS} ${LWS_CFLAGS} ${OPTIONAL_JSONC_STATIC_CFLAGS} ${OPTIONAL_YAML_STATIC_CFLAGS} ${OPTIONAL_BPF_CFLAGS} ${JUDY_CFLAGS} \
    ${OPTIONAL_ACLK_CFLAGS} ${OPTIONAL_ML_CFLAGS} ${OPTIONAL_OS_DEP_CFLAGS} ${HTTPD_CFLAGS}"

CXXFLAGS="${CFLAGS} ${OPTIONAL_KINESIS_CXXFLAGS} ${CPP_STD_FLAG}"

CPPFLAGS="\
	-DVARLIB_DIR=\"\\\"${varlibdir}\\\"\" \
	-DCACHE_DIR=\"\\\"${cachedir}\\\"\" \
	-DCONFIG_DIR=\"\\\"${configdir}\\\"\" \
	-DLIBCONFIG_DIR=\"\\\"${libconfigdir}\\\"\" \
	-DLOG_DIR=\"\\\"${logdir}\\\"\" \
	-DPLUGINS_DIR=\"\\\"${pluginsdir}\\\"\" \
	-DRUN_DIR=\"\\\"${localstatedir}/run/netdata\\\"\" \
	-DWEB_DIR=\"\\\"${webdir}\\\"\" \
"

AC_SUBST([OPTIONAL_MATH_CFLAGS])
AC_SUBST([OPTIONAL_MATH_LIBS])
AC_SUBST([OPTIONAL_DATACHANNEL_LIBS])
AC_SUBST([OPTIONAL_UV_LIBS])
AC_SUBST([OPTIONAL_LZ4_LIBS])
AC_SUBST([OPTIONAL_SSL_LIBS])
AC_SUBST([OPTIONAL_JSONC_LIBS])
AC_SUBST([OPTIONAL_YAML_LIBS])
AC_SUBST([OPTIONAL_NFACCT_CFLAGS])
AC_SUBST([OPTIONAL_NFACCT_LIBS])
AC_SUBST([ZLIB_CFLAGS])
AC_SUBST([ZLIB_LIBS])
AC_SUBST([OPTIONAL_UUID_CFLAGS])
AC_SUBST([OPTIONAL_UUID_LIBS])
AC_SUBST([OPTIONAL_BPF_CFLAGS])
AC_SUBST([OPTIONAL_BPF_LIBS])
AC_SUBST([OPTIONAL_MQTT_LIBS])
AC_SUBST([OPTIONAL_LIBCAP_CFLAGS])
AC_SUBST([OPTIONAL_LIBCAP_LIBS])
AC_SUBST([OPTIONAL_IPMIMONITORING_CFLAGS])
AC_SUBST([OPTIONAL_IPMIMONITORING_LIBS])
AC_SUBST([OPTIONAL_CUPS_CFLAGS])
AC_SUBST([OPTIONAL_CUPS_LIBS])
AC_SUBST([OPTIONAL_XENSTAT_CFLAGS])
AC_SUBST([OPTIONAL_XENSTAT_LIBS])
AC_SUBST([OPTIONAL_KINESIS_CFLAGS])
AC_SUBST([OPTIONAL_KINESIS_LIBS])
AC_SUBST([OPTIONAL_PUBSUB_CFLAGS])
AC_SUBST([OPTIONAL_PUBSUB_LIBS])
AC_SUBST([OPTIONAL_PROMETHEUS_REMOTE_WRITE_CFLAGS])
AC_SUBST([OPTIONAL_PROMETHEUS_REMOTE_WRITE_LIBS])
AC_SUBST([OPTIONAL_MONGOC_CFLAGS])
AC_SUBST([OPTIONAL_MONGOC_LIBS])
AC_SUBST([OPTIONAL_LWS_LIBS])
AC_SUBST([OPTIONAL_ACLK_CFLAGS])
AC_SUBST([OPTIONAL_PROTOBUF_CFLAGS])
AC_SUBST([OPTIONAL_PROTOBUF_LIBS])
AC_SUBST([OPTIONAL_GTEST_CFLAGS])
AC_SUBST([OPTIONAL_GTEST_LIBS])
AC_SUBST([OPTIONAL_ML_CFLAGS])
AC_SUBST([OPTIONAL_ML_LIBS])

# -----------------------------------------------------------------------------
# Check if cmocka is available - needed for unit testing

AC_ARG_ENABLE(
     [unit-tests],
     [AS_HELP_STRING([--disable-unit-tests],
                     [Disables building and running the unit tests suite])],
     [],
     [enable_unit_tests="yes"]
)


PKG_CHECK_MODULES(
    [CMOCKA],
    [cmocka],
    [have_cmocka="yes"],
    [AC_MSG_NOTICE([CMocka not found on the system. Unit tests disabled])]
)
AM_CONDITIONAL([ENABLE_UNITTESTS], [test "${enable_unit_tests}" = "yes" -a "${have_cmocka}" = "yes" ])
AC_SUBST([ENABLE_UNITTESTS])

TEST_CFLAGS="${CFLAGS} ${CMOCKA_CFLAGS}"
TEST_LIBS="${CMOCKA_LIBS}"

AC_SUBST([TEST_CFLAGS])
AC_SUBST([TEST_LIBS])

# -----------------------------------------------------------------------------
# save configure options for build info
AC_DEFINE_UNQUOTED(
    [CONFIGURE_COMMAND],
    ["$ac_configure_args"],
    [options passed to configure script]
)

AC_CONFIG_FILES([
    Makefile
    netdata.spec
    collectors/Makefile
    collectors/apps.plugin/Makefile
    collectors/cgroups.plugin/Makefile
    collectors/charts.d.plugin/Makefile
    collectors/debugfs.plugin/Makefile
    collectors/diskspace.plugin/Makefile
    collectors/timex.plugin/Makefile
    collectors/ioping.plugin/Makefile
    collectors/freebsd.plugin/Makefile
    collectors/freeipmi.plugin/Makefile
    collectors/cups.plugin/Makefile
    collectors/idlejitter.plugin/Makefile
    collectors/macos.plugin/Makefile
    collectors/nfacct.plugin/Makefile
    collectors/plugins.d/Makefile
    collectors/proc.plugin/Makefile
    collectors/python.d.plugin/Makefile
    collectors/slabinfo.plugin/Makefile
    collectors/statsd.plugin/Makefile
    collectors/ebpf.plugin/Makefile
    collectors/tc.plugin/Makefile
    collectors/xenstat.plugin/Makefile
    collectors/perf.plugin/Makefile
    daemon/Makefile
    database/Makefile
    database/contexts/Makefile
    database/engine/Makefile
    database/ram/Makefile
    database/sqlite/Makefile
    diagrams/Makefile
    exporting/Makefile
    exporting/graphite/Makefile
    exporting/json/Makefile
    exporting/opentsdb/Makefile
    exporting/prometheus/Makefile
    exporting/prometheus/remote_write/Makefile
    exporting/aws_kinesis/Makefile
    exporting/pubsub/Makefile
    exporting/mongodb/Makefile
    exporting/tests/Makefile
    health/Makefile
    health/notifications/Makefile
    libnetdata/Makefile
    libnetdata/tests/Makefile
    libnetdata/adaptive_resortable_list/Makefile
    libnetdata/aral/Makefile
    libnetdata/avl/Makefile
    libnetdata/buffer/Makefile
    libnetdata/clocks/Makefile
    libnetdata/completion/Makefile
    libnetdata/config/Makefile
    libnetdata/dictionary/Makefile
    libnetdata/ebpf/Makefile
    libnetdata/eval/Makefile
    libnetdata/july/Makefile
    libnetdata/locks/Makefile
    libnetdata/log/Makefile
    libnetdata/onewayalloc/Makefile
    libnetdata/parser/Makefile
    libnetdata/popen/Makefile
    libnetdata/procfile/Makefile
    libnetdata/simple_pattern/Makefile
    libnetdata/socket/Makefile
    libnetdata/statistical/Makefile
    libnetdata/string/Makefile
    libnetdata/storage_number/Makefile
    libnetdata/storage_number/tests/Makefile
    libnetdata/threads/Makefile
    libnetdata/url/Makefile
    libnetdata/json/Makefile
    libnetdata/health/Makefile
    libnetdata/worker_utilization/Makefile
    registry/Makefile
    streaming/Makefile
    system/Makefile
    tests/Makefile
    web/Makefile
    web/api/Makefile
    web/api/badges/Makefile
    web/api/exporters/Makefile
    web/api/exporters/shell/Makefile
    web/api/exporters/prometheus/Makefile
    web/api/formatters/Makefile
    web/api/formatters/csv/Makefile
    web/api/formatters/json/Makefile
    web/api/formatters/ssv/Makefile
    web/api/formatters/value/Makefile
    web/api/queries/Makefile
    web/api/queries/average/Makefile
    web/api/queries/countif/Makefile
    web/api/queries/des/Makefile
    web/api/queries/incremental_sum/Makefile
    web/api/queries/max/Makefile
    web/api/queries/median/Makefile
    web/api/queries/min/Makefile
    web/api/queries/percentile/Makefile
    web/api/queries/ses/Makefile
    web/api/queries/stddev/Makefile
    web/api/queries/sum/Makefile
    web/api/queries/trimmed_mean/Makefile
    web/api/health/Makefile
    web/gui/Makefile
    web/gui/dashboard/Makefile
    web/rtc/Makefile
    web/server/Makefile
    web/server/static/Makefile
    claim/Makefile
    spawn/Makefile
])

AC_OUTPUT

test "${with_math}" != "yes" && AC_MSG_WARN([You are building without math. math allows accurate calculations. It should be enabled.]) || :
