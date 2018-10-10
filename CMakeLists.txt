
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

set(NETDATA_FREEBSD_FILES
        src/plugins/freebsd.plugin/plugin_freebsd.c
        src/plugins/freebsd.plugin/plugin_freebsd.h
        src/plugins/freebsd.plugin/freebsd_sysctl.c
        src/plugins/freebsd.plugin/freebsd_getmntinfo.c
        src/plugins/freebsd.plugin/freebsd_getifaddrs.c
        src/plugins/freebsd.plugin/freebsd_devstat.c
        src/plugins/freebsd.plugin/freebsd_kstat_zfs.c
        src/plugins/freebsd.plugin/freebsd_ipfw.c

        # the following are common with Linux
        src/plugins/linux-proc.plugin/zfs_common.c
        src/plugins/linux-proc.plugin/zfs_common.h
        )

set(NETDATA_MACOS_FILES
        src/plugins/macos.plugin/plugin_macos.c
        src/plugins/macos.plugin/plugin_macos.h
        src/plugins/macos.plugin/macos_sysctl.c
        src/plugins/macos.plugin/macos_mach_smi.c
        src/plugins/macos.plugin/macos_fw.c
        )

set(NETDATA_LINUX_FILES
        src/plugins/linux-cgroups.plugin/sys_fs_cgroup.c
        src/plugins/linux-cgroups.plugin/sys_fs_cgroup.h
        src/plugins/linux-diskspace.plugin/plugin_diskspace.c
        src/plugins/linux-diskspace.plugin/plugin_diskspace.h
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
        src/plugins/linux-proc.plugin/proc_spl_kstat_zfs.c
        src/plugins/linux-proc.plugin/proc_stat.c
        src/plugins/linux-proc.plugin/proc_sys_kernel_random_entropy_avail.c
        src/plugins/linux-proc.plugin/proc_vmstat.c
        src/plugins/linux-proc.plugin/proc_uptime.c
        src/plugins/linux-proc.plugin/sys_devices_system_edac_mc.c
        src/plugins/linux-proc.plugin/sys_devices_system_node.c
        src/plugins/linux-proc.plugin/sys_kernel_mm_ksm.c
        src/plugins/linux-proc.plugin/sys_fs_btrfs.c
        src/plugins/linux-proc.plugin/zfs_common.c
        src/plugins/linux-proc.plugin/zfs_common.h
        src/plugins/linux-tc.plugin/plugin_tc.c
        src/plugins/linux-tc.plugin/plugin_tc.h
        )

IF(LINUX AND MNL_LIBRARIES AND NFACCT_LIBRARIES)
    message(STATUS "nfacct.plugin: enabled (will work only if netdata runs as root)")
    set(NETDATA_NFACCT_FILES
            src/plugins/linux-nfacct.plugin/plugin_nfacct.c
            src/plugins/linux-nfacct.plugin/plugin_nfacct.h
            )
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -DINTERNAL_PLUGIN_NFACCT=1")
ELSE()
    message(STATUS "nfacct.plugin: disabled (requires libmnl and libnetfilter_acct)")
    set(NETDATA_NFACCT_FILES
            )
ENDIF()

