
# SPDX-License-Identifier: GPL-3.0-or-later
# This file is only used for development (netdata in Clion)
# It can build netdata, but you are on your own...

cmake_minimum_required(VERSION 3.0.2)
project(netdata C)

find_package(Threads REQUIRED)
find_package(PkgConfig REQUIRED)

# default is "Debug"
#set(CMAKE_BUILD_TYPE "Release")

# set this to see the compilation commands
#set(CMAKE_VERBOSE_MAKEFILE 1)


# -----------------------------------------------------------------------------
# Set compilation options according to build type

IF("${CMAKE_BUILD_TYPE}" MATCHES "Debug")
    message(STATUS "building for: debugging")

    ## unfortunately these produce errors
    #include(CheckCXXCompilerFlag)
    #CHECK_CXX_COMPILER_FLAG("-Wformat-signedness" CXX_FORMAT_SIGNEDNESS)
    #CHECK_CXX_COMPILER_FLAG("-Werror=format-security" CXX_FORMAT_SECURITY)
    #CHECK_CXX_COMPILER_FLAG("-fstack-protector-all" CXX_STACK_PROTECTOR)
    set(CXX_FORMAT_SIGNEDNESS "-Wformat-signedness")
    set(CXX_FORMAT_SECURITY "-Werror=format-security")
    set(CXX_STACK_PROTECTOR "-fstack-protector-all")

    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -O1 -ggdb -Wall -Wextra -DNETDATA_INTERNAL_CHECKS=1 -DNETDATA_VERIFY_LOCKS=1 ${CXX_FORMAT_SIGNEDNESS} ${CXX_FORMAT_SECURITY} ${CXX_STACK_PROTECTOR}")
ELSE()
    message(STATUS "building for: release")
    cmake_policy(SET CMP0069 "NEW")
    include(CheckIPOSupported)
    check_ipo_supported(RESULT ipo_supported OUTPUT error)
    IF(${ipo_supported})
        message(STATUS "link time optimization: supported")
        set(CMAKE_INTERPROCEDURAL_OPTIMIZATION TRUE)
    ELSE()
        message(STATUS "link time optimization: not supported")
    ENDIF()
ENDIF()


# -----------------------------------------------------------------------------
# O/S Detection

# these are defined in common.h too
SET(LINUX   False)
SET(FREEBSD False)
SET(MACOS   False)

# Detect the operating system
IF(${CMAKE_SYSTEM_NAME} MATCHES "Darwin")
    SET(TARGET_OS_NAME "macos")
    SET(TARGET_OS 3)
    SET(MACOS True)
ELSEIF(${CMAKE_SYSTEM_NAME} MATCHES "FreeBSD")
    SET(TARGET_OS_NAME "freebsd")
    SET(TARGET_OS 2)
    SET(FREEBSD True)
ELSE()
    SET(TARGET_OS_NAME "linux")
    SET(TARGET_OS 1)
    SET(LINUX True)
ENDIF()

# show the operating system on the console
message(STATUS "operating system: ${TARGET_OS_NAME} (TARGET_OS=${TARGET_OS})")


# -----------------------------------------------------------------------------
# Detect libuuid

pkg_check_modules(UUID REQUIRED uuid)
set(NETDATA_COMMON_CFLAGS ${NETDATA_COMMON_CFLAGS} ${UUID_CFLAGS_OTHER})
set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} ${UUID_LIBRARIES})
set(NETDATA_COMMON_INCLUDE_DIRS ${NETDATA_COMMON_INCLUDE_DIRS} ${UUID_INCLUDE_DIRS})

# -----------------------------------------------------------------------------
# Detect libz

pkg_check_modules(ZLIB REQUIRED zlib)
set(NETDATA_COMMON_CFLAGS ${NETDATA_COMMON_CFLAGS} ${ZLIB_CFLAGS_OTHER})
set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} ${ZLIB_LIBRARIES})
set(NETDATA_COMMON_INCLUDE_DIRS ${NETDATA_COMMON_INCLUDE_DIRS} ${ZLIB_INCLUDE_DIRS})


# -----------------------------------------------------------------------------
# Detect libcap

IF(LINUX)
    pkg_check_modules(CAP QUIET libcap)
    # later we use:
    # ${CAP_LIBRARIES}
    # ${CAP_CFLAGS_OTHER}
    # ${CAP_INCLUDE_DIRS}
ENDIF(LINUX)


