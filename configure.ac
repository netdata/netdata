#
# Copyright (C) 2015 Alon Bar-Lev <alon.barlev@gmail.com>
# SPDX-License-Identifier: GPL-3.0-or-later
#
AC_PREREQ(2.60)

# We do not use m4_esyscmd_s to support older autoconf.
define([VERSION_STRING], m4_esyscmd([git describe --always 2>/dev/null | tr -d '\n']))
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
AM_PROG_CC_C_O
AC_PROG_INSTALL
PKG_PROG_PKG_CONFIG
AC_USE_SYSTEM_EXTENSIONS


# -----------------------------------------------------------------------------
# configurable options

AC_ARG_ENABLE(
    [plugin-nfacct],
    [AS_HELP_STRING([--enable-plugin-nfacct], [enable nfacct plugin, requires running netdata as root @<:@default disabled@:>@])],
    ,
    [enable_plugin_nfacct="no"]
)
AC_ARG_ENABLE(
    [plugin-freeipmi],
    [AS_HELP_STRING([--enable-plugin-freeipmi], [enable freeipmi plugin @<:@default autodetect@:>@])],
    ,
    [enable_plugin_freeipmi="detect"]
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
    [zlib],
    [AS_HELP_STRING([--without-zlib], [build without zlib @<:@default enabled@:>@])],
    ,
    [with_zlib="yes"]
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
    [AS_HELP_STRING([--disable-lto], [Link Time Optimizations @<:@default autodetect@:>@])],
    ,
    [enable_lto="detect"]
)


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
AC_C___ATOMIC
# AC_C_STMT_EXPR
AC_CHECK_SIZEOF([void *])
AC_CANONICAL_HOST
AC_HEADER_MAJOR
AC_HEADER_RESOLV

AC_CHECK_HEADERS_ONCE([sys/prctl.h])
AC_CHECK_HEADERS_ONCE([sys/vfs.h])
AC_CHECK_HEADERS_ONCE([sys/statfs.h])
AC_CHECK_HEADERS_ONCE([sys/statvfs.h])
AC_CHECK_HEADERS_ONCE([sys/mount.h])

if test "${enable_accept4}" != "no"; then
    AC_CHECK_FUNCS_ONCE(accept4)
fi

# -----------------------------------------------------------------------------
# operating system detection

AC_MSG_CHECKING([operating system])
case "$host_os" in
freebsd*)
    build_target=freebsd
    build_target_id=2
    CFLAGS="${CFLAGS} -I/usr/local/include"
    ;;
darwin*)
    build_target=macos
    build_target_id=3
    LDFLAGS="${LDFLAGS} -framework CoreFoundation -framework IOKit"
    ;;
*)
    build_target=linux
    build_target_id=1
    ;;
esac

AM_CONDITIONAL([FREEBSD], [test "${build_target}" = "freebsd"])
AM_CONDITIONAL([MACOS], [test "${build_target}" = "macos"])
AM_CONDITIONAL([LINUX], [test "${build_target}" = "linux"])
AC_MSG_RESULT([${build_target} with id ${build_target_id}])


# -----------------------------------------------------------------------------
# pthreads

ACX_PTHREAD(, [AC_MSG_ERROR([Cannot initialize pthread environment])])
LIBS="${PTHREAD_LIBS} ${LIBS}"
CFLAGS="${CFLAGS} ${PTHREAD_CFLAGS}"
CC="${PTHREAD_CC}"


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


# -----------------------------------------------------------------------------
# zlib

PKG_CHECK_MODULES(
    [ZLIB],
    [zlib],
    [have_zlib=yes],
    [have_zlib=no]
)
test "${with_zlib}" = "yes" -a "${have_zlib}" != "yes" && AC_MSG_ERROR([zlib required but not found. Try installing 'zlib1g-dev' or 'zlib-devel'.])

AC_MSG_CHECKING([if zlib should be used])
if test "${with_zlib}" != "no" -a "${have_zlib}" = "yes"; then
    with_zlib="yes"
    AC_DEFINE([NETDATA_WITH_ZLIB], [1], [zlib usability])
    OPTIONAL_ZLIB_CLFAGS="${ZLIB_CFLAGS}"
    OPTIONAL_ZLIB_LIBS="${ZLIB_LIBS}"
else
    with_zlib="no"
fi
AC_MSG_RESULT([${with_zlib}])


# -----------------------------------------------------------------------------
# libuuid

