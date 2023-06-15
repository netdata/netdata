
# SPDX-License-Identifier: GPL-3.0-or-later
# This file is only used for development (netdata in Clion)
# It can build netdata, but you are on your own...

cmake_minimum_required(VERSION 3.1.0)
project(netdata C CXX)

find_package(Threads REQUIRED)
find_package(PkgConfig REQUIRED)

include(CheckFunctionExists)
include(CheckLibraryExists)

# default is "Debug"
#set(CMAKE_BUILD_TYPE "Release")

# set this to see the compilation commands
#set(CMAKE_VERBOSE_MAKEFILE 1)

# -----------------------------------------------------------------------------
# Set compilation options according to build type

set(CMAKE_C_STANDARD 11)

IF("${CMAKE_BUILD_TYPE}" MATCHES "Debug")
    set(CXX_DEFAULT_CFLAGS "-O0 -g -DNETDATA_INTERNAL_CHECKS=1 -DNETDATA_DEV_MODE=1 -fstack-protector-all -fno-omit-frame-pointer")
    message(STATUS "building for: debugging")
ELSE()
    set(CXX_DEFAULT_CFLAGS "-O2")
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

set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -O1 -ggdb -Wall -Wextra -Wformat-signedness -Werror=format-security -Wunused-result ${CXX_DEFAULT_CFLAGS}")

# -----------------------------------------------------------------------------
# O/S Detection

# these are defined in common.h too
SET(LINUX   False)
SET(FREEBSD False)
SET(MACOS   False)

# Detect the operating system
IF(${CMAKE_SYSTEM_NAME} MATCHES "Darwin")
    SET(MACOS True)
ELSEIF(${CMAKE_SYSTEM_NAME} MATCHES "FreeBSD")
    SET(FREEBSD True)
ELSE()
    SET(LINUX True)
ENDIF()

# show the operating system on the console
message(STATUS "system name: ${CMAKE_SYSTEM_NAME}")

set(GENERATED_CONFIG_H_DIR ${CMAKE_BINARY_DIR})
set(GENERATED_CONFIG_H ${GENERATED_CONFIG_H_DIR}/config.h)

# -----------------------------------------------------------------------------
# Math

set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} m)
set(NETDATA_REQUIRED_DEFINES "-DSTORAGE_WITH_MATH=1 ${NETDATA_REQUIRED_DEFINES}")

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
# set(NETDATA_REQUIRED_DEFINES "${NETDATA_REQUIRED_DEFINES} -DENABLE_COMPRESSION=1")

# -----------------------------------------------------------------------------
# Judy General purpose dynamic array

set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} judy)

# -----------------------------------------------------------------------------
# OpenSSL Cryptography and SSL/TLS Toolkit

pkg_check_modules(OPENSSL REQUIRED openssl)
set(NETDATA_COMMON_CFLAGS ${NETDATA_COMMON_CFLAGS} ${OPENSSL_CFLAGS_OTHER})
set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} ${OPENSSL_LIBRARIES})
set(NETDATA_COMMON_INCLUDE_DIRS ${NETDATA_COMMON_INCLUDE_DIRS} ${OPENSSL_INCLUDE_DIRS})
# set(NETDATA_REQUIRED_DEFINES "${NETDATA_REQUIRED_DEFINES} -DENABLE_HTTPS=1")

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
# Detect libcups

pkg_check_modules(CUPS libcups)
IF(NOT CUPS_LIBRARIES)
    pkg_check_modules(CUPS cups)
ENDIF()

# later we use:
# ${CUPS_LIBRARIES}
# ${CUPS_CFLAGS_OTHER}
# ${CUPS_INCLUDE_DIRS}

# -----------------------------------------------------------------------------
# Detect libaws-c-common

find_library(HAVE_AWS_CHECKSUMS aws-checksums)
# later we use:
# ${HAVE_AWS_CHECKSUMS}

# -----------------------------------------------------------------------------
# Detect libaws-c-common

find_library(HAVE_AWS_COMMON aws-c-common)
# later we use:
# ${HAVE_AWS_COMMON}

# -----------------------------------------------------------------------------
# Detect libaws-c-common

find_library(HAVE_AWS_EVENT_STREAM aws-c-event-stream)
# later we use:
# ${HAVE_AWS_EVENT_STREAM}

# -----------------------------------------------------------------------------
# Detect libaws-cpp-sdk-core

pkg_check_modules(AWS_CORE aws-cpp-sdk-core)
# later we use:
# ${AWS_CORE_LIBRARIES}
# ${AWS_CORE_CFLAGS_OTHER}
# ${AWS_CORE_INCLUDE_DIRS}

# -----------------------------------------------------------------------------
# Detect libaws-cpp-sdk-kinesis

pkg_check_modules(KINESIS aws-cpp-sdk-kinesis)
# later we use:
# ${KINESIS_LIBRARIES}
# ${KINESIS_CFLAGS_OTHER}
# ${KINESIS_INCLUDE_DIRS}

# -----------------------------------------------------------------------------
# Detect libgrpc

pkg_check_modules(GRPC grpc)
# later we use:
# ${GRPC_LIBRARIES}
# ${GRPC_CFLAGS_OTHER}
# ${GRPC_INCLUDE_DIRS}

# -----------------------------------------------------------------------------
# Detect libgoogleapis_cpp_pubsub_protos

pkg_check_modules(PUBSUB google_cloud_cpp_pubsub_protos)
# later we use:
# ${PUBSUB_LIBRARIES}
# ${PUBSUB_CFLAGS_OTHER}
# ${PUBSUB_INCLUDE_DIRS}

# -----------------------------------------------------------------------------
# Detect libprotobuf

pkg_check_modules(PROTOBUF protobuf>=3)
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

pkg_check_modules(MONGOC libmongoc-1.0)
# later we use:
# ${MONGOC_LIBRARIES}
# ${MONGOC_CFLAGS_OTHER}
# ${MONGOC_INCLUDE_DIRS}


# -----------------------------------------------------------------------------
# Detect libbpf
IF(LINUX AND EXISTS "${CMAKE_SOURCE_DIR}/externaldeps/libbpf/libbpf.a")
    message(STATUS "libbpf library found")
    pkg_check_modules(ELF REQUIRED libelf)
    # later we use:
    # ${ELF_LIBRARIES}
    # ${ELF_CFLAGS_OTHER}
    # ${ELF_INCLUDE_DIRS}
    IF(ELF_LIBRARIES)
        list(APPEND NETDATA_COMMON_CFLAGS ${ELF_CFLAGS_OTHER})
        list(INSERT NETDATA_COMMON_LIBRARIES 0
            ${CMAKE_SOURCE_DIR}/externaldeps/libbpf/libbpf.a
            ${ELF_LIBRARIES})
        list(APPEND NETDATA_COMMON_INCLUDE_DIRS ${ELF_INCLUDE_DIRS})
        include_directories(BEFORE ${CMAKE_SOURCE_DIR}/externaldeps/libbpf/include ${CMAKE_SOURCE_DIR}/externaldeps/libbpf/include/uapi)
        set(ENABLE_PLUGIN_EBPF True)
        set(HAVE_LIBBPF True)
    ELSE(ELF_LIBRARIES)
        set(ENABLE_PLUGIN_EBPF False)
        message(STATUS "ebpf plugin: disabled (requires libelf)")
    ENDIF(ELF_LIBRARIES)
ENDIF()

# -----------------------------------------------------------------------------
# Detect ml dependencies
file(STRINGS "${CMAKE_SOURCE_DIR}/config.h" DEFINE_ENABLE_ML REGEX "^#define ENABLE_ML 1$")
IF(DEFINE_ENABLE_ML MATCHES ".+" AND
   EXISTS "${CMAKE_SOURCE_DIR}/ml/dlib/dlib/all/source.cpp")
    set(ENABLE_ML True)
    list(APPEND NETDATA_COMMON_CFLAGS "-DDLIB_NO_GUI_SUPPORT")
    list(APPEND NETDATA_COMMON_INCLUDE_DIRS "ml/dlib")
ELSE()
    set(ENABLE_ML False)
ENDIF()

