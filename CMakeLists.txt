
# SPDX-License-Identifier: GPL-3.0-or-later
# This file is only used for development (netdata in Clion)
# It can build netdata, but you are on your own...

cmake_minimum_required(VERSION 3.1.0)
project(netdata C CXX)

find_package(Threads REQUIRED)
find_package(PkgConfig REQUIRED)

# default is "Debug"
#set(CMAKE_BUILD_TYPE "Release")

# set this to see the compilation commands
# set(CMAKE_VERBOSE_MAKEFILE 1)


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
    set(CXX_FLAGS_DEBUG "-O0")
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -O1 -ggdb -Wall -Wextra -DNETDATA_INTERNAL_CHECKS=1 -DNETDATA_VERIFY_LOCKS=1 ${CXX_FORMAT_SIGNEDNESS} ${CXX_FORMAT_SECURITY} ${CXX_STACK_PROTECTOR} ${CXX_FLAGS_DEBUG}")
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
# libuv multi-platform support library with a focus on asynchronous I/O

pkg_check_modules(LIBUV REQUIRED libuv)
set(NETDATA_COMMON_CFLAGS ${NETDATA_COMMON_CFLAGS} ${LIBUV_CFLAGS_OTHER})
set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} ${LIBUV_LIBRARIES})
set(NETDATA_COMMON_INCLUDE_DIRS ${NETDATA_COMMON_INCLUDE_DIRS} ${LIBUV_INCLUDE_DIRS})

# -----------------------------------------------------------------------------
# lz4 Extremely Fast Compression algorithm

pkg_check_modules(LIBLZ4 REQUIRED liblz4)
set(NETDATA_COMMON_CFLAGS ${NETDATA_COMMON_CFLAGS} ${LIBLZ4_CFLAGS_OTHER})
set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} ${LIBLZ4_LIBRARIES})
set(NETDATA_COMMON_INCLUDE_DIRS ${NETDATA_COMMON_INCLUDE_DIRS} ${LIBLZ4_INCLUDE_DIRS})

# -----------------------------------------------------------------------------
# Judy General purpose dynamic array

# pkgconfig not working in Ubuntu, why? upstream package broken?
#pkg_check_modules(JUDY REQUIRED Judy)
#set(NETDATA_COMMON_CFLAGS ${NETDATA_COMMON_CFLAGS} ${JUDY_CFLAGS_OTHER})
#set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} ${JUDY_LIBRARIES})
#set(NETDATA_COMMON_INCLUDE_DIRS ${NETDATA_COMMON_INCLUDE_DIRS} ${JUDY_INCLUDE_DIRS})
set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} "-lJudy")
set(CMAKE_REQUIRED_LIBRARIES "Judy")
include(CheckSymbolExists)
check_symbol_exists("JudyLLast" "Judy.h" HAVE_JUDY)
IF(HAVE_JUDY)
    message(STATUS "Judy library found")
ELSE()
    message( FATAL_ERROR "libJudy required but not found. Try installing 'libjudy-dev' or 'Judy-devel'." )
ENDIF()

# -----------------------------------------------------------------------------
# OpenSSL Cryptography and SSL/TLS Toolkit

pkg_check_modules(OPENSSL REQUIRED openssl)
set(NETDATA_COMMON_CFLAGS ${NETDATA_COMMON_CFLAGS} ${OPENSSL_CFLAGS_OTHER})
set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} ${OPENSSL_LIBRARIES})
set(NETDATA_COMMON_INCLUDE_DIRS ${NETDATA_COMMON_INCLUDE_DIRS} ${OPENSSL_INCLUDE_DIRS})

# -----------------------------------------------------------------------------
# JSON-C used to health

pkg_check_modules(JSON REQUIRED json-c)
set(NETDATA_COMMON_CFLAGS ${NETDATA_COMMON_CFLAGS} ${JSONC_CFLAGS_OTHER})
set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} ${JSON_LIBRARIES})
set(NETDATA_COMMON_INCLUDE_DIRS ${NETDATA_COMMON_INCLUDE_DIRS} ${JSON_INCLUDE_DIRS})


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
# Detect libxenstat