PKG_CHECK_MODULES(
    [UUID],
    [uuid],
    [have_uuid=yes],
    [AC_MSG_ERROR([libuuid required but not found. Try installing 'uuid-dev' or 'libuuid-devel'.])]
)
AC_DEFINE([NETDATA_WITH_UUID], [1], [uuid usability])
OPTIONAL_UUID_CLFAGS="${UUID_CFLAGS}"
OPTIONAL_UUID_LIBS="${UUID_LIBS}"


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
    OPTIONAL_LIBCAP_CLFAGS="${LIBCAP_CFLAGS}"
    OPTIONAL_LIBCAP_LIBS="${LIBCAP_LIBS}"
else
    with_libcap="no"
fi
AC_MSG_RESULT([${with_libcap}])
AM_CONDITIONAL([ENABLE_CAPABILITY], [test "${with_libcap}" = "yes"])


# -----------------------------------------------------------------------------
# apps.plugin

AC_MSG_CHECKING([if apps.plugin should be enabled])
if test "${build_target}" != "macos"; then
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
    OPTIONAL_IPMIMONITORING_CLFAGS="${IPMIMONITORING_CFLAGS}"
    OPTIONAL_IPMIMONITORING_LIBS="${IPMIMONITORING_LIBS}"
else
    enable_plugin_freeipmi="no"
fi
AC_MSG_RESULT([${enable_plugin_freeipmi}])
AM_CONDITIONAL([ENABLE_PLUGIN_FREEIPMI], [test "${enable_plugin_freeipmi}" = "yes"])


# -----------------------------------------------------------------------------
# nfacct.plugin - libmnl, libnetfilter_acct

AC_CHECK_HEADERS_ONCE([linux/netfilter/nfnetlink_conntrack.h])

PKG_CHECK_MODULES(
    [NFACCT],
    [libnetfilter_acct],
    [have_libnetfilter_acct=yes],
    [have_libnetfilter_acct=no]
)

PKG_CHECK_MODULES(
    [LIBMNL],
    [libmnl],
    [have_libmnl=yes],
    [have_libmnl=no]
)

test "${enable_plugin_nfacct}" = "yes" -a "${have_libnetfilter_acct}" != "yes" && \
    AC_MSG_ERROR([netfilter_acct required but not found])

test "${enable_plugin_nfacct}" = "yes" -a "${have_libmnl}" != "yes" && \
    AC_MSG_ERROR([libmnl required but not found. Try installing 'libmnl-dev' or 'libmnl-devel'])

AC_MSG_CHECKING([if nfacct.plugin should be enabled])
if test "${enable_plugin_nfacct}" != "no" -a "${have_libnetfilter_acct}" = "yes" -a "${have_libmnl}" = "yes"; then
    enable_plugin_nfacct="yes"
    AC_DEFINE([HAVE_LIBMNL], [1], [libmnl usability])
    AC_DEFINE([HAVE_LIBNETFILTER_ACCT], [1], [libnetfilter_acct usability])
    AC_DEFINE([INTERNAL_PLUGIN_NFACCT], [1], [nfacct plugin usability])
    OPTIONAL_NFACCT_CLFAGS="${NFACCT_CFLAGS} ${LIBMNL_CFLAGS}"
    OPTIONAL_NFACCT_LIBS="${NFACCT_LIBS} ${LIBMNL_LIBS}"
else
    enable_plugin_nfacct="no"
fi
AC_MSG_RESULT([${enable_plugin_nfacct}])
AM_CONDITIONAL([ENABLE_PLUGIN_NFACCT], [test "${enable_plugin_nfacct}" = "yes"])


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
    CFLAGS="${CFLAGS} -flto ${OPTIONAL_MATH_CLFAGS} ${OPTIONAL_NFACCT_CLFAGS} ${OPTIONAL_ZLIB_CLFAGS} ${OPTIONAL_UUID_CLFAGS} ${OPTIONAL_LIBCAP_CFLAGS} ${OPTIONAL_IPMIMONITORING_CFLAGS}"
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
    CFLAGS="${CFLAGS} -flto"
else
    enable_lto="no"
fi
AC_MSG_RESULT([${enable_lto}])


# -----------------------------------------------------------------------------

AC_DEFINE_UNQUOTED([NETDATA_USER], ["${with_user}"], [use this user to drop privileged])

varlibdir="${localstatedir}/lib/netdata"
registrydir="${localstatedir}/lib/netdata/registry"
cachedir="${localstatedir}/cache/netdata"
chartsdir="${libexecdir}/netdata/charts.d"
nodedir="${libexecdir}/netdata/node.d"
pythondir="${libexecdir}/netdata/python.d"
configdir="${sysconfdir}/netdata"
libconfigdir="${libdir}/netdata/conf.d"
logdir="${localstatedir}/log/netdata"
pluginsdir="${libexecdir}/netdata/plugins.d"