set(LIBJUDY_SOURCES
        libnetdata/libjudy/src/Judy.h
        libnetdata/libjudy/src/JudyCommon/JudyMalloc.c
        libnetdata/libjudy/src/JudyCommon/JudyPrivate.h
        libnetdata/libjudy/src/JudyCommon/JudyPrivate1L.h
        libnetdata/libjudy/src/JudyCommon/JudyPrivateBranch.h
        libnetdata/libjudy/src/JudyL/JudyL.h
        libnetdata/libjudy/src/JudyL/JudyLByCount.c
        libnetdata/libjudy/src/JudyL/JudyLCascade.c
        libnetdata/libjudy/src/JudyL/JudyLCount.c
        libnetdata/libjudy/src/JudyL/JudyLCreateBranch.c
        libnetdata/libjudy/src/JudyL/JudyLDecascade.c
        libnetdata/libjudy/src/JudyL/JudyLDel.c
        libnetdata/libjudy/src/JudyL/JudyLFirst.c
        libnetdata/libjudy/src/JudyL/JudyLFreeArray.c
        libnetdata/libjudy/src/JudyL/j__udyLGet.c
        libnetdata/libjudy/src/JudyL/JudyLGet.c
        libnetdata/libjudy/src/JudyL/JudyLInsArray.c
        libnetdata/libjudy/src/JudyL/JudyLIns.c
        libnetdata/libjudy/src/JudyL/JudyLInsertBranch.c
        libnetdata/libjudy/src/JudyL/JudyLMallocIF.c
        libnetdata/libjudy/src/JudyL/JudyLMemActive.c
        libnetdata/libjudy/src/JudyL/JudyLMemUsed.c
        libnetdata/libjudy/src/JudyL/JudyLNext.c
        libnetdata/libjudy/src/JudyL/JudyLNextEmpty.c
        libnetdata/libjudy/src/JudyL/JudyLPrev.c
        libnetdata/libjudy/src/JudyL/JudyLPrevEmpty.c
        libnetdata/libjudy/src/JudyL/JudyLTables.c
        libnetdata/libjudy/src/JudyHS/JudyHS.c)

ADD_LIBRARY(judy STATIC
             ${LIBJUDY_SOURCES})

include_directories(BEFORE ${CMAKE_SOURCE_DIR}/libnetdata/libjudy/src)

target_include_directories(judy PUBLIC
    libnetdata/libjudy/src
    libnetdata/libjudy/src/JudyCommon)

target_include_directories(judy BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})

target_compile_definitions(judy PUBLIC
    JU_64BIT
    JUDYL)

target_compile_options(judy PUBLIC
    -Wno-sign-compare
    -Wno-implicit-fallthrough)

set(LIBJUDY_PREV_FILES
        libnetdata/libjudy/src/JudyL/JudyLPrev.c
        libnetdata/libjudy/src/JudyL/JudyLPrevEmpty.c)

set(LIBJUDY_NEXT_FILES
        libnetdata/libjudy/src/JudyL/JudyLNext.c
        libnetdata/libjudy/src/JudyL/JudyLNextEmpty.c)

set_source_files_properties(${LIBJUDY_PREV_FILES} PROPERTIES COMPILE_OPTIONS "-DJUDYPREV")
set_source_files_properties(${LIBJUDY_NEXT_FILES} PROPERTIES COMPILE_OPTIONS "-DJUDYNEXT")
set_source_files_properties(libnetdata/libjudy/src/JudyL/j__udyLGet.c PROPERTIES COMPILE_OPTIONS "-DJUDYGETINLINE")
set_source_files_properties(libnetdata/libjudy/src/JudyL/JudyLByCount.c PROPERTIES COMPILE_OPTIONS "-DNOSMARTJBB -DNOSMARTJBU -DNOSMARTJLB")
set_source_files_properties(JudyLTables.c PROPERTIES COMPILE_OPTIONS "-I${CMAKE_SOURCE_DIR}/libnetdata/libjudy/src/JudyL")

# -----------------------------------------------------------------------------
# netdata files

set(LIBNETDATA_FILES
        libnetdata/adaptive_resortable_list/adaptive_resortable_list.c
        libnetdata/adaptive_resortable_list/adaptive_resortable_list.h
        libnetdata/config/appconfig.c
        libnetdata/config/appconfig.h
        libnetdata/aral/aral.c
        libnetdata/aral/aral.h
        libnetdata/avl/avl.c
        libnetdata/avl/avl.h
        libnetdata/buffer/buffer.c
        libnetdata/buffer/buffer.h
        libnetdata/circular_buffer/circular_buffer.c
        libnetdata/circular_buffer/circular_buffer.h
        libnetdata/clocks/clocks.c
        libnetdata/clocks/clocks.h
        libnetdata/completion/completion.c
        libnetdata/completion/completion.h
        libnetdata/dictionary/dictionary.c
        libnetdata/dictionary/dictionary.h
        libnetdata/eval/eval.c
        libnetdata/eval/eval.h
        libnetdata/gorilla/gorilla.cc
        libnetdata/gorilla/gorilla.h
        libnetdata/health/health.c
        libnetdata/health/health.h
        libnetdata/july/july.c
        libnetdata/july/july.h
        libnetdata/inlined.h
        libnetdata/json/json.c
        libnetdata/json/json.h
        libnetdata/json/jsmn.c
        libnetdata/json/jsmn.h
        libnetdata/libnetdata.c
        libnetdata/libnetdata.h
        libnetdata/locks/locks.c
        libnetdata/locks/locks.h
        libnetdata/log/log.c
        libnetdata/log/log.h
        libnetdata/os.c
        libnetdata/os.h
        libnetdata/onewayalloc/onewayalloc.c
        libnetdata/onewayalloc/onewayalloc.h
        libnetdata/popen/popen.c
        libnetdata/popen/popen.h
        libnetdata/procfile/procfile.c
        libnetdata/procfile/procfile.h
        libnetdata/required_dummies.h
        libnetdata/socket/security.c
        libnetdata/socket/security.h
        libnetdata/simple_pattern/simple_pattern.c
        libnetdata/simple_pattern/simple_pattern.h
        libnetdata/socket/socket.c
        libnetdata/socket/socket.h
        libnetdata/statistical/statistical.c
        libnetdata/statistical/statistical.h
        libnetdata/storage_number/storage_number.c
        libnetdata/storage_number/storage_number.h
        libnetdata/string/string.c
        libnetdata/string/string.h
        libnetdata/threads/threads.c
        libnetdata/threads/threads.h
        libnetdata/url/url.c
        libnetdata/url/url.h
        libnetdata/string/utf8.h
        libnetdata/worker_utilization/worker_utilization.c
        libnetdata/worker_utilization/worker_utilization.h
		libnetdata/parser/parser.h
		libnetdata/parser/parser.c
        libnetdata/http/http_defs.h
        )

IF(ENABLE_PLUGIN_EBPF)
    list(APPEND LIBNETDATA_FILES
        libnetdata/ebpf/ebpf.c
        libnetdata/ebpf/ebpf.h)
ENDIF()

add_library(libnetdata OBJECT ${LIBNETDATA_FILES})

target_include_directories(libnetdata BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})

set(APPS_PLUGIN_FILES
        collectors/apps.plugin/apps_plugin.c)

set(DEBUGFS_PLUGIN_FILES
        collectors/debugfs.plugin/debugfs_plugin.c
        collectors/debugfs.plugin/debugfs_plugin.h
        collectors/debugfs.plugin/debugfs_extfrag.c
        collectors/debugfs.plugin/debugfs_zswap.c
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
        )

set(CGROUPS_PLUGIN_FILES
        collectors/cgroups.plugin/sys_fs_cgroup.c
        collectors/cgroups.plugin/sys_fs_cgroup.h
        )

set(CGROUP_NETWORK_FILES
        collectors/cgroups.plugin/cgroup-network.c
        )

set(DISKSPACE_PLUGIN_FILES
        collectors/diskspace.plugin/plugin_diskspace.c
        )

