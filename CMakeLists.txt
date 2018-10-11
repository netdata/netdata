
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

IF(LINUX)
    pkg_check_modules(NFACCT libnetfilter_acct)
    # later we use:
    # ${NFACCT_LIBRARIES}
    # ${NFACCT_CFLAGS_OTHER}
    # ${NFACCT_INCLUDE_DIRS}
ENDIF(LINUX)

# -----------------------------------------------------------------------------
# netdata files

set(LIBNETDATA_FILES
        src/libnetdata/adaptive_resortable_list.c
        src/libnetdata/adaptive_resortable_list.h
        src/libnetdata/appconfig.c
        src/libnetdata/appconfig.h
        src/libnetdata/avl.c
        src/libnetdata/avl.h
        src/libnetdata/clocks.c
        src/libnetdata/clocks.h
        src/libnetdata/common.c
        src/libnetdata/dictionary.c
        src/libnetdata/dictionary.h
        src/libnetdata/eval.c
        src/libnetdata/eval.h
        src/libnetdata/inlined.h
        src/libnetdata/libnetdata.h
        src/libnetdata/locks.c
        src/libnetdata/locks.h
        src/libnetdata/log.c
        src/libnetdata/log.h
        src/libnetdata/os.c
        src/libnetdata/os.h
        src/libnetdata/popen.c
        src/libnetdata/popen.h
        src/libnetdata/procfile.c
        src/libnetdata/procfile.h
        src/libnetdata/simple_pattern.c
        src/libnetdata/simple_pattern.h
        src/libnetdata/socket.c
        src/libnetdata/socket.h
        src/libnetdata/statistical.c
        src/libnetdata/statistical.h
        src/libnetdata/storage_number.c
        src/libnetdata/storage_number.h
        src/libnetdata/threads.c
        src/libnetdata/threads.h
        src/libnetdata/web_buffer.c
        src/libnetdata/web_buffer.h
        src/libnetdata/url.c
        src/libnetdata/url.h
        )

add_library(libnetdata OBJECT ${LIBNETDATA_FILES})

set(APPS_PLUGIN_FILES
        src/plugins/apps.plugin/apps_plugin.c
        )

set(CHECKS_PLUGIN_FILES
        src/plugins/checks.plugin/plugin_checks.c
        src/plugins/checks.plugin/plugin_checks.h
        )

set(FREEBSD_PLUGIN_FILES
        src/plugins/freebsd.plugin/plugin_freebsd.c
        src/plugins/freebsd.plugin/plugin_freebsd.h
        src/plugins/freebsd.plugin/freebsd_sysctl.c
        src/plugins/freebsd.plugin/freebsd_getmntinfo.c
        src/plugins/freebsd.plugin/freebsd_getifaddrs.c
        src/plugins/freebsd.plugin/freebsd_devstat.c
        src/plugins/freebsd.plugin/freebsd_kstat_zfs.c
        src/plugins/freebsd.plugin/freebsd_ipfw.c
        src/plugins/linux-proc.plugin/zfs_common.c
        src/plugins/linux-proc.plugin/zfs_common.h
        )

set(HEALTH_PLUGIN_FILES
        src/health/health.c
        src/health/health.h
        src/health/health_config.c
        src/health/health_json.c
        src/health/health_log.c
        )

set(IDLEJITTER_PLUGIN_FILES
        src/plugins/idlejitter.plugin/plugin_idlejitter.c
        src/plugins/idlejitter.plugin/plugin_idlejitter.h
        )

set(CGROUPS_PLUGIN_FILES
        src/plugins/linux-cgroups.plugin/sys_fs_cgroup.c
        src/plugins/linux-cgroups.plugin/sys_fs_cgroup.h
        )

set(CGROUP_NETWORK_FILES
        src/plugins/linux-cgroups.plugin/cgroup-network.c
        )

set(DISKSPACE_PLUGIN_FILES
        src/plugins/linux-diskspace.plugin/plugin_diskspace.h
        src/plugins/linux-diskspace.plugin/plugin_diskspace.c
        )

set(FREEIPMI_PLUGIN_FILES
        src/plugins/linux-freeipmi.plugin/freeipmi_plugin.c
        )

set(NFACCT_PLUGIN_FILES
        src/plugins/linux-nfacct.plugin/plugin_nfacct.c
        src/plugins/linux-nfacct.plugin/plugin_nfacct.h
        )

set(PROC_PLUGIN_FILES
        src/plugins/linux-proc.plugin/ipc.c
        src/plugins/linux-proc.plugin/plugin_proc.c
        src/plugins/linux-proc.plugin/plugin_proc.h
        src/plugins/linux-proc.plugin/proc_diskstats.c
        src/plugins/linux-proc.plugin/proc_interrupts.c
        src/plugins/linux-proc.plugin/proc_softirqs.c
        src/plugins/linux-proc.plugin/proc_loadavg.c
        src/plugins/linux-proc.plugin/proc_meminfo.c
        src/plugins/linux-proc.plugin/proc_net_dev.c
        src/plugins/linux-proc.plugin/proc_net_ip_vs_stats.c
        src/plugins/linux-proc.plugin/proc_net_netstat.c
        src/plugins/linux-proc.plugin/proc_net_rpc_nfs.c
        src/plugins/linux-proc.plugin/proc_net_rpc_nfsd.c
        src/plugins/linux-proc.plugin/proc_net_snmp.c
        src/plugins/linux-proc.plugin/proc_net_snmp6.c
        src/plugins/linux-proc.plugin/proc_net_sctp_snmp.c
        src/plugins/linux-proc.plugin/proc_net_sockstat.c
        src/plugins/linux-proc.plugin/proc_net_sockstat6.c
        src/plugins/linux-proc.plugin/proc_net_softnet_stat.c
        src/plugins/linux-proc.plugin/proc_net_stat_conntrack.c
        src/plugins/linux-proc.plugin/proc_net_stat_synproxy.c
        src/plugins/linux-proc.plugin/proc_self_mountinfo.c
        src/plugins/linux-proc.plugin/proc_self_mountinfo.h
        src/plugins/linux-proc.plugin/zfs_common.c
        src/plugins/linux-proc.plugin/zfs_common.h
        src/plugins/linux-proc.plugin/proc_spl_kstat_zfs.c
        src/plugins/linux-proc.plugin/proc_stat.c
        src/plugins/linux-proc.plugin/proc_sys_kernel_random_entropy_avail.c
        src/plugins/linux-proc.plugin/proc_vmstat.c
        src/plugins/linux-proc.plugin/proc_uptime.c
        src/plugins/linux-proc.plugin/sys_kernel_mm_ksm.c
        src/plugins/linux-proc.plugin/sys_devices_system_edac_mc.c
        src/plugins/linux-proc.plugin/sys_devices_system_node.c
        src/plugins/linux-proc.plugin/sys_fs_btrfs.c
        )