pkg_check_modules(XENSTAT libxenstat)
# later we use:
# ${XENSTAT_LIBRARIES}
# ${XENSTAT_CFLAGS_OTHER}
# ${XENSTAT_INCLUDE_DIRS}


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
# Detect libcrypto

pkg_check_modules(CRYPTO libcrypto)
# later we use:
# ${CRYPTO_LIBRARIES}
# ${CRYPTO_CFLAGS_OTHER}
# ${CRYPTO_INCLUDE_DIRS}


# -----------------------------------------------------------------------------
# Detect libssl

pkg_check_modules(SSL libssl)
# later we use:
# ${SSL_LIBRARIES}
# ${SSL_CFLAGS_OTHER}
# ${SSL_INCLUDE_DIRS}


# -----------------------------------------------------------------------------
# Detect libcurl

pkg_check_modules(CURL libcurl)
# later we use:
# ${CURL_LIBRARIES}
# ${CURL_CFLAGS_OTHER}
# ${CURL_INCLUDE_DIRS}


# -----------------------------------------------------------------------------
# Detect libaws-cpp-sdk-core

find_library(HAVE_AWS aws-cpp-sdk-core)
# later we use:
# ${HAVE_AWS}

# -----------------------------------------------------------------------------
# Detect libaws-cpp-sdk-kinesis

find_library(HAVE_KINESIS aws-cpp-sdk-kinesis)
# later we use:
# ${HAVE_KINESIS}


# -----------------------------------------------------------------------------
# Detect libprotobuf

pkg_check_modules(PROTOBUF protobuf)
# later we use:
# ${PROTOBUF_LIBRARIES}
# ${PROTOBUF_CFLAGS_OTHER}
# ${PROTOBUF_INCLUDE_DIRS}


# -----------------------------------------------------------------------------
# Detect libsnappy

pkg_check_modules(SNAPPY snappy)
# later we use:
# ${SNAPPY_LIBRARIES}
# ${SNAPPY_CFLAGS_OTHER}
# ${SNAPPY_INCLUDE_DIRS}


# -----------------------------------------------------------------------------
# Detect libmongoc

find_package(libmongoc-1.0)
# later we use:
# ${MONGOC_LIBRARIES}
# ${MONGOC_INCLUDE_DIRS}


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
        libnetdata/json/json.c
        libnetdata/json/json.h
        libnetdata/json/jsmn.c
        libnetdata/json/jsmn.h
        libnetdata/health/health.c
        libnetdata/health/health.h
        libnetdata/string/utf8.h
        libnetdata/socket/security.c
        libnetdata/socket/security.h)

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
        health/health_log.c)

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
        )

set(XENSTAT_PLUGIN_FILES
        collectors/xenstat.plugin/xenstat_plugin.c
        )

set(PERF_PLUGIN_FILES
        collectors/perf.plugin/perf_plugin.c
        )

set(SLABINFO_PLUGIN_FILES
        collectors/slabinfo.plugin/slabinfo.c
        )

set(PROC_PLUGIN_FILES
        collectors/proc.plugin/ipc.c
        collectors/proc.plugin/plugin_proc.c
        collectors/proc.plugin/plugin_proc.h
        collectors/proc.plugin/proc_diskstats.c
        collectors/proc.plugin/proc_mdstat.c
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
        collectors/proc.plugin/sys_block_zram.c
        collectors/proc.plugin/sys_devices_system_edac_mc.c
        collectors/proc.plugin/sys_devices_system_node.c
        collectors/proc.plugin/sys_fs_btrfs.c
        collectors/proc.plugin/sys_class_power_supply.c
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
        database/engine/rrdengine.c
        database/engine/rrdengine.h
        database/engine/rrddiskprotocol.h
        database/engine/datafile.c
        database/engine/datafile.h
        database/engine/journalfile.c
        database/engine/journalfile.h
        database/engine/rrdenginelib.c
        database/engine/rrdenginelib.h
        database/engine/rrdengineapi.c
        database/engine/rrdengineapi.h
        database/engine/pagecache.c
        database/engine/pagecache.h
        database/engine/rrdenglocking.c
        database/engine/rrdenglocking.h
        )