set(TIMEX_PLUGIN_FILES
        collectors/timex.plugin/plugin_timex.c
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

set(CUPS_PLUGIN_FILES
        collectors/cups.plugin/cups_plugin.c
        )

set(EBPF_PROCESS_PLUGIN_FILES
        collectors/ebpf.plugin/ebpf.c
        collectors/ebpf.plugin/ebpf.h
        collectors/ebpf.plugin/ebpf_cachestat.c
        collectors/ebpf.plugin/ebpf_cachestat.h
        collectors/ebpf.plugin/ebpf_dcstat.c
        collectors/ebpf.plugin/ebpf_dcstat.h
        collectors/ebpf.plugin/ebpf_disk.c
        collectors/ebpf.plugin/ebpf_disk.h
        collectors/ebpf.plugin/ebpf_fd.c
        collectors/ebpf.plugin/ebpf_fd.h
        collectors/ebpf.plugin/ebpf_hardirq.c
        collectors/ebpf.plugin/ebpf_hardirq.h
        collectors/ebpf.plugin/ebpf_mdflush.c
        collectors/ebpf.plugin/ebpf_mdflush.h
        collectors/ebpf.plugin/ebpf_mount.c
        collectors/ebpf.plugin/ebpf_mount.h
        collectors/ebpf.plugin/ebpf_filesystem.c
        collectors/ebpf.plugin/ebpf_filesystem.h
        collectors/ebpf.plugin/ebpf_oomkill.c
        collectors/ebpf.plugin/ebpf_oomkill.h
        collectors/ebpf.plugin/ebpf_process.c
        collectors/ebpf.plugin/ebpf_process.h
        collectors/ebpf.plugin/ebpf_shm.c
        collectors/ebpf.plugin/ebpf_shm.h
        collectors/ebpf.plugin/ebpf_socket.c
        collectors/ebpf.plugin/ebpf_socket.h
        collectors/ebpf.plugin/ebpf_softirq.c
        collectors/ebpf.plugin/ebpf_softirq.h
        collectors/ebpf.plugin/ebpf_sync.c
        collectors/ebpf.plugin/ebpf_sync.h
        collectors/ebpf.plugin/ebpf_swap.c
        collectors/ebpf.plugin/ebpf_swap.h
        collectors/ebpf.plugin/ebpf_vfs.c
        collectors/ebpf.plugin/ebpf_vfs.h
        collectors/ebpf.plugin/ebpf_apps.c
        collectors/ebpf.plugin/ebpf_apps.h
        collectors/ebpf.plugin/ebpf_cgroup.c
        collectors/ebpf.plugin/ebpf_cgroup.h
        collectors/ebpf.plugin/ebpf_unittest.c
        collectors/ebpf.plugin/ebpf_unittest.h
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
        collectors/proc.plugin/proc_pagetypeinfo.c
        collectors/proc.plugin/proc_net_dev.c
        collectors/proc.plugin/proc_net_wireless.c
        collectors/proc.plugin/proc_net_ip_vs_stats.c
        collectors/proc.plugin/proc_net_netstat.c
        collectors/proc.plugin/proc_net_rpc_nfs.c
        collectors/proc.plugin/proc_net_rpc_nfsd.c
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
        collectors/proc.plugin/proc_pressure.c
        collectors/proc.plugin/proc_pressure.h
        collectors/proc.plugin/sys_kernel_mm_ksm.c
        collectors/proc.plugin/sys_block_zram.c
        collectors/proc.plugin/sys_devices_system_edac_mc.c
        collectors/proc.plugin/sys_devices_system_node.c
        collectors/proc.plugin/sys_class_infiniband.c
        collectors/proc.plugin/sys_fs_btrfs.c
        collectors/proc.plugin/sys_class_power_supply.c
        )

set(TC_PLUGIN_FILES
        collectors/tc.plugin/plugin_tc.c
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
        collectors/plugins.d/pluginsd_parser.c
        collectors/plugins.d/pluginsd_parser.h
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
        )

set(RRD_PLUGIN_FILES
        database/contexts/api_v1.c
        database/contexts/api_v2.c
        database/contexts/context.c
        database/contexts/instance.c
        database/contexts/internal.h
        database/contexts/metric.c
        database/contexts/query_scope.c
        database/contexts/query_target.c
        database/contexts/rrdcontext.c
        database/contexts/rrdcontext.h
        database/contexts/worker.c
        database/rrdcalc.c
        database/rrdcalc.h
        database/rrdcalctemplate.c
        database/rrdcalctemplate.h
        database/rrddim.c
        database/rrddimvar.c
        database/rrddimvar.h
        database/rrdfamily.c
        database/rrdfunctions.c
        database/rrdfunctions.h
        database/rrdhost.c
        database/rrdlabels.c
        database/rrd.c
        database/rrd.h
        database/rrdset.c
        database/rrdsetvar.c
        database/rrdsetvar.h
        database/rrdvar.c
        database/rrdvar.h
        database/storage_engine.c
        database/storage_engine.h
        database/ram/rrddim_mem.c
        database/ram/rrddim_mem.h
        database/sqlite/sqlite_metadata.c
        database/sqlite/sqlite_metadata.h
        database/sqlite/sqlite_functions.c
        database/sqlite/sqlite_functions.h
        database/sqlite/sqlite_context.c
        database/sqlite/sqlite_context.h
        database/sqlite/sqlite_db_migration.c
        database/sqlite/sqlite_db_migration.h
        database/sqlite/sqlite_aclk.c
        database/sqlite/sqlite_aclk.h
        database/sqlite/sqlite_health.c
        database/sqlite/sqlite_health.h
        database/sqlite/sqlite_aclk_node.c
        database/sqlite/sqlite_aclk_node.h
        database/sqlite/sqlite_aclk_alert.c
        database/sqlite/sqlite_aclk_alert.h
        database/sqlite/sqlite3.c
        database/sqlite/sqlite3.h
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
        database/engine/cache.c
        database/engine/cache.h
        database/engine/metric.c
        database/engine/metric.h
        database/engine/pdc.c
        database/engine/pdc.h
        database/KolmogorovSmirnovDist.c
        database/KolmogorovSmirnovDist.h
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
        web/api/web_api.c
        web/api/web_api.h
        web/api/web_api_v1.c
        web/api/web_api_v1.h
        web/api/web_api_v2.c
        web/api/web_api_v2.h
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
        web/api/queries/countif/countif.c
        web/api/queries/countif/countif.h
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
        web/api/queries/percentile/percentile.c
        web/api/queries/percentile/percentile.h
        web/api/queries/stddev/stddev.c
        web/api/queries/stddev/stddev.h
        web/api/queries/ses/ses.c
        web/api/queries/ses/ses.h
        web/api/queries/des/des.c
        web/api/queries/des/des.h
        web/api/queries/trimmed_mean/trimmed_mean.c
        web/api/queries/trimmed_mean/trimmed_mean.h
        web/api/queries/weights.c
        web/api/queries/weights.h
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
        web/rtc/webrtc.c
        web/rtc/webrtc.h
        )

set(STREAMING_PLUGIN_FILES
        streaming/rrdpush.c
        streaming/rrdpush.h
        streaming/compression.c
        streaming/receiver.c
        streaming/sender.c
        streaming/replication.c
        streaming/replication.h
        )

set(CLAIM_PLUGIN_FILES
        claim/claim.c
        claim/claim.h
        )

set(ACLK_ALWAYS_BUILD
        aclk/aclk_rrdhost_state.h
        aclk/aclk_proxy.c
        aclk/aclk_proxy.h
        aclk/aclk.c
        aclk/aclk.h
        aclk/aclk_capas.c
        aclk/aclk_capas.h
        )

set(ACLK_FILES
        aclk/aclk_util.c
        aclk/aclk_util.h
        aclk/aclk_stats.c
        aclk/aclk_stats.h
        aclk/aclk_query.c
        aclk/aclk_query.h
        aclk/aclk_query_queue.c
        aclk/aclk_query_queue.h
        aclk/aclk_otp.c
        aclk/aclk_otp.h
        aclk/aclk_tx_msgs.c
        aclk/aclk_tx_msgs.h
        aclk/aclk_rx_msgs.c
        aclk/aclk_rx_msgs.h
        aclk/https_client.c
        aclk/https_client.h
        aclk/aclk_alarm_api.c
        aclk/aclk_alarm_api.h
        aclk/aclk_contexts_api.c
        aclk/aclk_contexts_api.h
        aclk/schema-wrappers/connection.cc
        aclk/schema-wrappers/connection.h
        aclk/schema-wrappers/node_connection.cc
        aclk/schema-wrappers/node_connection.h
        aclk/schema-wrappers/node_creation.cc
        aclk/schema-wrappers/node_creation.h
        aclk/schema-wrappers/alarm_stream.cc
        aclk/schema-wrappers/alarm_stream.h
        aclk/schema-wrappers/alarm_config.cc
        aclk/schema-wrappers/alarm_config.h
        aclk/schema-wrappers/node_info.cc
        aclk/schema-wrappers/node_info.h
        aclk/schema-wrappers/capability.cc
        aclk/schema-wrappers/capability.h
        aclk/schema-wrappers/proto_2_json.cc
        aclk/schema-wrappers/proto_2_json.h
        aclk/schema-wrappers/context_stream.cc
        aclk/schema-wrappers/context_stream.h
        aclk/schema-wrappers/context.cc
        aclk/schema-wrappers/context.h
        aclk/schema-wrappers/schema_wrappers.h
        aclk/schema-wrappers/schema_wrapper_utils.cc
        aclk/schema-wrappers/schema_wrapper_utils.h
        aclk/schema-wrappers/agent_cmds.cc
        aclk/schema-wrappers/agent_cmds.h
        aclk/helpers/mqtt_wss_pal.h
        aclk/helpers/ringbuffer_pal.h
        )

set(MQTT_WEBSOCKETS_FILES
        mqtt_websockets/src/mqtt_wss_client.c
        mqtt_websockets/src/include/mqtt_wss_client.h
        mqtt_websockets/src/mqtt_wss_log.c
        mqtt_websockets/src/include/mqtt_wss_log.h
        mqtt_websockets/src/ws_client.c
        mqtt_websockets/src/include/ws_client.h
        mqtt_websockets/src/mqtt_ng.c
        mqtt_websockets/src/include/mqtt_ng.h
        mqtt_websockets/src/common_public.c
        mqtt_websockets/src/include/common_public.h
        mqtt_websockets/src/include/common_internal.h
        mqtt_websockets/c-rbuf/src/ringbuffer.c
        mqtt_websockets/c-rbuf/include/ringbuffer.h
        mqtt_websockets/c-rbuf/src/ringbuffer_internal.h
        )

set(SPAWN_PLUGIN_FILES
        spawn/spawn.c
        spawn/spawn_server.c
        spawn/spawn_client.c
        spawn/spawn.h
        )

set(EXPORTING_ENGINE_FILES
        exporting/exporting_engine.c
        exporting/exporting_engine.h
        exporting/graphite/graphite.c
        exporting/graphite/graphite.h
        exporting/json/json.c
        exporting/json/json.h
        exporting/opentsdb/opentsdb.c
        exporting/opentsdb/opentsdb.h
        exporting/prometheus/prometheus.c
        exporting/prometheus/prometheus.h
        exporting/read_config.c
        exporting/clean_connectors.c
        exporting/init_connectors.c
        exporting/process_data.c
        exporting/check_filters.c
        exporting/send_data.c
        exporting/send_internal_metrics.c
        )

set(PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES
        exporting/prometheus/remote_write/remote_write.c
        exporting/prometheus/remote_write/remote_write.h
        exporting/prometheus/remote_write/remote_write_request.cc
        exporting/prometheus/remote_write/remote_write_request.h
        )

set(KINESIS_EXPORTING_FILES
        exporting/aws_kinesis/aws_kinesis.c
        exporting/aws_kinesis/aws_kinesis.h
        exporting/aws_kinesis/aws_kinesis_put_record.cc
        exporting/aws_kinesis/aws_kinesis_put_record.h
        )

set(PUBSUB_EXPORTING_FILES
        exporting/pubsub/pubsub.c
        exporting/pubsub/pubsub.h
        exporting/pubsub/pubsub_publish.cc
        exporting/pubsub/pubsub_publish.h
        )

set(MONGODB_EXPORTING_FILES
        exporting/mongodb/mongodb.c
        exporting/mongodb/mongodb.h
        )

set(DAEMON_FILES
        daemon/buildinfo.c
        daemon/buildinfo.h
        daemon/common.c
        daemon/common.h
        daemon/daemon.c
        daemon/daemon.h
        daemon/event_loop.c
        daemon/event_loop.h
        daemon/global_statistics.c
        daemon/global_statistics.h
        daemon/analytics.c
        daemon/analytics.h
        daemon/main.c
        daemon/main.h
        daemon/signals.c
        daemon/signals.h
        daemon/service.c
        daemon/static_threads.c
        daemon/static_threads.h
        daemon/commands.c
        daemon/commands.h
        daemon/unit_test.c
        daemon/unit_test.h
        )

set(ML_FILES
    ml/ml.h
    ml/ml-dummy.c
)

# -----------------------------------------------------------------------------
# ML

IF(ENABLE_ML)
    message(STATUS "ML: enabled")
    list(APPEND ML_FILES
            ml/ad_charts.h
            ml/ad_charts.cc
            ml/Config.cc
            ml/dlib/dlib/all/source.cpp
            ml/ml.cc
            ml/ml-private.h
    )
ELSE()
    message(STATUS "ML: disabled")
ENDIF()

set(NETDATA_FILES
        collectors/all.h
        ${DAEMON_FILES}
        ${API_PLUGIN_FILES}
        ${EXPORTING_ENGINE_FILES}
        ${HEALTH_PLUGIN_FILES}
        ${IDLEJITTER_PLUGIN_FILES}
        ${ML_FILES}
        ${PLUGINSD_PLUGIN_FILES}
        ${RRD_PLUGIN_FILES}
        ${REGISTRY_PLUGIN_FILES}
        ${STATSD_PLUGIN_FILES}
        ${STREAMING_PLUGIN_FILES}
        ${WEB_PLUGIN_FILES}
        ${CLAIM_PLUGIN_FILES}
        ${SPAWN_PLUGIN_FILES}
)

set(NETDATACLI_FILES
        daemon/commands.h
        cli/cli.c
        cli/cli.h
        )

include_directories(AFTER .)

add_definitions(
        -DHAVE_CONFIG_H
        -DCACHE_DIR="/var/cache/netdata"
        -DCONFIG_DIR="/etc/netdata"
        -DLIBCONFIG_DIR="/usr/lib/netdata/conf.d"
        -DLOG_DIR="/var/log/netdata"
        -DPLUGINS_DIR="/usr/libexec/netdata/plugins.d"
        -DWEB_DIR="/usr/share/netdata/web"
        -DVARLIB_DIR="/var/lib/netdata"
)

# -----------------------------------------------------------------------------
# kinesis exporting connector

IF(KINESIS_LIBRARIES AND AWS_CORE_LIBRARIES AND HAVE_AWS_EVENT_STREAM AND HAVE_AWS_COMMON AND HAVE_AWS_CHECKSUMS AND
   CRYPTO_LIBRARIES AND SSL_LIBRARIES AND CURL_LIBRARIES)
    SET(ENABLE_EXPORTING_KINESIS True)
ELSE()
    SET(ENABLE_EXPORTING_KINESIS False)
ENDIF()

IF(ENABLE_EXPORTING_KINESIS)
    message(STATUS "kinesis exporting: enabled")
    list(APPEND NETDATA_FILES ${KINESIS_EXPORTING_FILES})
    list(APPEND NETDATA_COMMON_LIBRARIES ${KINESIS_LIBRARIES} ${AWS_CORE_LIBRARIES}
                                         ${CRYPTO_LIBRARIES} ${SSL_LIBRARIES} ${CURL_LIBRARIES})
    list(APPEND NETDATA_COMMON_INCLUDE_DIRS ${KINESIS_INCLUDE_DIRS} ${AWS_CORE_INCLUDE_DIRS}
                                            ${CRYPTO_INCLUDE_DIRS} ${SSL_INCLUDE_DIRS} ${CURL_INCLUDE_DIRS})
    list(APPEND NETDATA_COMMON_CFLAGS ${CRYPTO_CFLAGS_OTHER} ${SSL_CFLAGS_OTHER} ${CURL_CFLAGS_OTHER})
ELSE()
    message(STATUS "kinesis exporting: disabled (requires AWS SDK for C++)")
ENDIF()

# -----------------------------------------------------------------------------
# Pub/Sub exporting connector

IF(GRPC_LIBRARIES AND PUBSUB_LIBRARIES)
    SET(ENABLE_EXPORTING_PUBSUB True)
ELSE()
    SET(ENABLE_EXPORTING_PUBSUB False)
ENDIF()

IF(ENABLE_EXPORTING_PUBSUB)
    message(STATUS "pubsub exporting connector: enabled")
    list(APPEND NETDATA_FILES ${PUBSUB_EXPORTING_FILES})
    list(APPEND NETDATA_COMMON_LIBRARIES ${GRPC_LIBRARIES} ${PUBSUB_LIBRARIES})
    list(APPEND NETDATA_COMMON_INCLUDE_DIRS ${GRPC_INCLUDE_DIRS} ${PUBSUB_INCLUDE_DIRS})
    list(APPEND NETDATA_COMMON_CFLAGS ${GRPC_CFLAGS_OTHER} ${PUBSUB_CFLAGS_OTHER})
ELSE()
    message(STATUS "pubsub exporting connector: disabled (requires grpc and googleapis)")
ENDIF()

# -----------------------------------------------------------------------------
# prometheus remote write exporting connector

IF(PROTOBUF_LIBRARIES AND SNAPPY_LIBRARIES)
    SET(ENABLE_EXPORTING_PROMETHEUS_REMOTE_WRITE True)
ELSE()
    SET(ENABLE_EXPORTING_PROMETHEUS_REMOTE_WRITE False)
ENDIF()

IF(ENABLE_EXPORTING_PROMETHEUS_REMOTE_WRITE)
    message(STATUS "prometheus remote write exporting: enabled")

    find_package(Protobuf REQUIRED)

    function(PROTOBUF_REMOTE_WRITE_GENERATE_CPP SRCS HDRS)
        if(NOT ARGN)
            message(SEND_ERROR "Error: PROTOBUF_REMOTE_WRITE_GENERATE_CPP() called without any proto files")
            return()
        endif()

        set(${SRCS})
        set(${HDRS})
        foreach(FIL ${ARGN})
            get_filename_component(ABS_FIL ${FIL} ABSOLUTE)
            get_filename_component(DIR ${ABS_FIL} DIRECTORY)
            get_filename_component(FIL_WE ${FIL} NAME_WE)
            set(GENERATED_PB_CC "${DIR}/${FIL_WE}.pb.cc")
            set(GENERATED_PB_H "${DIR}/${FIL_WE}.pb.h")
            list(APPEND ${SRCS} ${GENERATED_PB_CC})
            list(APPEND ${HDRS} ${GENERATED_PB_H})
            add_custom_command(
            OUTPUT ${GENERATED_PB_CC}
                    ${GENERATED_PB_H}
            COMMAND  ${PROTOBUF_PROTOC_EXECUTABLE}
            ARGS -I=${CMAKE_SOURCE_DIR}/exporting/prometheus/remote_write --cpp_out=${CMAKE_SOURCE_DIR}/exporting/prometheus/remote_write ${ABS_FIL}
            DEPENDS ${ABS_FIL} ${PROTOBUF_PROTOC_EXECUTABLE}
            COMMENT "Running C++ protocol buffer compiler on ${FIL}"
            VERBATIM )
        endforeach()
        set_source_files_properties(${${SRCS}} ${${HDRS}} PROPERTIES GENERATED TRUE)
        set(${SRCS} ${${SRCS}} PARENT_SCOPE)
        set(${HDRS} ${${HDRS}} PARENT_SCOPE)
    endfunction()

    protobuf_remote_write_generate_cpp(PROTO_SRCS PROTO_HDRS exporting/prometheus/remote_write/remote_write.proto)

    list(APPEND NETDATA_FILES ${PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES} ${PROTO_SRCS} ${PROTO_HDRS})
    list(APPEND NETDATA_COMMON_LIBRARIES ${PROTOBUF_LIBRARIES} ${SNAPPY_LIBRARIES})
    list(APPEND NETDATA_COMMON_INCLUDE_DIRS ${PROTOBUF_INCLUDE_DIRS} ${SNAPPY_INCLUDE_DIRS} ${CMAKE_CURRENT_BINARY_DIR})
    list(APPEND NETDATA_COMMON_CFLAGS ${PROTOBUF_CFLAGS_OTHER} ${SNAPPY_CFLAGS_OTHER})
ELSE()
    message(STATUS "prometheus remote write exporting: disabled (requires protobuf and snappy libraries)")
ENDIF()

# -----------------------------------------------------------------------------
# mongodb exporting connector

IF(MONGOC_LIBRARIES)
    message(STATUS "mongodb exporting: enabled")

    list(APPEND NETDATA_FILES ${MONGODB_EXPORTING_FILES})
    list(APPEND NETDATA_COMMON_LIBRARIES ${MONGOC_LIBRARIES})
    list(APPEND NETDATA_COMMON_INCLUDE_DIRS ${MONGOC_INCLUDE_DIRS})
    list(APPEND NETDATA_COMMON_CFLAGS ${MONGOC_CFLAGS_OTHER})
ELSE()
    message(STATUS "mongodb exporting: disabled (requires mongoc library)")
ENDIF()

set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} m ${CMAKE_THREAD_LIBS_INIT})