# -----------------------------------------------------------------------------
# Detect libipmimonitoring

IF(LINUX)
    pkg_check_modules(IPMI libipmimonitoring)
    # later we use:
    # ${IPMI_LIBRARIES}
    # ${IPMI_CFLAGS_OTHER}
    # ${IPMI_INCLUDE_DIRS}
ENDIF(LINUX)


# -----------------------------------------------------------------------------
# Detect libmnl
IF(LINUX)
    pkg_check_modules(MNL libmnl)
    # later we use:
    # ${MNL_LIBRARIES}
    # ${MNL_CFLAGS_OTHER}
    # ${MNL_INCLUDE_DIRS}
ENDIF(LINUX)


# -----------------------------------------------------------------------------
# Detect libmnl

pkg_check_modules(NFACCT libnetfilter_acct)
# later we use:
# ${NFACCT_LIBRARIES}
# ${NFACCT_CFLAGS_OTHER}
# ${NFACCT_INCLUDE_DIRS}


# -----------------------------------------------------------------------------
# Detect MacOS IOKit/Foundation framework

IF(MACOS)
    find_library(IOKIT IOKit)
    find_library(FOUNDATION Foundation)
    # later we use:
    # ${FOUNDATION}
    # ${IOKIT}
ENDIF(MACOS)


# -----------------------------------------------------------------------------
# netdata files

set(LIBNETDATA_FILES
        libnetdata/adaptive_resortable_list/adaptive_resortable_list.c
        libnetdata/adaptive_resortable_list/adaptive_resortable_list.h
        libnetdata/config/appconfig.c
        libnetdata/config/appconfig.h
        libnetdata/avl/avl.c
        libnetdata/avl/avl.h
        libnetdata/buffer/buffer.c
        libnetdata/buffer/buffer.h
        libnetdata/clocks/clocks.c
        libnetdata/clocks/clocks.h
        libnetdata/dictionary/dictionary.c
        libnetdata/dictionary/dictionary.h
        libnetdata/eval/eval.c
        libnetdata/eval/eval.h
        libnetdata/inlined.h
        libnetdata/libnetdata.c
        libnetdata/libnetdata.h
        libnetdata/locks/locks.c
        libnetdata/locks/locks.h
        libnetdata/log/log.c
        libnetdata/log/log.h
        libnetdata/os.c
        libnetdata/os.h
        libnetdata/popen/popen.c
        libnetdata/popen/popen.h
        libnetdata/procfile/procfile.c
        libnetdata/procfile/procfile.h
        libnetdata/simple_pattern/simple_pattern.c
        libnetdata/simple_pattern/simple_pattern.h
        libnetdata/socket/socket.c
        libnetdata/socket/socket.h
        libnetdata/statistical/statistical.c
        libnetdata/statistical/statistical.h
        libnetdata/storage_number/storage_number.c
        libnetdata/storage_number/storage_number.h
        libnetdata/threads/threads.c
        libnetdata/threads/threads.h
        libnetdata/url/url.c
        libnetdata/url/url.h
        )

add_library(libnetdata OBJECT ${LIBNETDATA_FILES})

set(APPS_PLUGIN_FILES
        collectors/apps.plugin/apps_plugin.c
        )

set(CHECKS_PLUGIN_FILES
        collectors/checks.plugin/plugin_checks.c
        collectors/checks.plugin/plugin_checks.h
        )

set(FREEBSD_PLUGIN_FILES
        collectors/freebsd.plugin/plugin_freebsd.c
        collectors/freebsd.plugin/plugin_freebsd.h
        collectors/freebsd.plugin/freebsd_sysctl.c
        collectors/freebsd.plugin/freebsd_getmntinfo.c
        collectors/freebsd.plugin/freebsd_getifaddrs.c
        collectors/freebsd.plugin/freebsd_devstat.c
        collectors/freebsd.plugin/freebsd_kstat_zfs.c
        collectors/freebsd.plugin/freebsd_ipfw.c
        collectors/proc.plugin/zfs_common.c
        collectors/proc.plugin/zfs_common.h
        )

set(HEALTH_PLUGIN_FILES
        health/health.c
        health/health.h
        health/health_config.c
        health/health_json.c
        health/health_log.c
        )

set(IDLEJITTER_PLUGIN_FILES
        collectors/idlejitter.plugin/plugin_idlejitter.c
        collectors/idlejitter.plugin/plugin_idlejitter.h
        )