set(TC_PLUGIN_FILES
        src/plugins/linux-tc.plugin/plugin_tc.c
        src/plugins/linux-tc.plugin/plugin_tc.h
        )

set(MACOS_PLUGIN_FILES
        src/plugins/macos.plugin/plugin_macos.c
        src/plugins/macos.plugin/plugin_macos.h
        src/plugins/macos.plugin/macos_sysctl.c
        src/plugins/macos.plugin/macos_mach_smi.c
        src/plugins/macos.plugin/macos_fw.c
        )

set(PLUGINSD_PLUGIN_FILES
        src/plugins/plugins.d.plugin/plugins_d.c
        src/plugins/plugins.d.plugin/plugins_d.h
        )

set(REGISTRY_PLUGIN_FILES
        src/registry/registry.c
        src/registry/registry.h
        src/registry/registry_db.c
        src/registry/registry_init.c
        src/registry/registry_internals.c
        src/registry/registry_internals.h
        src/registry/registry_log.c
        src/registry/registry_machine.c
        src/registry/registry_machine.h
        src/registry/registry_person.c
        src/registry/registry_person.h
        src/registry/registry_url.c
        src/registry/registry_url.h
        )

set(STATSD_PLUGIN_FILES
        src/plugins/statsd.plugin/statsd.c
        src/plugins/statsd.plugin/statsd.h
        )

set(RRD_PLUGIN_FILES
        src/database/rrdcalc.c
        src/database/rrdcalc.h
        src/database/rrdcalctemplate.c
        src/database/rrdcalctemplate.h
        src/database/rrddim.c
        src/database/rrddimvar.c
        src/database/rrddimvar.h
        src/database/rrdfamily.c
        src/database/rrdhost.c
        src/database/rrd.c
        src/database/rrd.h
        src/database/rrdset.c
        src/database/rrdsetvar.c
        src/database/rrdsetvar.h
        src/database/rrdvar.c
        src/database/rrdvar.h
        )

set(WEB_PLUGIN_FILES
        src/webserver/web_client.c
        src/webserver/web_client.h
        src/webserver/web_server.c
        src/webserver/web_server.h
        )

set(API_PLUGIN_FILES
        src/api/rrd2json.c
        src/api/rrd2json.h
        src/api/web_api_v1.c
        src/api/web_api_v1.h
        src/api/web_buffer_svg.c
        src/api/web_buffer_svg.h
        )

set(STREAMING_PLUGIN_FILES
        src/streaming/rrdpush.c
        src/streaming/rrdpush.h
        )

set(BACKENDS_PLUGIN_FILES
        src/backends/backends.c
        src/backends/backends.h
        src/backends/graphite/graphite.c
        src/backends/graphite/graphite.h
        src/backends/json/json.c
        src/backends/json/json.h
        src/backends/opentsdb/opentsdb.c
        src/backends/opentsdb/opentsdb.h
        src/backends/prometheus/backend_prometheus.c
        src/backends/prometheus/backend_prometheus.h
        )

set(NETDATA_FILES
        src/plugins/all.h
        src/common.c
        src/common.h
        src/daemon.c
        src/daemon.h
        src/global_statistics.c
        src/global_statistics.h
        src/main.c
        src/main.h
        src/signals.c
        src/signals.h
        src/unit_test.c
        src/unit_test.h
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
    IF(IPMI_LIBRARIES)
        SET(ENABLE_PLUGIN_FREEIPMI True)
    ELSE()
        SET(ENABLE_PLUGIN_FREEIPMI False)
    ENDIF()

ELSEIF(FREEBSD)
    add_executable(netdata config.h ${NETDATA_FILES} ${FREEBSD_PLUGIN_FILES})
    target_link_libraries (netdata libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(netdata PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_compile_options(netdata PUBLIC ${NETDATA_COMMON_CFLAGS})
    SET(ENABLE_PLUGIN_CGROUP_NETWORK False)
    SET(ENABLE_PLUGIN_APPS True)
    SET(ENABLE_PLUGIN_FREEIPMI False)

ELSEIF(MACOS)
    add_executable(netdata config.h ${NETDATA_FILES} ${MACOS_PLUGIN_FILES})
    target_link_libraries (netdata libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(netdata PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_compile_options(netdata PUBLIC ${NETDATA_COMMON_CFLAGS})
    SET(ENABLE_PLUGIN_CGROUP_NETWORK False)
    SET(ENABLE_PLUGIN_APPS False)
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