option(ENABLE_ACLK "Enables functionality required to connect to cloud (must be activated by user at run time)" ON)

if(ENABLE_ACLK)

find_package(Protobuf REQUIRED)

function(PROTOBUF_ACLK_GENERATE_CPP SRCS HDRS)
  if(NOT ARGN)
    message(SEND_ERROR "Error: PROTOBUF_ACLK_GENERATE_CPP() called without any proto files")
    return()
  endif()

  set(${SRCS})
  set(${HDRS})
  foreach(FIL ${ARGN})
    get_filename_component(ABS_FIL ${FIL} ABSOLUTE)
    get_filename_component(DIR ${ABS_FIL} DIRECTORY)
    get_filename_component(FIL_WE ${FIL} NAME_WE)
    set(GENERATED_PB_CC "${DIR}/${FIL_WE}.pb.cc")
    set(GENERATED_PB_H "${DIR}/${FIL_WE}.pb.h")
# cmake > 3.20 required :(
#    cmake_path(SET GENERATED_PB_CC "${DIR}")
#    cmake_path(SET GENERATED_PB_H "${DIR}")
#    cmake_path(APPEND GENERATED_PB_CC "${FIL_WE}.pb.cc")
#    cmake_path(APPEND GENERATED_PB_H "${FIL_WE}.pb.h")

    list(APPEND ${SRCS} ${GENERATED_PB_CC})
    list(APPEND ${HDRS} ${GENERATED_PB_H})
    add_custom_command(
      OUTPUT ${GENERATED_PB_CC}
             ${GENERATED_PB_H}
      COMMAND  ${PROTOBUF_PROTOC_EXECUTABLE}
      ARGS -I=${CMAKE_SOURCE_DIR}/aclk/aclk-schemas --cpp_out=${CMAKE_SOURCE_DIR}/aclk/aclk-schemas ${ABS_FIL}
      DEPENDS ${ABS_FIL} ${PROTOBUF_PROTOC_EXECUTABLE}
      COMMENT "Running C++ protocol buffer compiler on ${FIL}"
      VERBATIM )
  endforeach()
  set_source_files_properties(${${SRCS}} ${${HDRS}} PROPERTIES GENERATED TRUE)
  set(${SRCS} ${${SRCS}} PARENT_SCOPE)
  set(${HDRS} ${${HDRS}} PARENT_SCOPE)