set(CGROUPS_PLUGIN_FILES
        collectors/cgroups.plugin/sys_fs_cgroup.c
        collectors/cgroups.plugin/sys_fs_cgroup.h
        )

set(CGROUP_NETWORK_FILES
        collectors/cgroups.plugin/cgroup-network.c
        )

set(DISKSPACE_PLUGIN_FILES
        collectors/diskspace.plugin/plugin_diskspace.h
        collectors/diskspace.plugin/plugin_diskspace.c
        )

set(FREEIPMI_PLUGIN_FILES
        collectors/freeipmi.plugin/freeipmi_plugin.c
        )

set(NFACCT_PLUGIN_FILES
        collectors/nfacct.plugin/plugin_nfacct.c
        collectors/nfacct.plugin/plugin_nfacct.h
        )

set(PROC_PLUGIN_FILES
        collectors/proc.plugin/ipc.c
        collectors/proc.plugin/plugin_proc.c
        collectors/proc.plugin/plugin_proc.h
        collectors/proc.plugin/proc_diskstats.c
        collectors/proc.plugin/proc_interrupts.c
        collectors/proc.plugin/proc_softirqs.c
        collectors/proc.plugin/proc_loadavg.c
        collectors/proc.plugin/proc_meminfo.c
        collectors/proc.plugin/proc_net_dev.c
        collectors/proc.plugin/proc_net_ip_vs_stats.c
        collectors/proc.plugin/proc_net_netstat.c
        collectors/proc.plugin/proc_net_rpc_nfs.c
        collectors/proc.plugin/proc_net_rpc_nfsd.c
        collectors/proc.plugin/proc_net_snmp.c
        collectors/proc.plugin/proc_net_snmp6.c
        collectors/proc.plugin/proc_net_sctp_snmp.c
        collectors/proc.plugin/proc_net_sockstat.c
        collectors/proc.plugin/proc_net_sockstat6.c
        collectors/proc.plugin/proc_net_softnet_stat.c
        collectors/proc.plugin/proc_net_stat_conntrack.c
        collectors/proc.plugin/proc_net_stat_synproxy.c
        collectors/proc.plugin/proc_self_mountinfo.c
        collectors/proc.plugin/proc_self_mountinfo.h
        collectors/proc.plugin/zfs_common.c
        collectors/proc.plugin/zfs_common.h
        collectors/proc.plugin/proc_spl_kstat_zfs.c
        collectors/proc.plugin/proc_stat.c
        collectors/proc.plugin/proc_sys_kernel_random_entropy_avail.c
        collectors/proc.plugin/proc_vmstat.c
        collectors/proc.plugin/proc_uptime.c
        collectors/proc.plugin/sys_kernel_mm_ksm.c
        collectors/proc.plugin/sys_devices_system_edac_mc.c
        collectors/proc.plugin/sys_devices_system_node.c
        collectors/proc.plugin/sys_fs_btrfs.c
        )

set(TC_PLUGIN_FILES
        collectors/tc.plugin/plugin_tc.c
        collectors/tc.plugin/plugin_tc.h
        )

set(MACOS_PLUGIN_FILES
        collectors/macos.plugin/plugin_macos.c
        collectors/macos.plugin/plugin_macos.h
        collectors/macos.plugin/macos_sysctl.c
        collectors/macos.plugin/macos_mach_smi.c
        collectors/macos.plugin/macos_fw.c
        )

set(PLUGINSD_PLUGIN_FILES
        collectors/plugins.d/plugins_d.c
        collectors/plugins.d/plugins_d.h
        )

set(REGISTRY_PLUGIN_FILES
        registry/registry.c
        registry/registry.h
        registry/registry_db.c
        registry/registry_init.c
        registry/registry_internals.c
        registry/registry_internals.h
        registry/registry_log.c
        registry/registry_machine.c
        registry/registry_machine.h
        registry/registry_person.c
        registry/registry_person.h
        registry/registry_url.c
        registry/registry_url.h
        )

set(STATSD_PLUGIN_FILES
        collectors/statsd.plugin/statsd.c
        collectors/statsd.plugin/statsd.h
        )

set(RRD_PLUGIN_FILES
        database/rrdcalc.c
        database/rrdcalc.h
        database/rrdcalctemplate.c
        database/rrdcalctemplate.h
        database/rrddim.c
        database/rrddimvar.c
        database/rrddimvar.h
        database/rrdfamily.c
        database/rrdhost.c
        database/rrd.c
        database/rrd.h
        database/rrdset.c
        database/rrdsetvar.c
        database/rrdsetvar.h
        database/rrdvar.c
        database/rrdvar.h
        )

