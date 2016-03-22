#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
#
AC_PREREQ(2.60)

define([VERSION_MAJOR], [1])
define([VERSION_MINOR], [0])
define([VERSION_FIX], [1])
define([VERSION_NUMBER], VERSION_MAJOR[.]VERSION_MINOR[.]VERSION_FIX)
define([VERSION_SUFFIX], [_master])

dnl Set to "1" for a first RPM release of a new version
PACKAGE_RPM_RELEASE="0.0.$(echo VERSION_SUFFIX | sed 's/^_//')"

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

ACX_PTHREAD(, [AC_MSG_ERROR([Cannot initialize pthread environment])])
LIBS="${PTHREAD_LIBS} ${LIBS}"
CFLAGS="${CFLAGS} ${PTHREAD_CFLAGS}"
CC="${PTHREAD_CC}"

AC_TYPE_UINT8_T
AC_TYPE_UINT16_T
AC_TYPE_UINT32_T
AC_C_INLINE

AC_ARG_VAR([MATH_CFLAGS], [C compiler flags for math])
AC_ARG_VAR([MATH_LIBS], [linker flags for math])
if test -z "${MATH_LIBS}"; then
	AC_CHECK_LIB(
		[m],
		[sin],
		[MATH_LIBS="-lm"]
	)
fi

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
	test -z "${LIBMNL_LIBS}" && AC_MSG_ERROR([libmnl required but not found])
	AC_DEFINE([INTERNAL_PLUGIN_NFACCT], [1], [nfacct plugin settings])
	OPTIONAL_NFACCT_CLFAGS="${NFACCT_CFLAGS} ${LIBMNL_CFLAGS}"
	OPTIONAL_NFACCT_LIBS="${NFACCT_LIBS} ${LIBMNL_LIBS}"
fi
if test "${with_zlib}" = "yes"; then
	PKG_CHECK_MODULES(
		[ZLIB],
		[zlib],
	)
	test -z "${ZLIB_LIBS}" && AC_MSG_ERROR([zlib required but not found])
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

AC_SUBST([cachedir], ["\$(localstatedir)/cache/netdata"])
AC_SUBST([chartsdir], ["\$(libexecdir)/netdata/charts.d"])
AC_SUBST([nodedir], ["\$(libexecdir)/netdata/node.d"])
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

AC_CONFIG_FILES([
	Makefile
	charts.d/Makefile
	node.d/Makefile
	conf.d/Makefile
	netdata.spec
	plugins.d/Makefile
	src/Makefile
	web/Makefile
])
AC_OUTPUT

test "${with_math}" != "yes" && AC_MSG_WARN([You are building without math. math allows accurate calculations. It should be enabled.]) || :
test "${with_zlib}" != "yes" && AC_MSG_WARN([You are building without zlib. zlib allows netdata to trasnfer a lot less data with web clients. It should be enabled.]) || :