set(WEB_PLUGIN_FILES
        web/server/web_client.c
        web/server/web_client.h
        web/server/web_server.c
        web/server/web_server.h
        web/server/static/static-threaded.c
        web/server/static/static-threaded.h
        web/server/web_client_cache.c
        web/server/web_client_cache.h
        )

set(API_PLUGIN_FILES
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
        web/api/queries/des/des.c
        web/api/queries/des/des.h
        web/api/formatters/rrd2json.c
        web/api/formatters/rrd2json.h
        web/api/formatters/csv/csv.c
        web/api/formatters/csv/csv.h
        web/api/formatters/json/json.c
        web/api/formatters/json/json.h
        web/api/formatters/ssv/ssv.c
        web/api/formatters/ssv/ssv.h
        web/api/formatters/value/value.c
        web/api/formatters/value/value.h
        web/api/formatters/json_wrapper.c
        web/api/formatters/json_wrapper.h
        web/api/formatters/charts2json.c
        web/api/formatters/charts2json.h
        web/api/formatters/rrdset2json.c
        web/api/formatters/rrdset2json.h
        web/api/health/health_cmdapi.c
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

set(KINESIS_BACKEND_FILES
        backends/aws_kinesis/aws_kinesis.c
        backends/aws_kinesis/aws_kinesis.h
        backends/aws_kinesis/aws_kinesis_put_record.cc
        backends/aws_kinesis/aws_kinesis_put_record.h
        )

set(PROMETHEUS_REMOTE_WRITE_BACKEND_FILES
        backends/prometheus/remote_write/remote_write.cc
        backends/prometheus/remote_write/remote_write.h
        )

set(MONGODB_BACKEND_FILES
        backends/mongodb/mongodb.c
        backends/mongodb/mongodb.h
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

include_directories(AFTER .)

add_definitions(
        -DHAVE_CONFIG_H
        -DTARGET_OS=${TARGET_OS}
        -DCACHE_DIR="/var/cache/netdata"
        -DCONFIG_DIR="/etc/netdata"
        -DLIBCONFIG_DIR="/usr/lib/netdata/conf.d"
        -DLOG_DIR="/var/log/netdata"
        -DPLUGINS_DIR="/usr/libexec/netdata/plugins.d"
        -DWEB_DIR="/usr/share/netdata/web"
        -DVARLIB_DIR="/var/lib/netdata"
)

# -----------------------------------------------------------------------------
# kinesis backend

IF(HAVE_KINESIS AND HAVE_AWS AND CRYPTO_LIBRARIES AND SSL_LIBRARIES AND CURL_LIBRARIES)
    SET(ENABLE_BACKEND_KINESIS True)
ELSE()
    SET(ENABLE_BACKEND_KINESIS False)
ENDIF()

IF(ENABLE_BACKEND_KINESIS)
    message(STATUS "kinesis backend: enabled")
    list(APPEND NETDATA_FILES ${KINESIS_BACKEND_FILES})
    list(APPEND NETDATA_COMMON_LIBRARIES aws-cpp-sdk-kinesis aws-cpp-sdk-core ${CRYPTO_LIBRARIES} ${SSL_LIBRARIES} ${CURL_LIBRARIES})
    list(APPEND NETDATA_COMMON_INCLUDE_DIRS ${CRYPTO_INCLUDE_DIRS} ${SSL_INCLUDE_DIRS} ${CURL_INCLUDE_DIRS})
    list(APPEND NETDATA_COMMON_CFLAGS ${CRYPTO_CFLAGS_OTHER} ${SSL_CFLAGS_OTHER} ${CURL_CFLAGS_OTHER})
ELSE()
    message(STATUS "kinesis backend: disabled (requires AWS SDK for C++)")
ENDIF()

# -----------------------------------------------------------------------------
# prometheus remote write backend

IF(PROTOBUF_LIBRARIES AND SNAPPY_LIBRARIES)
    SET(ENABLE_BACKEND_PROMETHEUS_REMOTE_WRITE True)
ELSE()
    SET(ENABLE_BACKEND_PROMETHEUS_REMOTE_WRITE False)
ENDIF()

IF(ENABLE_BACKEND_PROMETHEUS_REMOTE_WRITE)
    message(STATUS "prometheus remote write backend: enabled")

    find_package(Protobuf REQUIRED)
    protobuf_generate_cpp(PROTO_SRCS PROTO_HDRS backends/prometheus/remote_write/remote_write.proto)

    list(APPEND NETDATA_FILES ${PROMETHEUS_REMOTE_WRITE_BACKEND_FILES} ${PROTO_SRCS} ${PROTO_HDRS})
    list(APPEND NETDATA_COMMON_LIBRARIES ${PROTOBUF_LIBRARIES} ${SNAPPY_LIBRARIES})
    list(APPEND NETDATA_COMMON_INCLUDE_DIRS ${PROTOBUF_INCLUDE_DIRS} ${SNAPPY_INCLUDE_DIRS} ${CMAKE_CURRENT_BINARY_DIR})
    list(APPEND NETDATA_COMMON_CFLAGS ${PROTOBUF_CFLAGS_OTHER} ${SNAPPY_CFLAGS_OTHER})
ELSE()
    message(STATUS "prometheus remote write backend: disabled (requires protobuf and snappy libraries)")
ENDIF()

# -----------------------------------------------------------------------------
# mongodb backend

IF(libmongoc-1.0_FOUND)
    message(STATUS "mongodb backend: enabled")

    list(APPEND NETDATA_FILES ${MONGODB_BACKEND_FILES})
    list(APPEND NETDATA_COMMON_LIBRARIES ${MONGOC_LIBRARIES})
    list(APPEND NETDATA_COMMON_INCLUDE_DIRS ${MONGOC_INCLUDE_DIRS})
ELSE()
    message(STATUS "mongodb backend: disabled (requires mongoc library)")
ENDIF()

# -----------------------------------------------------------------------------
# netdata

set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} m ${CMAKE_THREAD_LIBS_INIT})