set(WEB_PLUGIN_FILES
        web/server/web_client.c
        web/server/web_client.h
        web/server/web_server.c
        web/server/web_server.h
        web/server/single/single-threaded.c
        web/server/single/single-threaded.h
        web/server/multi/multi-threaded.c
        web/server/multi/multi-threaded.h
        web/server/static/static-threaded.c
        web/server/static/static-threaded.h
        web/server/web_client_cache.c
        web/server/web_client_cache.h
        )

set(API_PLUGIN_FILES
        web/api/rrd2json.c
        web/api/rrd2json.h
        web/api/web_api_v1.c
        web/api/web_api_v1.h
        web/api/badges/web_buffer_svg.c
        web/api/badges/web_buffer_svg.h
        web/api/exporters/allmetrics.c
        web/api/exporters/allmetrics.h
        web/api/exporters/shell/allmetrics_shell.c
        web/api/exporters/shell/allmetrics_shell.h
        web/api/queries/rrdr.c
        web/api/queries/rrdr.h
        web/api/queries/query.c
        web/api/queries/query.h
        web/api/queries/average/average.c
        web/api/queries/average/average.h
        web/api/queries/incremental_sum/incremental_sum.c
        web/api/queries/incremental_sum/incremental_sum.h
        web/api/queries/max/max.c
        web/api/queries/max/max.h
        web/api/queries/min/min.c
        web/api/queries/min/min.h
        web/api/queries/sum/sum.c
        web/api/queries/sum/sum.h
        web/api/queries/median/median.c
        web/api/queries/median/median.h
        web/api/queries/stddev/stddev.c
        web/api/queries/stddev/stddev.h
        web/api/queries/ses/ses.c
        web/api/queries/ses/ses.h
        )

set(STREAMING_PLUGIN_FILES
        streaming/rrdpush.c
        streaming/rrdpush.h
        )

set(BACKENDS_PLUGIN_FILES
        backends/backends.c
        backends/backends.h
        backends/graphite/graphite.c
        backends/graphite/graphite.h
        backends/json/json.c
        backends/json/json.h
        backends/opentsdb/opentsdb.c
        backends/opentsdb/opentsdb.h
        backends/prometheus/backend_prometheus.c
        backends/prometheus/backend_prometheus.h
        )

set(DAEMON_FILES
        daemon/common.c
        daemon/common.h
        daemon/daemon.c
        daemon/daemon.h
        daemon/global_statistics.c
        daemon/global_statistics.h
        daemon/main.c
        daemon/main.h
        daemon/signals.c
        daemon/signals.h
        daemon/unit_test.c
        daemon/unit_test.h
        )

set(NETDATA_FILES
        collectors/all.h
        ${DAEMON_FILES}
        ${API_PLUGIN_FILES}
        ${BACKENDS_PLUGIN_FILES}
        ${CHECKS_PLUGIN_FILES}
        ${HEALTH_PLUGIN_FILES}
        ${IDLEJITTER_PLUGIN_FILES}
        ${PLUGINSD_PLUGIN_FILES}
        ${RRD_PLUGIN_FILES}
        ${REGISTRY_PLUGIN_FILES}
        ${STATSD_PLUGIN_FILES}
        ${STREAMING_PLUGIN_FILES}
        ${WEB_PLUGIN_FILES}
        )

IF(LINUX AND MNL_LIBRARIES AND NFACCT_LIBRARIES)
    message(STATUS "nfacct.plugin: enabled (will work only if netdata runs as root)")
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -DINTERNAL_PLUGIN_NFACCT=1")
ELSE()
    message(STATUS "nfacct.plugin: disabled (requires libmnl and libnetfilter_acct)")
ENDIF()

include_directories(AFTER .)

add_definitions(
        -DHAVE_CONFIG_H
        -DTARGET_OS=${TARGET_OS}
        -DCACHE_DIR="/var/cache/netdata"
        -DCONFIG_DIR="/etc/netdata"
        -DLIBCONFIG_DIR="/usr/lib/netdata/conf.d"
        -DLOG_DIR="/var/log/netdata"
        -DPLUGINS_DIR="/usr/libexec/netdata"
        -DWEB_DIR="/usr/share/netdata"
        -DVARLIB_DIR="/var/lib/netdata"
)

