#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
#
AC_PREREQ(2.60)

define([VERSION_MAJOR], [1])
define([VERSION_MINOR], [4])
define([VERSION_FIX], [1])
define([VERSION_NUMBER], VERSION_MAJOR[.]VERSION_MINOR[.]VERSION_FIX)
define([VERSION_SUFFIX], [_master])

dnl Set to "1" for a first RPM release of a new version
PACKAGE_RPM_RELEASE="0.0.$(echo VERSION_SUFFIX | sed s/^_//)"

AC_INIT([netdata], VERSION_NUMBER[]VERSION_SUFFIX)

AM_MAINTAINER_MODE([disable])
if test x"$USE_MAINTAINER_MODE" = xyes; then
AC_MSG_NOTICE(***************** MAINTAINER MODE *****************)
PACKAGE_BUILT_DATE=$(date '+%d %b %Y')
fi

PACKAGE_RPM_VERSION="VERSION_NUMBER"
AC_SUBST([PACKAGE_RPM_VERSION])
AC_SUBST([PACKAGE_RPM_RELEASE])

AC_CONFIG_AUX_DIR([.])
AC_CONFIG_HEADERS([config.h])
AC_CONFIG_MACRO_DIR([m4])
AC_CONFIG_SRCDIR([src/main.c])
AM_INIT_AUTOMAKE
AC_CANONICAL_HOST
AC_PROG_CC
AC_PROG_INSTALL
PKG_PROG_PKG_CONFIG
AC_USE_SYSTEM_EXTENSIONS
AC_CHECK_FUNCS_ONCE(accept4)
AC_CHECK_TYPES([struct timespec, clockid_t], [], [], [[#include <time.h>]])
AC_SEARCH_LIBS([clock_gettime], [rt posix4])
AC_CHECK_FUNCS([clock_gettime])

# Check system type
case "$host_os" in
freebsd*)
	build_target=freebsd
	;;
*)
	;;
esac

AM_CONDITIONAL([FREEBSD], [test x$build_target = xfreebsd])

AC_ARG_ENABLE(
	[plugin-nfacct],
	[AS_HELP_STRING([--enable-plugin-nfacct], [enable nfacct plugin, requires root])],
	,
	[enable_plugin_nfacct="no"]
)
AC_ARG_ENABLE(
	[pedantic],
	[AS_HELP_STRING([--enable-pedantic], [enable pedantic compiler warnings])],
	,
	[enable_pedantic="no"]
)
AC_ARG_WITH(
	[webdir],
	[AS_HELP_STRING([--with-webdir], [location of webdir @<:@PKGDATADIR/web@:>@])],
	[webdir="${withval}"],
	[webdir="\$(pkgdatadir)/web"]
)
AC_ARG_WITH(
	[zlib],
	[AS_HELP_STRING([--with-zlib], [build with zlib])],
	,
	[with_zlib="yes"]
)
AC_ARG_WITH(
	[math],
	[AS_HELP_STRING([--with-math], [build with math])],
	,
	[with_math="yes"]
)
AC_ARG_WITH(
	[user],
	[AS_HELP_STRING([--with-user], [use this user to drop privilege])],
	,
	[with_user="nobody"]
)
AC_ARG_ENABLE(
	[x86-sse],
	[AS_HELP_STRING([--disable-x86-sse], [SSE/SS2 optimizations on x86 @<:@default enabled@:>@])],
	,
	[enable_x86_sse="yes"]
)

ACX_PTHREAD(, [AC_MSG_ERROR([Cannot initialize pthread environment])])
LIBS="${PTHREAD_LIBS} ${LIBS}"
CFLAGS="${CFLAGS} ${PTHREAD_CFLAGS}"
CC="${PTHREAD_CC}"

TS_CHECK_JEMALLOC
if test "$has_jemalloc" = "1"; then
	AC_DEFINE([ENABLE_JEMALLOC], [1], [compile and link with jemalloc])
else
	TS_CHECK_TCMALLOC
	if test "$has_tcmalloc" = "1"; then
		AC_DEFINE([ENABLE_TCMALLOC], [1], [compile and link with tcmalloc])
	else
		AC_C_MALLOPT
		AC_C_MALLINFO
	fi
fi

AC_TYPE_UINT8_T
AC_TYPE_UINT16_T
AC_TYPE_UINT32_T
AC_C_INLINE
AC_FUNC_STRERROR_R
AC_C__GENERIC
AC_C___ATOMIC
AC_CHECK_SIZEOF([void *])
AC_CANONICAL_HOST

AC_ARG_VAR([SSE_CANDIDATE], [C compiler flags for SSE])
AS_CASE([$host_cpu],
	[i?86], [SSE_CANDIDATE="yes"]
)
AC_SUBST([SSE_CANDIDATE])
if test "${SSE_CANDIDATE}" = "yes" -a "${enable_x86_sse}" = "yes"; then
	opt="-msse2 -mfpmath=sse"
	AX_CHECK_COMPILE_FLAG($opt, [CFLAGS="$CFLAGS $opt"], [])
fi

AC_ARG_VAR([MATH_CFLAGS], [C compiler flags for math])
AC_ARG_VAR([MATH_LIBS], [linker flags for math])
if test -z "${MATH_LIBS}"; then
	AC_CHECK_LIB(
		[m],
		[sin],
		[MATH_LIBS="-lm"]
	)
fi

PKG_CHECK_MODULES(
	[UUID],
	[uuid],
)
test -z "${UUID_LIBS}" && AC_MSG_ERROR([libuuid required but not found. Try installing 'uuid-dev' or 'libuuid-devel'.])
AC_DEFINE([NETDATA_WITH_UUID], [1], [uuid settings])
OPTIONAL_UUID_CLFAGS="${UUID_CFLAGS}"
OPTIONAL_UUID_LIBS="${UUID_LIBS}"

if test "${enable_plugin_nfacct}" = "yes"; then
	PKG_CHECK_MODULES(
		[NFACCT],
		[libnetfilter_acct],
	)
	PKG_CHECK_MODULES(
		[LIBMNL],
		[libmnl],
	)
	test -z "${NFACCT_LIBS}" && AC_MSG_ERROR([netfilter_acct required but not found])
	test -z "${LIBMNL_LIBS}" && AC_MSG_ERROR([libmnl required but not found. Try installing 'libmnl-dev' or 'libmnl-devel'])
	AC_DEFINE([INTERNAL_PLUGIN_NFACCT], [1], [nfacct plugin settings])
	OPTIONAL_NFACCT_CLFAGS="${NFACCT_CFLAGS} ${LIBMNL_CFLAGS}"
	OPTIONAL_NFACCT_LIBS="${NFACCT_LIBS} ${LIBMNL_LIBS}"
fi
if test "${with_zlib}" = "yes"; then
	PKG_CHECK_MODULES(
		[ZLIB],
		[zlib],
	)
	test -z "${ZLIB_LIBS}" && AC_MSG_ERROR([zlib required but not found. Try installing 'zlib1g-dev' or 'zlib-devel'.])
	AC_DEFINE([NETDATA_WITH_ZLIB], [1], [zlib settings])
	OPTIONAL_ZLIB_CLFAGS="${ZLIB_CFLAGS}"
	OPTIONAL_ZLIB_LIBS="${ZLIB_LIBS}"
fi
if test "${with_math}" = "yes"; then
	test -z "${MATH_LIBS}" && AC_MSG_ERROR([math required but not found])
	AC_DEFINE([STORAGE_WITH_MATH], [1], [math settings])
	OPTIONAL_MATH_CFLAGS="${MATH_CFLAGS}"
	OPTIONAL_MATH_LIBS="${MATH_LIBS}"
fi

if test "${GCC}" = "yes"; then
	AC_DEFINE_UNQUOTED([likely(x)], [__builtin_expect(!!(x), 1)], [gcc branch optimization])
	AC_DEFINE_UNQUOTED([unlikely(x)], [__builtin_expect(!!(x), 0)], [gcc branch optimization])
else
	AC_DEFINE_UNQUOTED([likely(x)], [(x)], [gcc branch optimization])
	AC_DEFINE_UNQUOTED([unlikely(x)], [(x)], [gcc branch optimization])
fi

if test "${enable_pedantic}" = "yes"; then
	enable_strict="yes"
	CFLAGS="${CFLAGS} -pedantic -Wall -Wextra -Wno-long-long"
fi

AC_DEFINE_UNQUOTED([NETDATA_USER], ["${with_user}"], [use this user to drop privileged])

AC_SUBST([varlibdir], ["\$(localstatedir)/lib/netdata"])
AC_SUBST([registrydir], ["\$(localstatedir)/lib/netdata/registry"])
AC_SUBST([cachedir], ["\$(localstatedir)/cache/netdata"])
AC_SUBST([chartsdir], ["\$(libexecdir)/netdata/charts.d"])
AC_SUBST([nodedir], ["\$(libexecdir)/netdata/node.d"])
AC_SUBST([pythondir], ["\$(libexecdir)/netdata/python.d"])
AC_SUBST([configdir], ["\$(sysconfdir)/netdata"])
AC_SUBST([logdir], ["\$(localstatedir)/log/netdata"])
AC_SUBST([pluginsdir], ["\$(libexecdir)/netdata/plugins.d"])
AC_SUBST([webdir])

AC_SUBST([OPTIONAL_MATH_CLFAGS])
AC_SUBST([OPTIONAL_MATH_LIBS])
AC_SUBST([OPTIONAL_NFACCT_CLFAGS])
AC_SUBST([OPTIONAL_NFACCT_LIBS])
AC_SUBST([OPTIONAL_ZLIB_CLFAGS])
AC_SUBST([OPTIONAL_ZLIB_LIBS])
AC_SUBST([OPTIONAL_UUID_CLFAGS])
AC_SUBST([OPTIONAL_UUID_LIBS])

AC_CONFIG_FILES([
	Makefile
	charts.d/Makefile
	conf.d/Makefile
	netdata.spec
	python.d/Makefile
	node.d/Makefile
	plugins.d/Makefile
	src/Makefile
	system/Makefile
	web/Makefile
	contrib/Makefile
])
AC_OUTPUT

test "${with_math}" != "yes" && AC_MSG_WARN([You are building without math. math allows accurate calculations. It should be enabled.]) || :
test "${with_zlib}" != "yes" && AC_MSG_WARN([You are building without zlib. zlib allows netdata to trasnfer a lot less data with web clients. It should be enabled.]) || :