endfunction()

set(ACLK_PROTO_DEFS
        aclk/aclk-schemas/proto/aclk/v1/lib.proto
        aclk/aclk-schemas/proto/agent/v1/disconnect.proto
        aclk/aclk-schemas/proto/agent/v1/connection.proto
        aclk/aclk-schemas/proto/alarm/v1/config.proto
        aclk/aclk-schemas/proto/alarm/v1/stream.proto
        aclk/aclk-schemas/proto/nodeinstance/connection/v1/connection.proto
        aclk/aclk-schemas/proto/nodeinstance/create/v1/creation.proto
        aclk/aclk-schemas/proto/nodeinstance/info/v1/info.proto
        aclk/aclk-schemas/proto/context/v1/context.proto
        aclk/aclk-schemas/proto/context/v1/stream.proto
        aclk/aclk-schemas/proto/agent/v1/cmds.proto
        )
PROTOBUF_ACLK_GENERATE_CPP(ACLK_PROTO_BUILT_SRCS ACLK_PROTO_BUILT_HDRS ${ACLK_PROTO_DEFS})

list(APPEND NETDATA_COMMON_LIBRARIES ${PROTOBUF_LIBRARIES})
list(APPEND NETDATA_COMMON_INCLUDE_DIRS ${PROTOBUF_INCLUDE_DIRS})
list(APPEND NETDATA_COMMON_CFLAGS ${PROTOBUF_CFLAGS_OTHER})
list(APPEND NETDATA_FILES ${ACLK_FILES} ${ACLK_PROTO_BUILT_SRCS} ${ACLK_PROTO_BUILT_HDRS})
include_directories(BEFORE ${CMAKE_SOURCE_DIR}/aclk/aclk-schemas)
include_directories(BEFORE ${CMAKE_SOURCE_DIR}/mqtt_websockets/src/include)
include_directories(BEFORE ${CMAKE_SOURCE_DIR}/mqtt_websockets/c-rbuf/include)

ADD_LIBRARY(mqttwebsockets STATIC
            ${MQTT_WEBSOCKETS_FILES})