# -----------------------------------------------------------------------------
# netdata

set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} m ${CMAKE_THREAD_LIBS_INIT})

IF(LINUX)
    add_executable(netdata config.h ${NETDATA_FILES}
            ${CGROUPS_PLUGIN_FILES}
            ${DISKSPACE_PLUGIN_FILES}
            ${NFACCT_PLUGIN_FILES}
            ${PROC_PLUGIN_FILES}
            ${TC_PLUGIN_FILES}
            )
    target_link_libraries (netdata libnetdata ${NETDATA_COMMON_LIBRARIES}
            ${MNL_LIBRARIES}
            ${NFACCT_LIBRARIES}
            )
    target_include_directories(netdata PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS}
            ${MNL_INCLUDE_DIRS}
            ${NFACCT_INCLUDE_DIRS}
            )
    target_compile_options(netdata PUBLIC ${NETDATA_COMMON_CFLAGS}
            ${MNL_CFLAGS_OTHER}
            ${NFACCT_CFLAGS_OTHER}
            )

    SET(ENABLE_PLUGIN_CGROUP_NETWORK True)
    SET(ENABLE_PLUGIN_APPS True)

ELSEIF(FREEBSD)
    add_executable(netdata config.h ${NETDATA_FILES} ${FREEBSD_PLUGIN_FILES})
    target_link_libraries (netdata libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(netdata PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_compile_options(netdata PUBLIC ${NETDATA_COMMON_CFLAGS})
    SET(ENABLE_PLUGIN_CGROUP_NETWORK False)
    SET(ENABLE_PLUGIN_APPS True)

ELSEIF(MACOS)
    add_executable(netdata config.h ${NETDATA_FILES} ${MACOS_PLUGIN_FILES})
    target_link_libraries (netdata libnetdata ${NETDATA_COMMON_LIBRARIES} ${IOKIT} ${FOUNDATION})
    target_include_directories(netdata PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_compile_options(netdata PUBLIC ${NETDATA_COMMON_CFLAGS})
    SET(ENABLE_PLUGIN_CGROUP_NETWORK False)
    SET(ENABLE_PLUGIN_APPS False)

ENDIF()

IF(IPMI_LIBRARIES)
    SET(ENABLE_PLUGIN_FREEIPMI True)
ELSE()
    SET(ENABLE_PLUGIN_FREEIPMI False)
ENDIF()


# -----------------------------------------------------------------------------
# apps.plugin

IF(ENABLE_PLUGIN_APPS)
    message(STATUS "apps.plugin: enabled")
    add_executable(apps.plugin config.h ${APPS_PLUGIN_FILES})
    target_link_libraries (apps.plugin libnetdata ${NETDATA_COMMON_LIBRARIES} ${CAP_LIBRARIES})
    target_include_directories(apps.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS} ${CAP_INCLUDE_DIRS})
    target_compile_options(apps.plugin PUBLIC ${NETDATA_COMMON_CFLAGS} ${CAP_CFLAGS_OTHER})
ELSE()
    message(STATUS "apps.plugin: disabled")
ENDIF()


# -----------------------------------------------------------------------------
# freeipmi.plugin

IF(ENABLE_PLUGIN_FREEIPMI)
    message(STATUS "freeipmi.plugin: enabled")
    add_executable(freeipmi.plugin config.h ${FREEIPMI_PLUGIN_FILES})
    target_link_libraries (freeipmi.plugin libnetdata ${NETDATA_COMMON_LIBRARIES} ${IPMI_LIBRARIES})
    target_include_directories(apps.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS} ${IPMI_INCLUDE_DIRS})
    target_compile_options(apps.plugin PUBLIC ${NETDATA_COMMON_CFLAGS} ${IPMI_CFLAGS_OTHER})
ELSE()
    message(STATUS "freeipmi.plugin: disabled (depends on libipmimonitoring)")
ENDIF()


# -----------------------------------------------------------------------------
# cgroup-network

IF(ENABLE_PLUGIN_CGROUP_NETWORK)
    message(STATUS "cgroup-network: enabled")
    add_executable(cgroup-network config.h ${CGROUP_NETWORK_FILES})
    target_link_libraries (cgroup-network libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(apps.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_compile_options(apps.plugin PUBLIC ${NETDATA_COMMON_CFLAGS})
ELSE()
    message(STATUS "cgroup-network: disabled (requires Linux)")
ENDIF()