set(NETDATA_COMMON_FILES
        src/libnetdata/adaptive_resortable_list.c
        src/libnetdata/adaptive_resortable_list.h
        src/libnetdata/appconfig.c
        src/libnetdata/appconfig.h
        src/libnetdata/avl.c
        src/libnetdata/avl.h
        src/backend_prometheus.c
        src/backend_prometheus.h
        src/backends.c
        src/backends.h
        src/libnetdata/clocks.c
        src/libnetdata/clocks.h
        src/libnetdata/common.c
        src/common.h
        src/daemon.c
        src/daemon.h
        src/libnetdata/dictionary.c
        src/libnetdata/dictionary.h
        src/libnetdata/eval.c
        src/libnetdata/eval.h
        src/global_statistics.c
        src/global_statistics.h
        src/libnetdata/inlined.h
        src/libnetdata/locks.c
        src/libnetdata/locks.h
        src/libnetdata/log.c
        src/libnetdata/log.h
        src/main.c
        src/main.h
        src/libnetdata/popen.c
        src/libnetdata/popen.h
        src/libnetdata/procfile.c
        src/libnetdata/procfile.h
        src/registry.c
        src/registry.h
        src/registry_db.c
        src/registry_init.c
        src/registry_internals.c
        src/registry_internals.h
        src/registry_log.c
        src/registry_machine.c
        src/registry_machine.h
        src/registry_person.c
        src/registry_person.h
        src/registry_url.c
        src/registry_url.h
        src/rrd.c
        src/rrd.h
        src/rrd2json.c
        src/rrd2json.h
        src/rrdcalc.c
        src/rrdcalc.h
        src/rrdcalctemplate.c
        src/rrdcalctemplate.h
        src/rrddim.c
        src/rrddimvar.c
        src/rrddimvar.h
        src/rrdfamily.c
        src/rrdhost.c
        src/rrdpush.c
        src/rrdpush.h
        src/rrdset.c
        src/rrdsetvar.c
        src/rrdsetvar.h
        src/rrdvar.c
        src/rrdvar.h
        src/signals.c
        src/signals.h
        src/libnetdata/simple_pattern.c
        src/libnetdata/simple_pattern.h
        src/libnetdata/socket.c
        src/libnetdata/socket.h
        src/libnetdata/statistical.c
        src/libnetdata/statistical.h
        src/plugins/checks.plugin/plugin_checks.c
        src/plugins/checks.plugin/plugin_checks.h
        src/plugins/health.plugin/health.c
        src/plugins/health.plugin/health.h
        src/plugins/health.plugin/health_config.c
        src/plugins/health.plugin/health_json.c
        src/plugins/health.plugin/health_log.c
        src/plugins/idlejitter.plugin/plugin_idlejitter.c
        src/plugins/idlejitter.plugin/plugin_idlejitter.h
        src/plugins/plugins.d.plugin/plugins_d.c
        src/plugins/plugins.d.plugin/plugins_d.h
        src/plugins/statsd.plugin/statsd.c
        src/plugins/statsd.plugin/statsd.h
        src/libnetdata/storage_number.c
        src/libnetdata/storage_number.h
        src/libnetdata/threads.c
        src/libnetdata/threads.h
        src/unit_test.c
        src/unit_test.h
        src/url.c
        src/url.h
        src/web_api_v1.c
        src/web_api_v1.h
        src/libnetdata/web_buffer.c
        src/libnetdata/web_buffer.h
        src/web_buffer_svg.c
        src/web_buffer_svg.h
        src/web_client.c
        src/web_client.h
        src/web_server.c
        src/web_server.h
        src/plugins/all.h
        config.h
        )

set(APPS_PLUGIN_SOURCE_FILES
        src/libnetdata/appconfig.c
        src/libnetdata/appconfig.h
        src/plugins/apps.plugin/apps_plugin.c
        src/libnetdata/avl.c
        src/libnetdata/avl.h
        src/libnetdata/adaptive_resortable_list.c
        src/libnetdata/adaptive_resortable_list.h
        src/libnetdata/common.c
        src/common.h
        src/libnetdata/clocks.c
        src/libnetdata/clocks.h
        src/libnetdata/inlined.h
        src/libnetdata/locks.c
        src/libnetdata/locks.h
        src/libnetdata/log.c
        src/libnetdata/log.h
        src/libnetdata/procfile.c
        src/libnetdata/procfile.h
        src/libnetdata/threads.c
        src/libnetdata/threads.h
        src/libnetdata/web_buffer.c
        src/libnetdata/web_buffer.h
        config.h
        )

set(FREEIPMI_PLUGIN_SOURCE_FILES
        src/plugins/linux-freeipmi.plugin/freeipmi_plugin.c
        src/libnetdata/common.c
        src/common.h
        src/libnetdata/clocks.c
        src/libnetdata/clocks.h
        src/libnetdata/inlined.h
        src/libnetdata/locks.c
        src/libnetdata/locks.h
        src/libnetdata/log.c
        src/libnetdata/log.h
        src/libnetdata/procfile.c
        src/libnetdata/procfile.h
        src/libnetdata/threads.c
        src/libnetdata/threads.h
        config.h
        )