target_compile_options(mqttwebsockets PUBLIC
                        -DMQTT_WSS_CUSTOM_ALLOC
                        -DRBUF_CUSTOM_MALLOC
                        -DMQTT_WSS_CPUSTATS)

target_include_directories(mqttwebsockets PUBLIC
             ${CMAKE_SOURCE_DIR}/aclk/helpers)

set(NETDATA_COMMON_LIBRARIES ${NETDATA_COMMON_LIBRARIES} mqttwebsockets)

ENDIF()

list(APPEND NETDATA_FILES ${ACLK_ALWAYS_BUILD})
list(APPEND NETDATA_FILES ${TIMEX_PLUGIN_FILES})

# -----------------------------------------------------------------------------
# netdata

IF(LINUX)
    list(APPEND NETDATA_FILES daemon/static_threads_linux.c)
    list(APPEND NETDATA_COMMON_LIBRARIES rt)

    add_executable(netdata ${GENERATED_CONFIG_H} ${NETDATA_FILES}
            ${CGROUPS_PLUGIN_FILES}
            ${DISKSPACE_PLUGIN_FILES}
            ${PROC_PLUGIN_FILES}
            ${TC_PLUGIN_FILES}
            )
    target_link_libraries (netdata libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(netdata PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_include_directories(netdata BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})
    target_compile_options(netdata PUBLIC ${NETDATA_COMMON_CFLAGS})

    SET(ENABLE_PLUGIN_CGROUP_NETWORK True)
    SET(ENABLE_PLUGIN_APPS True)
    SET(ENABLE_PLUGIN_PERF True)
    SET(ENABLE_PLUGIN_SLABINFO True)

ELSEIF(FREEBSD)
    list(APPEND NETDATA_FILES daemon/static_threads_freebsd.c)

    add_executable(netdata ${GENERATED_CONFIG_H} ${NETDATA_FILES} ${FREEBSD_PLUGIN_FILES})
    target_link_libraries (netdata libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(netdata PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_include_directories(netdata BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})
    target_compile_options(netdata PUBLIC ${NETDATA_COMMON_CFLAGS})
    SET(ENABLE_PLUGIN_CGROUP_NETWORK False)
    SET(ENABLE_PLUGIN_APPS True)
    SET(ENABLE_PLUGIN_PERF False)
    SET(ENABLE_PLUGIN_SLABINFO False)
    SET(ENABLE_PLUGIN_EBPF False)

ELSEIF(MACOS)
    list(APPEND NETDATA_FILES daemon/static_threads_macos.c)

    add_executable(netdata ${GENERATED_CONFIG_H} ${NETDATA_FILES} ${MACOS_PLUGIN_FILES})
    target_link_libraries (netdata libnetdata ${NETDATA_COMMON_LIBRARIES} ${IOKIT} ${FOUNDATION})
    target_include_directories(netdata PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_include_directories(netdata BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})
    target_compile_options(netdata PUBLIC ${NETDATA_COMMON_CFLAGS})
    SET(ENABLE_PLUGIN_CGROUP_NETWORK False)
    SET(ENABLE_PLUGIN_APPS False)
    SET(ENABLE_PLUGIN_PERF False)
    SET(ENABLE_PLUGIN_SLABINFO False)
    SET(ENABLE_PLUGIN_EBPF False)

ENDIF()

IF(ENABLE_EXPORTING_KINESIS OR ENABLE_EXPORTING_PUBSUB OR ENABLE_EXPORTING_PROMETHEUS_REMOTE_WRITE)
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

IF(CUPS_LIBRARIES)
    SET(ENABLE_PLUGIN_CUPS True)
ELSE()
    SET(ENABLE_PLUGIN_CUPS False)
ENDIF()


# -----------------------------------------------------------------------------
# netdatacli

add_executable(netdatacli ${GENERATED_CONFIG_H} ${NETDATACLI_FILES})
target_link_libraries (netdatacli libnetdata ${NETDATA_COMMON_LIBRARIES})
target_include_directories(netdatacli PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
target_include_directories(netdatacli BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})
target_compile_options(netdatacli PUBLIC ${NETDATA_COMMON_CFLAGS})


# -----------------------------------------------------------------------------
# debugfs.plugin

IF(ENABLE_PLUGIN_DEBUGFS)
    message(STATUS "debugfs.plugin: enabled")
    add_executable(debugfs.plugin ${GENERATED_CONFIG_H} ${DEBUGFS_PLUGIN_FILES})
    target_link_libraries (debugfs.plugin libnetdata ${NETDATA_COMMON_LIBRARIES} ${CAP_LIBRARIES})
    target_include_directories(debugfs.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS} ${CAP_INCLUDE_DIRS})
    target_include_directories(debugfs.plugin BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})
    target_compile_options(debugfs.plugin PUBLIC ${NETDATA_COMMON_CFLAGS} ${CAP_CFLAGS_OTHER})
ELSE()
    message(STATUS "debugfs.plugin: disabled")
ENDIF()


# -----------------------------------------------------------------------------
# apps.plugin

IF(ENABLE_PLUGIN_APPS)
    message(STATUS "apps.plugin: enabled")
    add_executable(apps.plugin ${GENERATED_CONFIG_H} ${APPS_PLUGIN_FILES})
    target_link_libraries (apps.plugin libnetdata ${NETDATA_COMMON_LIBRARIES} ${CAP_LIBRARIES})
    target_include_directories(apps.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS} ${CAP_INCLUDE_DIRS})
    target_include_directories(apps.plugin BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})
    target_compile_options(apps.plugin PUBLIC ${NETDATA_COMMON_CFLAGS} ${CAP_CFLAGS_OTHER})
ELSE()
    message(STATUS "apps.plugin: disabled")
ENDIF()


# -----------------------------------------------------------------------------
# freeipmi.plugin

IF(ENABLE_PLUGIN_FREEIPMI)
    message(STATUS "freeipmi.plugin: enabled")
    add_executable(freeipmi.plugin ${GENERATED_CONFIG_H} ${FREEIPMI_PLUGIN_FILES})
    target_link_libraries (freeipmi.plugin libnetdata ${NETDATA_COMMON_LIBRARIES} ${IPMI_LIBRARIES})
    target_include_directories(freeipmi.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS} ${IPMI_INCLUDE_DIRS})
    target_include_directories(freeipmi.plugin BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})
    target_compile_options(freeipmi.plugin PUBLIC ${NETDATA_COMMON_CFLAGS} ${IPMI_CFLAGS_OTHER})
ELSE()
    message(STATUS "freeipmi.plugin: disabled (depends on libipmimonitoring)")
ENDIF()


# -----------------------------------------------------------------------------
# nfacct.plugin

IF(ENABLE_PLUGIN_NFACCT)
    message(STATUS "nfacct.plugin: enabled")
    add_executable(nfacct.plugin ${GENERATED_CONFIG_H} ${NFACCT_PLUGIN_FILES})
    target_link_libraries (nfacct.plugin libnetdata ${NETDATA_COMMON_LIBRARIES} ${MNL_LIBRARIES} ${NFACCT_LIBRARIES})
    target_include_directories(nfacct.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS} ${MNL_INCLUDE_DIRS} ${NFACCT_INCLUDE_DIRS})
    target_include_directories(nfacct.plugin BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})
    target_compile_options(nfacct.plugin PUBLIC ${NETDATA_COMMON_CFLAGS} ${MNL_CFLAGS_OTHER} ${NFACCT_CFLAGS_OTHER})
ELSE()
    message(STATUS "nfacct.plugin: disabled (requires libmnl and libnetfilter_acct)")
ENDIF()


# -----------------------------------------------------------------------------
# xenstat.plugin

IF(ENABLE_PLUGIN_XENSTAT)
    message(STATUS "xenstat.plugin: enabled")
    add_executable(xenstat.plugin ${GENERATED_CONFIG_H} ${XENSTAT_PLUGIN_FILES})
    target_link_libraries (xenstat.plugin libnetdata ${NETDATA_COMMON_LIBRARIES} ${XENSTAT_LIBRARIES})
    target_include_directories(xenstat.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS} ${XENSTAT_INCLUDE_DIRS})
    target_include_directories(xenstat.plugin BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})
    target_compile_options(xenstat.plugin PUBLIC ${NETDATA_COMMON_CFLAGS} ${XENSTAT_CFLAGS_OTHER})
ELSE()
    message(STATUS "xenstat.plugin: disabled (requires libxenstat)")
ENDIF()


# -----------------------------------------------------------------------------
# perf.plugin