AC_SUBST([build_target])
AC_SUBST([varlibdir])
AC_SUBST([registrydir])
AC_SUBST([cachedir])
AC_SUBST([chartsdir])
AC_SUBST([nodedir])
AC_SUBST([pythondir])
AC_SUBST([configdir])
AC_SUBST([libconfigdir])
AC_SUBST([logdir])
AC_SUBST([pluginsdir])
AC_SUBST([webdir])

CPPFLAGS="\
    -DTARGET_OS=${build_target_id} \
	-DVARLIB_DIR=\"\\\"${varlibdir}\\\"\" \
	-DCACHE_DIR=\"\\\"${cachedir}\\\"\" \
	-DCONFIG_DIR=\"\\\"${configdir}\\\"\" \
	-DLIBCONFIG_DIR=\"\\\"${libconfigdir}\\\"\" \
	-DLOG_DIR=\"\\\"${logdir}\\\"\" \
	-DPLUGINS_DIR=\"\\\"${pluginsdir}\\\"\" \
	-DRUN_DIR=\"\\\"${localstatedir}/run/netdata\\\"\" \
	-DWEB_DIR=\"\\\"${webdir}\\\"\" \
"

AC_SUBST([OPTIONAL_MATH_CLFAGS])
AC_SUBST([OPTIONAL_MATH_LIBS])
AC_SUBST([OPTIONAL_NFACCT_CLFAGS])
AC_SUBST([OPTIONAL_NFACCT_LIBS])
AC_SUBST([OPTIONAL_ZLIB_CLFAGS])
AC_SUBST([OPTIONAL_ZLIB_LIBS])
AC_SUBST([OPTIONAL_UUID_CLFAGS])
AC_SUBST([OPTIONAL_UUID_LIBS])
AC_SUBST([OPTIONAL_LIBCAP_CFLAGS])
AC_SUBST([OPTIONAL_LIBCAP_LIBS])
AC_SUBST([OPTIONAL_IPMIMONITORING_CFLAGS])
AC_SUBST([OPTIONAL_IPMIMONITORING_LIBS])

AC_CONFIG_FILES([
    Makefile
    netdata.spec
    backends/graphite/Makefile
    backends/json/Makefile
    backends/Makefile
    backends/opentsdb/Makefile
    backends/prometheus/Makefile
    collectors/Makefile
    collectors/apps.plugin/Makefile
    collectors/cgroups.plugin/Makefile
    collectors/charts.d.plugin/Makefile
    collectors/checks.plugin/Makefile
    collectors/diskspace.plugin/Makefile
    collectors/fping.plugin/Makefile
    collectors/freebsd.plugin/Makefile
    collectors/freeipmi.plugin/Makefile
    collectors/idlejitter.plugin/Makefile
    collectors/macos.plugin/Makefile
    collectors/nfacct.plugin/Makefile
    collectors/node.d.plugin/Makefile
    collectors/plugins.d/Makefile
    collectors/proc.plugin/Makefile
    collectors/python.d.plugin/Makefile
    collectors/statsd.plugin/Makefile
    collectors/tc.plugin/Makefile
    daemon/Makefile
    database/Makefile
    diagrams/Makefile
    health/Makefile
    health/notifications/Makefile
    libnetdata/Makefile
    libnetdata/adaptive_resortable_list/Makefile
    libnetdata/avl/Makefile
    libnetdata/buffer/Makefile
    libnetdata/clocks/Makefile
    libnetdata/config/Makefile
    libnetdata/dictionary/Makefile
    libnetdata/eval/Makefile
    libnetdata/locks/Makefile
    libnetdata/log/Makefile
    libnetdata/popen/Makefile
    libnetdata/procfile/Makefile
    libnetdata/simple_pattern/Makefile
    libnetdata/socket/Makefile
    libnetdata/statistical/Makefile
    libnetdata/storage_number/Makefile
    libnetdata/threads/Makefile
    libnetdata/url/Makefile
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
    web/api/queries/des/Makefile
    web/api/queries/incremental_sum/Makefile
    web/api/queries/max/Makefile
    web/api/queries/median/Makefile
    web/api/queries/min/Makefile
    web/api/queries/ses/Makefile
    web/api/queries/stddev/Makefile
    web/api/queries/sum/Makefile
    web/gui/Makefile
    web/server/Makefile
    web/server/single/Makefile
    web/server/multi/Makefile
    web/server/static/Makefile
])
AC_OUTPUT

test "${with_math}" != "yes" && AC_MSG_WARN([You are building without math. math allows accurate calculations. It should be enabled.]) || :
test "${with_zlib}" != "yes" && AC_MSG_WARN([You are building without zlib. zlib allows netdata to transfer a lot less data with web clients. It should be enabled.]) || :