set(CGROUP_NETWORK_SOURCE_FILES
        src/plugins/linux-cgroups.plugin/cgroup-network.c
        src/libnetdata/common.c
        src/common.h
        src/libnetdata/clocks.c
        src/libnetdata/clocks.h
        src/libnetdata/inlined.h
        src/libnetdata/locks.c
        src/libnetdata/locks.h
        src/libnetdata/log.c
        src/libnetdata/log.h
        src/libnetdata/popen.c
        src/libnetdata/popen.h
        src/libnetdata/procfile.c
        src/libnetdata/procfile.h
        src/signals.c
        src/signals.h
        src/libnetdata/storage_number.c
        src/libnetdata/storage_number.h
        src/libnetdata/threads.c
        src/libnetdata/threads.h
        src/libnetdata/web_buffer.c
        src/libnetdata/web_buffer.h
        config.h
        )

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
    add_executable(netdata ${NETDATA_COMMON_FILES} ${NETDATA_LINUX_FILES} ${NETDATA_NFACCT_FILES})
    target_link_libraries (netdata ${NETDATA_COMMON_LIBRARIES} ${MNL_LIBRARIES} ${NFACCT_LIBRARIES})
    target_include_directories(netdata PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS} ${MNL_INCLUDE_DIRS} ${NFACCT_INCLUDE_DIRS})
    target_compile_options(netdata PUBLIC ${NETDATA_COMMON_CFLAGS} ${MNL_CFLAGS_OTHER} ${NFACCT_CFLAGS_OTHER})
    SET(build_plugin_apps True)

ELSEIF(FREEBSD)
    add_executable(netdata ${NETDATA_COMMON_FILES} ${NETDATA_FREEBSD_FILES})
    target_link_libraries (netdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(netdata PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_compile_options(netdata PUBLIC ${NETDATA_COMMON_CFLAGS})
    SET(build_plugin_apps True)

ELSEIF(MACOS)
    add_executable(netdata ${NETDATA_COMMON_FILES} ${NETDATA_MACOS_FILES})
    target_link_libraries (netdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(netdata PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_compile_options(netdata PUBLIC ${NETDATA_COMMON_CFLAGS})
    SET(build_plugin_apps False)

ENDIF()


# -----------------------------------------------------------------------------
# apps.plugin

IF(build_plugin_apps)
    message(STATUS "apps.plugin: enabled")
    add_executable(apps.plugin ${APPS_PLUGIN_SOURCE_FILES} src/libnetdata/inlined.h)
    target_link_libraries (apps.plugin m ${CAP_LIBRARIES} ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(apps.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS} ${CAP_INCLUDE_DIRS})
    target_compile_options(apps.plugin PUBLIC ${NETDATA_COMMON_CFLAGS} ${CAP_CFLAGS_OTHER})
ELSE()
    message(STATUS "apps.plugin: disabled")
ENDIF()


# -----------------------------------------------------------------------------
# freeipmi.plugin

IF(LINUX AND IPMI_LIBRARIES)
    message(STATUS "freeipmi.plugin: enabled")
    add_executable(freeipmi.plugin ${FREEIPMI_PLUGIN_SOURCE_FILES})
    target_link_libraries (freeipmi.plugin ${IPMI_LIBRARIES} ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(apps.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS} ${IPMI_INCLUDE_DIRS})
    target_compile_options(apps.plugin PUBLIC ${NETDATA_COMMON_CFLAGS} ${IPMI_CFLAGS_OTHER})
ELSE()
    message(STATUS "freeipmi.plugin: disabled (depends on libipmimonitoring)")
ENDIF()


# -----------------------------------------------------------------------------
# cgroup-network

IF(LINUX)
    message(STATUS "cgroup-network: enabled")
    add_executable(cgroup-network ${CGROUP_NETWORK_SOURCE_FILES})
    target_link_libraries (cgroup-network ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(apps.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_compile_options(apps.plugin PUBLIC ${NETDATA_COMMON_CFLAGS})
ELSE()
    message(STATUS "cgroup-network: disabled (requires Linux)")
ENDIF()