IF(ENABLE_PLUGIN_PERF)
    message(STATUS "perf.plugin: enabled")
    add_executable(perf.plugin ${GENERATED_CONFIG_H} ${PERF_PLUGIN_FILES})
    target_link_libraries (perf.plugin libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(perf.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_include_directories(perf.plugin BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})
    target_compile_options(perf.plugin PUBLIC ${NETDATA_COMMON_CFLAGS})
ELSE()
    message(STATUS "perf.plugin: disabled")
ENDIF()


# -----------------------------------------------------------------------------
# ebpf_process.plugin

IF(ENABLE_PLUGIN_EBPF)
    message(STATUS "ebpf.plugin: enabled")
    add_executable(ebpf.plugin ${GENERATED_CONFIG_H} ${EBPF_PROCESS_PLUGIN_FILES})
    target_link_libraries (ebpf.plugin libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(ebpf.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_include_directories(ebpf.plugin BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})
    target_compile_options(ebpf.plugin PUBLIC ${NETDATA_COMMON_CFLAGS})
ELSE()
    message(STATUS "ebpf.plugin: disabled")
ENDIF()


# -----------------------------------------------------------------------------
# slabinfo.plugin

IF(ENABLE_PLUGIN_SLABINFO)
    message(STATUS "slabinfo.plugin: enabled")
    add_executable(slabinfo.plugin ${GENERATED_CONFIG_H} ${SLABINFO_PLUGIN_FILES})
    target_link_libraries (slabinfo.plugin libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(slabinfo.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_include_directories(slabinfo.plugin BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})
    target_compile_options(slabinfo.plugin PUBLIC ${NETDATA_COMMON_CFLAGS})
ELSE()
    message(STATUS "slabinfo.plugin: disabled")
ENDIF()


# -----------------------------------------------------------------------------
# cups.plugin

IF(ENABLE_PLUGIN_CUPS)
    message(STATUS "cups.plugin: enabled")
    add_executable(cups.plugin ${GENERATED_CONFIG_H} ${CUPS_PLUGIN_FILES})
    target_link_libraries (cups.plugin libnetdata ${NETDATA_COMMON_LIBRARIES} ${CUPS_LIBRARIES})
    target_include_directories(cups.plugin PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS} ${CUPS_INCLUDE_DIRS})
    target_include_directories(cups.plugin BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})
    target_compile_options(cups.plugin PUBLIC ${NETDATA_COMMON_CFLAGS} ${CUPS_CFLAGS_OTHER})
ELSE()
    message(STATUS "cups.plugin: disabled")
ENDIF()


# -----------------------------------------------------------------------------
# cgroup-network

IF(ENABLE_PLUGIN_CGROUP_NETWORK)
    message(STATUS "cgroup-network: enabled")
    add_executable(cgroup-network ${GENERATED_CONFIG_H} ${CGROUP_NETWORK_FILES})
    target_link_libraries (cgroup-network libnetdata ${NETDATA_COMMON_LIBRARIES})
    target_include_directories(cgroup-network PUBLIC ${NETDATA_COMMON_INCLUDE_DIRS})
    target_include_directories(cgroup-network BEFORE PUBLIC ${GENERATED_CONFIG_H_DIR})
    target_compile_options(cgroup-network PUBLIC ${NETDATA_COMMON_CFLAGS})
ELSE()
    message(STATUS "cgroup-network: disabled (requires Linux)")
ENDIF()

# -----------------------------------------------------------------------------
# Unit tests

if(UNIT_TESTING)
    message(STATUS "Looking for CMocka which is required for unit testing")
    find_package(CMocka REQUIRED)
    include(CTest)

if(BUILD_TESTING)
    add_executable(str2ld_testdriver libnetdata/tests/test_str2ld.c)
    target_link_libraries(str2ld_testdriver libnetdata ${NETDATA_COMMON_LIBRARIES} ${CMOCKA_LIBRARIES})
    add_test(NAME test_str2ld COMMAND str2ld_testdriver)

    add_executable(storage_number_testdriver libnetdata/storage_number/tests/test_storage_number.c)
    target_link_libraries(storage_number_testdriver libnetdata ${NETDATA_COMMON_LIBRARIES} ${CMOCKA_LIBRARIES})
    add_test(NAME test_storage_number COMMAND storage_number_testdriver)

    set(EXPORTING_ENGINE_TEST_FILES
        exporting/tests/test_exporting_engine.c
        exporting/tests/test_exporting_engine.h
        exporting/tests/exporting_fixtures.c
        exporting/tests/exporting_doubles.c
        exporting/tests/netdata_doubles.c
        exporting/tests/system_doubles.c
        database/rrdlabels.c
        database/rrdvar.c
        )
    set(TEST_NAME exporting_engine)
    set(PROMETHEUS_REMOTE_WRITE_LINK_OPTIONS)
    set(KINESIS_LINK_OPTIONS)
    set(PUBSUB_LINK_OPTIONS)
    set(MONGODB_LINK_OPTIONS)
if(ENABLE_EXPORTING_PROMETHEUS_REMOTE_WRITE)
    list(APPEND EXPORTING_ENGINE_FILES ${PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES} ${PROTO_SRCS} ${PROTO_HDRS})
    list(
        APPEND PROMETHEUS_REMOTE_WRITE_LINK_OPTIONS
        -Wl,--wrap=init_write_request
        -Wl,--wrap=add_host_info
        -Wl,--wrap=add_label
        -Wl,--wrap=add_metric
    )
endif()
if(ENABLE_EXPORTING_KINESIS)
    list(APPEND EXPORTING_ENGINE_FILES ${KINESIS_EXPORTING_FILES})
    list(
        APPEND KINESIS_LINK_OPTIONS
        -Wl,--wrap=aws_sdk_init
        -Wl,--wrap=kinesis_init
        -Wl,--wrap=kinesis_put_record
        -Wl,--wrap=kinesis_get_result
    )
endif()
if(ENABLE_EXPORTING_PUBSUB)
    list(APPEND EXPORTING_ENGINE_FILES ${PUBSUB_EXPORTING_FILES})
    list(
        APPEND PUBSUB_LINK_OPTIONS
        -Wl,--wrap=pubsub_init
        -Wl,--wrap=pubsub_add_message
        -Wl,--wrap=pubsub_publish
        -Wl,--wrap=pubsub_get_result
    )
endif()
if(MONGOC_LIBRARIES)
    list(APPEND EXPORTING_ENGINE_FILES ${MONGODB_EXPORTING_FILES})
    list(
        APPEND MONGODB_LINK_OPTIONS
        -Wl,--wrap=mongoc_init
        -Wl,--wrap=mongoc_uri_new_with_error
        -Wl,--wrap=mongoc_uri_get_option_as_int32
        -Wl,--wrap=mongoc_uri_set_option_as_int32
        -Wl,--wrap=mongoc_client_new_from_uri
        -Wl,--wrap=mongoc_client_set_appname
        -Wl,--wrap=mongoc_client_get_collection
        -Wl,--wrap=mongoc_uri_destroy
        -Wl,--wrap=mongoc_collection_insert_many
    )
endif()
    add_executable(${TEST_NAME}_testdriver ${EXPORTING_ENGINE_TEST_FILES} ${EXPORTING_ENGINE_FILES})
    target_compile_options(
        ${TEST_NAME}_testdriver
        PRIVATE
        -DUNIT_TESTING
    )
    target_include_directories(
        ${TEST_NAME}_testdriver
        PUBLIC
        ${CMAKE_CURRENT_BINARY_DIR}
        ${NETDATA_COMMON_INCLUDE_DIRS}
    )
    target_link_options(
        ${TEST_NAME}_testdriver
        PRIVATE
        -Wl,--wrap=read_exporting_config
        -Wl,--wrap=init_connectors
        -Wl,--wrap=mark_scheduled_instances
        -Wl,--wrap=rrdhost_is_exportable
        -Wl,--wrap=rrdset_is_exportable
        -Wl,--wrap=exporting_calculate_value_from_stored_data
        -Wl,--wrap=prepare_buffers
        -Wl,--wrap=send_internal_metrics
        -Wl,--wrap=now_realtime_sec
        -Wl,--wrap=uv_thread_set_name_np
        -Wl,--wrap=uv_thread_create
        -Wl,--wrap=uv_mutex_lock
        -Wl,--wrap=uv_mutex_unlock
        -Wl,--wrap=uv_cond_signal
        -Wl,--wrap=uv_cond_wait
        -Wl,--wrap=strdupz
        -Wl,--wrap=info_int
        -Wl,--wrap=recv
        -Wl,--wrap=send
        -Wl,--wrap=connect_to_one_of
        -Wl,--wrap=create_main_rusage_chart
        -Wl,--wrap=send_main_rusage
        -Wl,--wrap=simple_connector_end_batch
        ${PROMETHEUS_REMOTE_WRITE_LINK_OPTIONS}
        ${KINESIS_LINK_OPTIONS}
        ${PUBSUB_LINK_OPTIONS}
        ${MONGODB_LINK_OPTIONS}
    )
    target_link_libraries(${TEST_NAME}_testdriver libnetdata ${NETDATA_COMMON_LIBRARIES} ${CMOCKA_LIBRARIES})
    add_test(NAME test_${TEST_NAME} COMMAND ${TEST_NAME}_testdriver)

    set(WEB_API_TEST_FILES
        web/api/tests/web_api.c
        web/server/web_client.c
        )
    add_executable(web_api_testdriver ${WEB_API_TEST_FILES})
    target_link_options(
        web_api_testdriver
        PRIVATE
        -Wl,--wrap=rrdhost_find_by_hostname
        -Wl,--wrap=finished_web_request_statistics
        -Wl,--wrap=config_get
        -Wl,--wrap=web_client_api_request_v1
        -Wl,--wrap=rrdhost_find_by_guid
        -Wl,--wrap=rrdset_find_byname
        -Wl,--wrap=sql_create_host_by_uuid
        -Wl,--wrap=rrdset_find
        -Wl,--wrap=rrdpush_receiver_thread_spawn
        -Wl,--wrap=debug_int
        -Wl,--wrap=error_int
        -Wl,--wrap=info_int
        -Wl,--wrap=fatal_int
    )
    target_link_libraries(web_api_testdriver libnetdata ${NETDATA_COMMON_LIBRARIES} ${CMOCKA_LIBRARIES})
#    add_test(NAME test_web_api COMMAND web_api_testdriver)

    set(VALID_URLS_TEST_FILES
        web/api/tests/valid_urls.c
        web/server/web_client.c
        )
    add_executable(valid_urls_testdriver ${VALID_URLS_TEST_FILES})
    target_link_options(
        valid_urls_testdriver
        PRIVATE
        -Wl,--wrap=rrdhost_find_by_hostname
        -Wl,--wrap=finished_web_request_statistics
        -Wl,--wrap=config_get
        -Wl,--wrap=web_client_api_request_v1
        -Wl,--wrap=rrdhost_find_by_guid
        -Wl,--wrap=rrdset_find_byname
        -Wl,--wrap=sql_create_host_by_uuid
        -Wl,--wrap=rrdset_find
        -Wl,--wrap=rrdpush_receiver_thread_spawn
        -Wl,--wrap=debug_int
        -Wl,--wrap=error_int
        -Wl,--wrap=info_int
        -Wl,--wrap=fatal_int
        -Wl,--wrap=mysendfile
        -DREMOVE_MYSENDFILE
    )
    target_link_libraries(valid_urls_testdriver libnetdata ${NETDATA_COMMON_LIBRARIES} ${CMOCKA_LIBRARIES})
#    add_test(NAME test_valid_urls COMMAND valid_urls_testdriver)

    set(CGROUPS_TEST_FILES
        collectors/cgroups.plugin/tests/test_cgroups_plugin.c
        collectors/cgroups.plugin/tests/test_cgroups_plugin.h
        collectors/cgroups.plugin/tests/test_doubles.c
        database/rrdlabels.c
        database/rrd.h
    )
    add_executable(cgroups_testdriver ${CGROUPS_TEST_FILES} ${CGROUPS_PLUGIN_FILES})
    target_link_libraries(cgroups_testdriver libnetdata ${NETDATA_COMMON_LIBRARIES} ${CMOCKA_LIBRARIES})
    add_test(NAME test_cgroups COMMAND cgroups_testdriver)

    set_target_properties(
        str2ld_testdriver
        storage_number_testdriver
        exporting_engine_testdriver
        web_api_testdriver
        valid_urls_testdriver
        cgroups_testdriver
        PROPERTIES RUNTIME_OUTPUT_DIRECTORY tests
    )

endif()
endif()


# generate config.h so that CMake becomes independent of automake

## netdata version
set(GIT_EXECUTABLE "git")
execute_process(COMMAND ${GIT_EXECUTABLE} describe OUTPUT_VARIABLE NETDATA_VERSION OUTPUT_STRIP_TRAILING_WHITESPACE)

IF("${NETDATA_VERSION}" STREQUAL "")
    file(STRINGS "packaging/version" NETDATA_VERSION LIMIT_COUNT 1)
ENDIF()

set(NETDATA_USER "netdata" CACHE STRING "use this user to drop privileges")

if(MAC_OS)
    set(ENABLE_APPS_PLUGIN False)
else()
    set(ENABLE_APPS_PLUGIN True)
endif()

check_include_file(time.h HAVE_TIME_H)
check_include_file(unistd.h HAVE_UNISTD_H)
if (HAVE_TIME_H)
    include(CheckStructHasMember)
    check_struct_has_member("struct timespec" tv_sec "time.h" HAVE_STRUCT_TIMESPEC)
endif ()

check_include_file(sys/statfs.h HAVE_SYS_STATFS_H)
check_include_file(sys/statvfs.h HAVE_SYS_STATVFS_H)
check_include_file(inttypes.h HAVE_INTTYPES_H)
check_include_file(stdint.h HAVE_STDINT_H)

include(CheckSymbolExists)
check_include_file(sys/mkdev.h HAVE_SYS_MKDEV_H)
if (HAVE_SYS_MKDEV_H)
    check_symbol_exists(major "sys/mkdev.h" MAJOR_IN_MKDEV)
endif()
check_include_file(sys/sysmacros.h HAVE_SYS_SYSMACROS_H)
if (HAVE_SYS_SYSMACROS_H)
    check_symbol_exists(major "sys/sysmacros.h" MAJOR_IN_SYSMACROS)
endif()

if (CRYPTO_FOUND)
    set(HAVE_CRYPTO True)
    FIND_LIBRARY(CRYPTO_LIBRARY_LOCATION NAMES crypto)
    check_library_exists(crypto X509_VERIFY_PARAM_set1_host ${CRYPTO_LIBRARY_LOCATION} HAVE_X509_VERIFY_PARAM_set1_host)
    if (HAVE_X509_VERIFY_PARAM_set1_host)
        set(HAVE_X509_VERIFY_PARAM_set1_host True)
    endif()
endif()

include(CheckCSourceCompiles)
check_c_source_compiles("
  #define _GNU_SOURCE
  #include <string.h>
  int main() { char x = *strerror_r(0, &x, sizeof(x)); return 0; }
  " STRERROR_R_CHAR_P)

IF(${CMAKE_C_COMPILER_ID} STREQUAL "GNU")
    SET(LIKELY_MACRO "__builtin_expect(!!(x), 1)")
    SET(UNLIKELY_MACRO "__builtin_expect(!!(x), 0)")
ELSE()
    SET(LIKELY_MACRO "(x)")
    SET(UNLIKELY_MACRO "(x)")
ENDIF()

IF(${CMAKE_C_COMPILER_ID} STREQUAL "GNU")
    SET(ALWAYS_UNUSED_MACRO "__attribute__((unused))")
    SET(MAYBE_UNUSED_MACRO "__attribute__((unused))")
ELSE()
    SET(ALWAYS_UNUSED_MACRO "")
    SET(MAYBE_UNUSED_MACRO "")
ENDIF()

# -----------------------------------------------------------------------------

check_library_exists(c clock_gettime "" HAVE_CLOCK_GETTIME)

IF(NOT HAVE_CLOCK_GETTIME)
    CHECK_LIBRARY_EXISTS(rt clock_gettime "" HAVE_CLOCK_GETTIME)
ENDIF()

IF(HAVE_CLOCK_GETTIME)
    message("-- clock_gettime(): found")
    set(NETDATA_REQUIRED_DEFINES "-DHAVE_CLOCK_GETTIME=1 ${NETDATA_REQUIRED_DEFINES}")
ELSE()
    message("-- clock_gettime(): not found")
ENDIF()

# -----------------------------------------------------------------------------
set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} ${NETDATA_REQUIRED_DEFINES}")
message("CFLAGS=\"${CMAKE_C_FLAGS}\"")

configure_file(config.cmake.h.in config.h)