IF(LINUX)
    add_executable(netdata config.h ${NETDATA_FILES}
            ${CGROUPS_PLUGIN_FILES}
            ${DISKSPACE_PLUGIN_FILES}
            ${PROC_PLUGIN_FILES}
            ${TC_PLUGIN_FILES}
            )
    target_link_libraries (netdata libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(netdata PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_compile_options(netdata PUBLIC ${NETDATA_COMMON_CFLAGS})

    SET(ENABLE_PLUGIN_CGROUP_NETWORK True)
    SET(ENABLE_PLUGIN_APPS True)
    SET(ENABLE_PLUGIN_PERF True)
    SET(ENABLE_PLUGIN_SLABINFO True)

ELSEIF(FREEBSD)
    add_executable(netdata config.h ${NETDATA_FILES} ${FREEBSD_PLUGIN_FILES})
    target_link_libraries (netdata libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(netdata PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_compile_options(netdata PUBLIC ${NETDATA_COMMON_CFLAGS})
    SET(ENABLE_PLUGIN_CGROUP_NETWORK False)
    SET(ENABLE_PLUGIN_APPS True)
    SET(ENABLE_PLUGIN_PERF False)
    SET(ENABLE_PLUGIN_SLABINFO False)

ELSEIF(MACOS)
    add_executable(netdata config.h ${NETDATA_FILES} ${MACOS_PLUGIN_FILES})
    target_link_libraries (netdata libnetdata ${NETDATA_COMMON_LIBRARIES} ${IOKIT} ${FOUNDATION})
    target_include_directories(netdata PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_compile_options(netdata PUBLIC ${NETDATA_COMMON_CFLAGS})
    SET(ENABLE_PLUGIN_CGROUP_NETWORK False)
    SET(ENABLE_PLUGIN_APPS False)
    SET(ENABLE_PLUGIN_PERF False)
    SET(ENABLE_PLUGIN_SLABINFO False)

ENDIF()

IF(ENABLE_BACKEND_KINESIS OR ENABLE_BACKEND_PROMETHEUS_REMOTE_WRITE)
    set_property(TARGET netdata PROPERTY CXX_STANDARD 11)
    set_property(TARGET netdata PROPERTY CMAKE_CXX_STANDARD_REQUIRED ON)
ENDIF()

IF(IPMI_LIBRARIES)
    SET(ENABLE_PLUGIN_FREEIPMI True)
ELSE()
    SET(ENABLE_PLUGIN_FREEIPMI False)
ENDIF()

IF(LINUX AND MNL_LIBRARIES AND NFACCT_LIBRARIES)
    SET(ENABLE_PLUGIN_NFACCT True)
ELSE()
    SET(ENABLE_PLUGIN_NFACCT False)
ENDIF()

IF(XENSTAT_LIBRARIES)
    SET(ENABLE_PLUGIN_XENSTAT True)
ELSE()
    SET(ENABLE_PLUGIN_XENSTAT False)
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
    target_include_directories(freeipmi.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS} ${IPMI_INCLUDE_DIRS})
    target_compile_options(freeipmi.plugin PUBLIC ${NETDATA_COMMON_CFLAGS} ${IPMI_CFLAGS_OTHER})
ELSE()
    message(STATUS "freeipmi.plugin: disabled (depends on libipmimonitoring)")
ENDIF()


# -----------------------------------------------------------------------------
# nfacct.plugin

IF(ENABLE_PLUGIN_NFACCT)
    message(STATUS "nfacct.plugin: enabled")
    add_executable(nfacct.plugin config.h ${NFACCT_PLUGIN_FILES})
    target_link_libraries (nfacct.plugin libnetdata ${NETDATA_COMMON_LIBRARIES} ${MNL_LIBRARIES} ${NFACCT_LIBRARIES})
    target_include_directories(nfacct.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS} ${MNL_INCLUDE_DIRS} ${NFACCT_INCLUDE_DIRS})
    target_compile_options(nfacct.plugin PUBLIC ${NETDATA_COMMON_CFLAGS} ${MNL_CFLAGS_OTHER} ${NFACCT_CFLAGS_OTHER})
ELSE()
    message(STATUS "nfacct.plugin: disabled (requires libmnl and libnetfilter_acct)")
ENDIF()


# -----------------------------------------------------------------------------
# xenstat.plugin

IF(ENABLE_PLUGIN_XENSTAT)
    message(STATUS "xenstat.plugin: enabled")
    add_executable(xenstat.plugin config.h ${XENSTAT_PLUGIN_FILES})
    target_link_libraries (xenstat.plugin libnetdata ${NETDATA_COMMON_LIBRARIES} ${XENSTAT_LIBRARIES})
    target_include_directories(xenstat.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS} ${XENSTAT_INCLUDE_DIRS})
    target_compile_options(xenstat.plugin PUBLIC ${NETDATA_COMMON_CFLAGS} ${XENSTAT_CFLAGS_OTHER})
ELSE()
    message(STATUS "xenstat.plugin: disabled (requires libxenstat)")
ENDIF()


# -----------------------------------------------------------------------------
# perf.plugin

IF(ENABLE_PLUGIN_PERF)
    message(STATUS "perf.plugin: enabled")
    add_executable(perf.plugin config.h ${PERF_PLUGIN_FILES})
    target_link_libraries (perf.plugin libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(perf.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_compile_options(perf.plugin PUBLIC ${NETDATA_COMMON_CFLAGS})
ELSE()
    message(STATUS "perf.plugin: disabled")
ENDIF()


# -----------------------------------------------------------------------------
# slabinfo.plugin

IF(ENABLE_PLUGIN_SLABINFO)
    message(STATUS "slabinfo.plugin: enabled")
    add_executable(slabinfo.plugin config.h ${SLABINFO_PLUGIN_FILES})
    target_link_libraries (slabinfo.plugin libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(slabinfo.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_compile_options(slabinfo.plugin PUBLIC ${NETDATA_COMMON_CFLAGS})
ELSE()
    message(STATUS "slabinfo.plugin: disabled")
ENDIF()


# -----------------------------------------------------------------------------
# cgroup-network

IF(ENABLE_PLUGIN_CGROUP_NETWORK)
    message(STATUS "cgroup-network: enabled")
    add_executable(cgroup-network config.h ${CGROUP_NETWORK_FILES})
    target_link_libraries (cgroup-network libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(cgroup-network PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_compile_options(cgroup-network PUBLIC ${NETDATA_COMMON_CFLAGS})
ELSE()
    message(STATUS "cgroup-network: disabled (requires Linux)")
ENDIF()
