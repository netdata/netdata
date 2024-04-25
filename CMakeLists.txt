# SPDX-License-Identifier: GPL-3.0-or-later

cmake_minimum_required(VERSION 3.13.0)

if(POLICY CMP0135)
    cmake_policy(SET CMP0135 NEW)
endif()

#
# version atrocities
#

find_package(Git)

if(GIT_EXECUTABLE)
        execute_process(COMMAND ${GIT_EXECUTABLE} describe
                        WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
                        RESULT_VARIABLE GIT_DESCRIBE_RESULT
                        OUTPUT_VARIABLE GIT_DESCRIBE_OUTPUT)
        if(GIT_DESCRIBE_RESULT)
                file(STRINGS packaging/version GIT_DESCRIBE_OUTPUT)
                message(WARNING "using version from packaging/version: '${GIT_DESCRIBE_OUTPUT}'")
        endif()
else()
        file(STRINGS packaging/version GIT_DESCRIBE_OUTPUT)
        message(WARNING "using version from packaging/version: '${GIT_DESCRIBE_OUTPUT}'")
endif()

string(STRIP ${GIT_DESCRIBE_OUTPUT} GIT_DESCRIBE_OUTPUT)
string(REGEX MATCH "v?([0-9]+)\\.([0-9]+)\\.([0-9]+)-?([0-9]+)?-?([0-9a-zA-Z]+)?" MATCHES "${GIT_DESCRIBE_OUTPUT}")

if(CMAKE_MATCH_COUNT EQUAL 3)
        set(FIELD_MAJOR ${CMAKE_MATCH_1})
        set(FIELD_MINOR ${CMAKE_MATCH_2})
        set(FIELD_PATCH ${CMAKE_MATCH_3})
        set(FIELD_TWEAK 0)
        set(FIELD_DESCR "N/A")
elseif(CMAKE_MATCH_COUNT EQUAL 4)
        set(FIELD_MAJOR ${CMAKE_MATCH_1})
        set(FIELD_MINOR ${CMAKE_MATCH_2})
        set(FIELD_PATCH ${CMAKE_MATCH_3})
        set(FIELD_TWEAK ${CMAKE_MATCH_4})
        set(FIELD_DESCR "N/A")
elseif(CMAKE_MATCH_COUNT EQUAL 5)
        set(FIELD_MAJOR ${CMAKE_MATCH_1})
        set(FIELD_MINOR ${CMAKE_MATCH_2})
        set(FIELD_PATCH ${CMAKE_MATCH_3})
        set(FIELD_TWEAK ${CMAKE_MATCH_4})
        set(FIELD_DESCR ${CMAKE_MATCH_5})
else()
        message(FATAL_ERROR "Wrong version regex match count ${CMAKE_MATCH_COUNT} (should be in 3, 4 or 5)")
endif()

set(NETDATA_VERSION ${FIELD_MAJOR}.${FIELD_MINOR}.${FIELD_PATCH}.${FIELD_TWEAK})

#
# project
#

project(netdata
        VERSION ${NETDATA_VERSION}
        DESCRIPTION "Netdata real-time monitoring"
        HOMEPAGE_URL "https://www.netdata.cloud"
        LANGUAGES C CXX)
list(APPEND CMAKE_MODULE_PATH "${PROJECT_SOURCE_DIR}/packaging/cmake/Modules")
include(CMakeDependentOption)

find_package(PkgConfig REQUIRED)

set(CMAKE_C_STANDARD 11)
set(CMAKE_CXX_STANDARD 14)

option(USE_CXX_11 "Use C++11 instead of C++14 (should only be used on legacy systems that cannot support C++14, may disable some features)" False)
mark_as_advanced(USE_CXX_11)

if(USE_CXX_11)
    set(CMAKE_CXX_STANDARD 11)
endif()

set(CMAKE_C_STANDARD_REQUIRED On)
set(CMAKE_CXX_STANDARD_REQUIRED On)

set(SIZEOF_VOID_P ${CMAKE_SIZEOF_VOID_P})

if(NOT CMAKE_BUILD_TYPE)
        set(CMAKE_BUILD_TYPE "Release")
endif()

option(ENABLE_ADDRESS_SANITIZER "Build with address sanitizer enabled" False)
mark_as_advanced(ENABLE_ADDRESS_SANITIZER)

if(ENABLE_ADDRESS_SANITIZER)
        set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -fsanitize=address")
endif()

set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -fexceptions")
set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} ${CMAKE_C_FLAGS}")

set(CMAKE_EXPORT_COMPILE_COMMANDS On)

set(CONFIG_H_DIR ${CMAKE_BINARY_DIR})
set(CONFIG_H ${CONFIG_H_DIR}/config.h)

# This is intended to make life easier for developers who are working on one
# specific feature.
option(DEFAULT_FEATURE_STATE "Specify the default state for most optional features" True)
mark_as_advanced(DEFAULT_FEATURE_STATE)

# High-level features
option(ENABLE_ACLK "Enable Netdata Cloud support (ACLK)" ${DEFAULT_FEATURE_STATE})
cmake_dependent_option(ENABLE_CLOUD "Enable Netdata Cloud by default at runtime" True "NOT ENABLE_ACLK" False)
mark_as_advanced(ENABLE_CLOUD)
option(ENABLE_ML "Enable machine learning features" ${DEFAULT_FEATURE_STATE})
option(ENABLE_DBENGINE "Enable dbengine metrics storage" True)

# Data collection plugins
option(ENABLE_PLUGIN_APPS "Enable per-process resource usage monitoring" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_CGROUP_NETWORK "Enable Linux CGroup network usage monitoring" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_CUPS "Enable CUPS monitoring" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_DEBUGFS "Enable Linux DebugFS metric collection" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_EBPF "Enable Linux eBPF metric collection" ${DEFAULT_FEATURE_STATE})
cmake_dependent_option(ENABLE_LEGACY_EBPF_PROGRAMS "Enable eBPF programs for kernels without BTF support" True ENABLE_PLUGIN_EBPF False)
mark_as_advanced(ENABLE_LEGACY_EBPF_PROGRAMS)
option(ENABLE_PLUGIN_FREEIPMI "Enable IPMI monitoring" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_GO "Enable metric collectors written in Go" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_LOCAL_LISTENERS "Enable local listening socket tracking (including service auto-discovery support)" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_LOGS_MANAGEMENT "Enable log collection and monitoring based on Fluent Bit" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_NETWORK_VIEWER "Enable network viewer functionality" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_NFACCT "Enable Linux NFACCT metric collection" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_PERF "Enable Linux performance counter monitoring" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_SLABINFO "Enable Linux kernel SLAB allocator monitoring" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_SYSTEMD_JOURNAL "Enable systemd journal log collection" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_XENSTAT "Enable Xen domain monitoring" ${DEFAULT_FEATURE_STATE})

# Metrics exporters
option(ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE "Enable exporting to Prometheus via remote write API" ${DEFAULT_FEATURE_STATE})
option(ENABLE_EXPORTER_MONGODB "Enable exporting to MongoDB" ${DEFAULT_FEATURE_STATE})

# Vendoring
option(ENABLE_BUNDLED_JSONC "Force use of a vendored copy of JSON-C" False)
option(ENABLE_BUNDLED_YAML "Force use of a vendored copy of libyaml" False)
option(ENABLE_BUNDLED_PROTOBUF "Use a vendored copy of protobuf" False)

# Optional test code
cmake_dependent_option(ENABLE_LOGS_MANAGEMENT_TESTS "Enable test code for logs-management plugin." True "NOT ENABLE_PLUGIN_LOGS_MANAGEMENT" False)
mark_as_advanced(ENABLE_LOGS_MANAGEMENT_TESTS)

# Experimental features
option(ENABLE_WEBRTC "Enable WebRTC dashboard communications (experimental)" False)
mark_as_advanced(ENABLE_WEBRTC)
option(ENABLE_H2O "Enable H2O web server (experimental)" True)
mark_as_advanced(ENABLE_H2O)

# Other optional functionality
option(ENABLE_SENTRY "Build with Sentry Native crash reporting" False)
mark_as_advanced(ENABLE_SENTRY)

cmake_dependent_option(FORCE_LEGACY_LIBBPF "Force usage of libbpf 0.0.9 instead of the latest version." False "ENABLE_PLUGIN_LIBBPF" False)
mark_as_advanced(FORCE_LEGACY_LIBBPF)

if(ENABLE_ACLK OR ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE)
        set(NEED_PROTOBUF True)
else()
        set(NEED_PROTOBUF False)
endif()

if(ENABLE_PLUGIN_GO)
    include(NetdataGoTools)

    find_min_go_version("${CMAKE_SOURCE_DIR}/src/go")

    find_package(Go "${MIN_GO_VERSION}" REQUIRED)
endif()

if(ENABLE_WEBRTC)
        include(FetchContent)

        # ignore debhelper
        set(FETCHCONTENT_FULLY_DISCONNECTED Off)

        set(PREFER_SYSTEM_LIB True)
        set(NO_MEDIA True)
        set(NO_WEBSOCKET True)

        set(HAVE_LIBDATACHANNEL True)

        FetchContent_Declare(libdatachannel
            GIT_REPOSITORY https://github.com/paullouisageneau/libdatachannel.git
            GIT_TAG v0.20.1
        )
        FetchContent_MakeAvailable(libdatachannel)
endif()

if(NEED_PROTOBUF)
        include(NetdataProtobuf)

        if(ENABLE_BUNDLED_PROTOBUF)
                netdata_bundle_protobuf()
        endif()
endif()

#
# handling of extra compiler flags
#

include(NetdataCompilerFlags)

# Disable hardening for debug builds by default.
if(CMAKE_BUILD_TYPE STREQUAL "Debug")
        option(DISABLE_HARDENING "Disable adding extra compiler flags for hardening" TRUE)
else()
        option(DISABLE_HARDENING "Disable adding extra compiler flags for hardening" FALSE)
endif()

set(EXTRA_HARDENING_C_FLAGS "")
set(EXTRA_HARDENING_CXX_FLAGS "")

set(EXTRA_OPT_C_FLAGS "")
set(EXTRA_OPT_CXX_FLAGS "")

if(NOT ${DISABLE_HARDENING})
        add_double_extra_compiler_flag("stack-protector" "-fstack-protector-strong" "-fstack-protector" EXTRA_HARDENING)
        add_double_extra_compiler_flag("_FORTIFY_SOURCE" "-D_FORTIFY_SOURCE=3" "-D_FORTIFY_SOURCE=2" EXTRA_HARDENING)
        add_simple_extra_compiler_flag("stack-clash-protection" "-fstack-clash-protection" EXTRA_HARDENING)
        add_simple_extra_compiler_flag("-fcf-protection" "-fcf-protection=full" EXTRA_HARDENING)
        add_simple_extra_compiler_flag("branch-protection" "-mbranch-protection=standard" EXTRA_HARDENING)
endif()

foreach(FLAG function-sections data-sections)
        add_simple_extra_compiler_flag("${FLAG}" "-f${FLAG}" EXTRA_OPT)
endforeach()

add_simple_extra_compiler_flag("-Wbuiltin-macro-redefined" "-Wno-builtin-macro-redefined" EXTRA_OPT)

foreach(RELTYP RELEASE DEBUG RELWITHDEBINFO MINSIZEREL)
        foreach(L C CXX)
                set(CMAKE_${L}_FLAGS_${RELTYP} "${CMAKE_${L}_FLAGS_${RELTYP}} ${EXTRA_HARDENING_C_FLAGS} ${EXTRA_OPT_C_FLAGS}")
        endforeach()
endforeach()

#
# detect OS
#

set(LINUX   False)
set(FREEBSD False)
set(MACOS   False)

if(${CMAKE_SYSTEM_NAME} MATCHES "Darwin")
        set(MACOS True)
        set(COMPILED_FOR_MACOS True)

        find_library(IOKIT IOKit)
        find_library(FOUNDATION Foundation)
elseif(${CMAKE_SYSTEM_NAME} MATCHES "FreeBSD")
        set(FREEBSD True)
        set(COMPILED_FOR_FREEBSD True)
else()
        set(LINUX True)
        set(COMPILED_FOR_LINUX True)
        add_definitions(-D_GNU_SOURCE)
endif()

if(ENABLE_PLUGIN_EBPF)
        include(NetdataLibBPF)
        include(NetdataEBPFCORE)

        if(NOT LINUX)
            message(FATAL_ERROR "The eBPF plugin is not supported on non-Linux systems")
        endif()

        netdata_bundle_libbpf()
        netdata_fetch_ebpf_co_re()
endif()

#
# Libm
#

# checks link with cmake required libs
cmake_policy(SET CMP0075 NEW)

include(CheckFunctionExists)

check_function_exists(log10 HAVE_LOG10)
if(NOT HAVE_LOG10)
        unset(HAVE_LOG10 CACHE)
        list(APPEND CMAKE_REQUIRED_LIBRARIES m)
        check_function_exists(log10 HAVE_LOG10)
        if(HAVE_LOG10)
                set(LINK_LIBM True)
        else()
                message(FATAL_ERROR "Can not use log10 with/without libm.")
        endif()
endif()

#
# Custom Modules
#

include(NetdataJSONC)
include(NetdataYAML)

if(ENABLE_LEGACY_EBPF_PROGRAMS)
        include(NetdataEBPFLegacy)
endif()

if(ENABLE_SENTRY)
        include(NetdataSentry)
endif()

#
# Checks from custom modules
#

netdata_detect_jsonc()
netdata_detect_libyaml()

if(ENABLE_LEGACY_EBPF_PROGRAMS)
        netdata_fetch_legacy_ebpf_code()
endif()

if(ENABLE_SENTRY)
        netdata_bundle_sentry()
endif()

#
# check include files
#

include(CheckIncludeFile)

check_include_file("netinet/in.h" HAVE_NETINET_IN_H)
check_include_file("resolv.h" HAVE_RESOLV_H)
check_include_file("netdb.h" HAVE_NETDB_H)
check_include_file("sys/prctl.h" HAVE_SYS_PRCTL_H)
check_include_file("sys/stat.h" HAVE_SYS_STAT_H)
check_include_file("sys/vfs.h" HAVE_SYS_VFS_H)
check_include_file("sys/statfs.h" HAVE_SYS_STATFS_H)
check_include_file("linux/magic.h" HAVE_LINUX_MAGIC_H)
check_include_file("sys/mount.h" HAVE_SYS_MOUNT_H)
check_include_file("sys/statvfs.h" HAVE_SYS_STATVFS_H)
check_include_file("inttypes.h" HAVE_INTTYPES_H)
check_include_file("stdint.h" HAVE_STDINT_H)
check_include_file("sys/capability.h" HAVE_SYS_CAPABILITY_H)

#
# check symbols
#

include(CheckSymbolExists)
check_symbol_exists(major "sys/sysmacros.h" MAJOR_IN_SYSMACROS)
check_symbol_exists(major "sys/mkdev.h" MAJOR_IN_MKDEV)
check_symbol_exists(clock_gettime "time.h" HAVE_CLOCK_GETTIME)
check_symbol_exists(strerror_r "string.h" HAVE_STRERROR_R)
check_symbol_exists(finite "math.h" HAVE_FINITE)
check_symbol_exists(isfinite "math.h" HAVE_ISFINITE)
check_symbol_exists(dlsym "dlfcn.h" HAVE_DLSYM)

check_function_exists(nice HAVE_NICE)
check_function_exists(recvmmsg HAVE_RECVMMSG)
check_function_exists(getpriority HAVE_GETPRIORITY)

check_function_exists(sched_getscheduler HAVE_SCHED_GETSCHEDULER)
check_function_exists(sched_setscheduler HAVE_SCHED_SETSCHEDULER)
check_function_exists(sched_get_priority_min HAVE_SCHED_GET_PRIORITY_MIN)
check_function_exists(sched_get_priority_max HAVE_SCHED_GET_PRIORITY_MAX)

check_function_exists(close_range HAVE_CLOSE_RANGE)
check_function_exists(backtrace HAVE_BACKTRACE)

#
# check source compilation
#

include(CheckCSourceCompiles)
include(CheckCXXSourceCompiles)

set(CMAKE_REQUIRED_LIBRARIES pthread)
check_c_source_compiles("
#define _GNU_SOURCE
#include <pthread.h>
int main() {
        char name[16];
        pthread_t thread = pthread_self();
        return pthread_getname_np(thread, name, sizeof(name));
}
" HAVE_PTHREAD_GETNAME_NP)

check_c_source_compiles("
#include <stdio.h>
#define mytype(X) _Generic((X), int: 'i', float: 'f', default: 'u')
int main() {
        char type = mytype(0);
        return 0;
}
" HAVE_C__GENERIC)

check_c_source_compiles("
#include <malloc.h>
int main() {
        mallopt(M_ARENA_MAX, 1);
        mallopt(M_PERTURB, 0x5A);
        return 0;
}
" HAVE_C_MALLOPT)

check_c_source_compiles("
#define _GNU_SOURCE
#include <stdio.h>
#include <sys/socket.h>
int main() {
        accept4(0, NULL, NULL, 0);
        return 0;
}
" HAVE_ACCEPT4)

check_c_source_compiles("
#define _GNU_SOURCE
#include <string.h>
int main() {
        char x = *strerror_r(0, &x, sizeof(x)); return 0;
}
" STRERROR_R_CHAR_P)

check_c_source_compiles("
#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif
#include <sched.h>
int main() {
        setns(0, 0); return 0;
}
" HAVE_SETNS)

check_cxx_source_compiles("
int main() {
        __atomic_load_8(nullptr, 0);
        return 0;
}
" HAVE_BUILTIN_ATOMICS)

check_c_source_compiles("
void my_printf(char const *s, ...) __attribute__((format(printf, 1, 2)));
int main() { return 0; }
" HAVE_FUNC_ATTRIBUTE_FORMAT)

check_c_source_compiles("
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
void* my_alloc(size_t size) __attribute__((malloc));
int main() {
    void *x = my_alloc(1);
    free(x);
    return 0;
}
void* my_alloc(size_t size) {
    void *ret = malloc(size);
    if(!ret) exit(1);
    return ret;
}
" HAVE_FUNC_ATTRIBUTE_MALLOC)

check_c_source_compiles("
void my_function() __attribute__((noinline));
int main() { my_function(); return 0; }
void my_function() { ; }
" HAVE_FUNC_ATTRIBUTE_NOINLINE)

check_c_source_compiles("
void my_exit_function() __attribute__((noreturn));
int main() {
        my_exit_function(); // Call the noreturn function
        return 0;
}
void my_exit_function() {
        exit(1);
}
" HAVE_FUNC_ATTRIBUTE_NORETURN)

check_c_source_compiles("
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
void* my_alloc(size_t size) __attribute__((returns_nonnull));
int main() {
        void* ptr = my_alloc(10);
        free(ptr);
        return 0;
}
void* my_alloc(size_t size) {
        void *ret = malloc(size);
        if(!ret) exit(1);
        return ret;
}
" HAVE_FUNC_ATTRIBUTE_RETURNS_NONNULL)

check_c_source_compiles("
int my_function() __attribute__((warn_unused_result));
int main() {
        return my_function();
}
int my_function() {
        return 1;
}
" HAVE_FUNC_ATTRIBUTE_WARN_UNUSED_RESULT)

if(FREEBSD OR MACOS)
        set(HAVE_BUILTIN_ATOMICS True)
endif()

# openssl/crypto
set(ENABLE_OPENSSL True)
pkg_check_modules(OPENSSL openssl)

if(NOT OPENSSL_FOUND)
        if(MACOS)
                execute_process(COMMAND
                                brew --prefix --installed openssl
                                RESULT_VARIABLE BREW_OPENSSL
                                OUTPUT_VARIABLE BREW_OPENSSL_PREFIX
                                OUTPUT_STRIP_TRAILING_WHITESPACE)

                if((BREW_OPENSSL NOT EQUAL 0) OR (NOT EXISTS "${BREW_OPENSSL_PREFIX}"))
                        message(FATAL_ERROR "OpenSSL (or LibreSSL) is required for building Netdata, but could not be found.")
                endif()

                set(OPENSSL_INCLUDE_DIRS "${BREW_OPENSSL_PREFIX}/include")
                set(OPENSSL_CFLAGS_OTHER "")
                set(OPENSSL_LDFLAGS "-L${BREW_OPENSSL_PREFIX}/lib;-lssl;-lcrypto")
        else()
            message(FATAL_ERROR "OpenSSL (or LibreSSL) is required for building Netdata, but could not be found.")
        endif()
endif()

if(NOT MACOS)
        pkg_check_modules(CRYPTO libcrypto)
endif()

#
# figure out if we need protoc/protobuf
#

if(NEED_PROTOBUF)
        netdata_detect_protobuf()
endif()

#
# source files
#

set(LIBJUDY_PREV_FILES
        src/libnetdata/libjudy/src/JudyL/JudyLPrev.c
        src/libnetdata/libjudy/src/JudyL/JudyLPrevEmpty.c
)

set(LIBJUDY_NEXT_FILES
        src/libnetdata/libjudy/src/JudyL/JudyLNext.c
        src/libnetdata/libjudy/src/JudyL/JudyLNextEmpty.c
)

set(LIBJUDY_SOURCES
        src/libnetdata/libjudy/src/Judy.h
        src/libnetdata/libjudy/src/JudyCommon/JudyMalloc.c
        src/libnetdata/libjudy/src/JudyCommon/JudyPrivate.h
        src/libnetdata/libjudy/src/JudyCommon/JudyPrivate1L.h
        src/libnetdata/libjudy/src/JudyCommon/JudyPrivateBranch.h
        src/libnetdata/libjudy/src/JudyL/JudyL.h
        src/libnetdata/libjudy/src/JudyL/JudyLByCount.c
        src/libnetdata/libjudy/src/JudyL/JudyLCascade.c
        src/libnetdata/libjudy/src/JudyL/JudyLCount.c
        src/libnetdata/libjudy/src/JudyL/JudyLCreateBranch.c
        src/libnetdata/libjudy/src/JudyL/JudyLDecascade.c
        src/libnetdata/libjudy/src/JudyL/JudyLDel.c
        src/libnetdata/libjudy/src/JudyL/JudyLFirst.c
        src/libnetdata/libjudy/src/JudyL/JudyLFreeArray.c
        src/libnetdata/libjudy/src/JudyL/j__udyLGet.c
        src/libnetdata/libjudy/src/JudyL/JudyLGet.c
        src/libnetdata/libjudy/src/JudyL/JudyLInsArray.c
        src/libnetdata/libjudy/src/JudyL/JudyLIns.c
        src/libnetdata/libjudy/src/JudyL/JudyLInsertBranch.c
        src/libnetdata/libjudy/src/JudyL/JudyLMallocIF.c
        src/libnetdata/libjudy/src/JudyL/JudyLMemActive.c
        src/libnetdata/libjudy/src/JudyL/JudyLMemUsed.c
        src/libnetdata/libjudy/src/JudyL/JudyLTables.c
        src/libnetdata/libjudy/src/JudyHS/JudyHS.c
        ${LIBJUDY_PREV_FILES}
        ${LIBJUDY_NEXT_FILES}
)

set(LIBNETDATA_FILES
        ${CONFIG_H}
        src/libnetdata/adaptive_resortable_list/adaptive_resortable_list.c
        src/libnetdata/adaptive_resortable_list/adaptive_resortable_list.h
        src/libnetdata/config/appconfig.c
        src/libnetdata/config/appconfig.h
        src/libnetdata/aral/aral.c
        src/libnetdata/aral/aral.h
        src/libnetdata/avl/avl.c
        src/libnetdata/avl/avl.h
        src/libnetdata/buffer/buffer.c
        src/libnetdata/buffer/buffer.h
        src/libnetdata/circular_buffer/circular_buffer.c
        src/libnetdata/circular_buffer/circular_buffer.h
        src/libnetdata/clocks/clocks.c
        src/libnetdata/clocks/clocks.h
        src/libnetdata/completion/completion.c
        src/libnetdata/completion/completion.h
        src/libnetdata/datetime/iso8601.c
        src/libnetdata/datetime/iso8601.h
        src/libnetdata/datetime/rfc7231.c
        src/libnetdata/datetime/rfc7231.h
        src/libnetdata/datetime/rfc3339.c
        src/libnetdata/datetime/rfc3339.h
        src/libnetdata/dictionary/dictionary.c
        src/libnetdata/dictionary/dictionary.h
        src/libnetdata/eval/eval.c
        src/libnetdata/eval/eval.h
        src/libnetdata/facets/facets.c
        src/libnetdata/facets/facets.h
        src/libnetdata/functions_evloop/functions_evloop.c
        src/libnetdata/functions_evloop/functions_evloop.h
        src/libnetdata/gorilla/gorilla.cc
        src/libnetdata/gorilla/gorilla.h
        src/libnetdata/july/july.c
        src/libnetdata/july/july.h
        src/libnetdata/inlined.h
        src/libnetdata/json/json.c
        src/libnetdata/json/json.h
        src/libnetdata/json/jsmn.c
        src/libnetdata/json/jsmn.h
        src/libnetdata/libnetdata.c
        src/libnetdata/libnetdata.h
        src/libnetdata/line_splitter/line_splitter.c
        src/libnetdata/line_splitter/line_splitter.h
        src/libnetdata/libnetdata.h
        src/libnetdata/locks/locks.c
        src/libnetdata/locks/locks.h
        src/libnetdata/log/journal.c
        src/libnetdata/log/journal.h
        src/libnetdata/log/log.c
        src/libnetdata/log/log.h
        src/libnetdata/os.c
        src/libnetdata/os.h
        src/libnetdata/simple_hashtable.h
        src/libnetdata/byteorder.h
        src/libnetdata/onewayalloc/onewayalloc.c
        src/libnetdata/onewayalloc/onewayalloc.h
        src/libnetdata/popen/popen.c
        src/libnetdata/popen/popen.h
        src/libnetdata/procfile/procfile.c
        src/libnetdata/procfile/procfile.h
        src/libnetdata/query_progress/progress.c
        src/libnetdata/query_progress/progress.h
        src/libnetdata/required_dummies.h
        src/libnetdata/socket/security.c
        src/libnetdata/socket/security.h
        src/libnetdata/simple_pattern/simple_pattern.c
        src/libnetdata/simple_pattern/simple_pattern.h
        src/libnetdata/socket/socket.c
        src/libnetdata/socket/socket.h
        src/libnetdata/statistical/statistical.c
        src/libnetdata/statistical/statistical.h
        src/libnetdata/storage_number/storage_number.c
        src/libnetdata/storage_number/storage_number.h
        src/libnetdata/string/string.c
        src/libnetdata/string/string.h
        src/libnetdata/threads/threads.c
        src/libnetdata/threads/threads.h
        src/libnetdata/url/url.c
        src/libnetdata/url/url.h
        src/libnetdata/uuid/uuid.c
        src/libnetdata/uuid/uuid.h
        src/libnetdata/string/utf8.h
        src/libnetdata/worker_utilization/worker_utilization.c
        src/libnetdata/worker_utilization/worker_utilization.h
        src/libnetdata/http/http_access.c
        src/libnetdata/http/http_access.h
        src/libnetdata/http/http_defs.c
        src/libnetdata/http/http_defs.h
        src/libnetdata/http/content_type.c
        src/libnetdata/http/content_type.h
        src/libnetdata/config/dyncfg.c
        src/libnetdata/config/dyncfg.h
        src/libnetdata/json/json-c-parser-inline.h
        src/libnetdata/template-enum.h
        src/libnetdata/dictionary/dictionary-internals.h
        src/libnetdata/dictionary/dictionary-unittest.c
        src/libnetdata/dictionary/thread-cache.c
        src/libnetdata/dictionary/thread-cache.h
        src/libnetdata/dictionary/dictionary-traversal.c
        src/libnetdata/dictionary/dictionary-statistics.h
        src/libnetdata/dictionary/dictionary-locks.h
        src/libnetdata/dictionary/dictionary-refcount.h
        src/libnetdata/dictionary/dictionary-hashtable.h
        src/libnetdata/dictionary/dictionary-item.h
        src/libnetdata/dictionary/dictionary-callbacks.h
        src/libnetdata/linked-lists.h
        src/libnetdata/storage-point.h
        src/libnetdata/bitmap64.h
)

if(ENABLE_PLUGIN_EBPF)
        list(APPEND LIBNETDATA_FILES
                src/libnetdata/ebpf/ebpf.c
                src/libnetdata/ebpf/ebpf.h
        )
endif()

set(LIBH2O_FILES
        src/web/server/h2o/libh2o/deps/cloexec/cloexec.c
        src/web/server/h2o/libh2o/deps/libgkc/gkc.c
        src/web/server/h2o/libh2o/deps/libyrmcds/close.c
        src/web/server/h2o/libh2o/deps/libyrmcds/connect.c
        src/web/server/h2o/libh2o/deps/libyrmcds/recv.c
        src/web/server/h2o/libh2o/deps/libyrmcds/send.c
        src/web/server/h2o/libh2o/deps/libyrmcds/send_text.c
        src/web/server/h2o/libh2o/deps/libyrmcds/socket.c
        src/web/server/h2o/libh2o/deps/libyrmcds/strerror.c
        src/web/server/h2o/libh2o/deps/libyrmcds/text_mode.c
        src/web/server/h2o/libh2o/deps/picohttpparser/picohttpparser.c
        src/web/server/h2o/libh2o/lib/common/cache.c
        src/web/server/h2o/libh2o/lib/common/file.c
        src/web/server/h2o/libh2o/lib/common/filecache.c
        src/web/server/h2o/libh2o/lib/common/hostinfo.c
        src/web/server/h2o/libh2o/lib/common/http1client.c
        src/web/server/h2o/libh2o/lib/common/memcached.c
        src/web/server/h2o/libh2o/lib/common/memory.c
        src/web/server/h2o/libh2o/lib/common/multithread.c
        src/web/server/h2o/libh2o/lib/common/serverutil.c
        src/web/server/h2o/libh2o/lib/common/socket.c
        src/web/server/h2o/libh2o/lib/common/socketpool.c
        src/web/server/h2o/libh2o/lib/common/string.c
        src/web/server/h2o/libh2o/lib/common/time.c
        src/web/server/h2o/libh2o/lib/common/timeout.c
        src/web/server/h2o/libh2o/lib/common/url.c
        src/web/server/h2o/libh2o/lib/core/config.c
        src/web/server/h2o/libh2o/lib/core/configurator.c
        src/web/server/h2o/libh2o/lib/core/context.c
        src/web/server/h2o/libh2o/lib/core/headers.c
        src/web/server/h2o/libh2o/lib/core/logconf.c
        src/web/server/h2o/libh2o/lib/core/proxy.c
        src/web/server/h2o/libh2o/lib/core/request.c
        src/web/server/h2o/libh2o/lib/core/token.c
        src/web/server/h2o/libh2o/lib/core/util.c
        src/web/server/h2o/libh2o/lib/handler/access_log.c
        src/web/server/h2o/libh2o/lib/handler/chunked.c
        src/web/server/h2o/libh2o/lib/handler/compress.c
        src/web/server/h2o/libh2o/lib/handler/compress/gzip.c
        src/web/server/h2o/libh2o/lib/handler/errordoc.c
        src/web/server/h2o/libh2o/lib/handler/expires.c
        src/web/server/h2o/libh2o/lib/handler/fastcgi.c
        src/web/server/h2o/libh2o/lib/handler/file.c
        src/web/server/h2o/libh2o/lib/handler/headers.c
        src/web/server/h2o/libh2o/lib/handler/mimemap.c
        src/web/server/h2o/libh2o/lib/handler/proxy.c
        src/web/server/h2o/libh2o/lib/handler/redirect.c
        src/web/server/h2o/libh2o/lib/handler/reproxy.c
        src/web/server/h2o/libh2o/lib/handler/throttle_resp.c
        src/web/server/h2o/libh2o/lib/handler/status.c
        src/web/server/h2o/libh2o/lib/handler/headers_util.c
        src/web/server/h2o/libh2o/lib/handler/status/events.c
        src/web/server/h2o/libh2o/lib/handler/status/requests.c
        src/web/server/h2o/libh2o/lib/handler/http2_debug_state.c
        src/web/server/h2o/libh2o/lib/handler/status/durations.c
        src/web/server/h2o/libh2o/lib/handler/configurator/access_log.c
        src/web/server/h2o/libh2o/lib/handler/configurator/compress.c
        src/web/server/h2o/libh2o/lib/handler/configurator/errordoc.c
        src/web/server/h2o/libh2o/lib/handler/configurator/expires.c
        src/web/server/h2o/libh2o/lib/handler/configurator/fastcgi.c
        src/web/server/h2o/libh2o/lib/handler/configurator/file.c
        src/web/server/h2o/libh2o/lib/handler/configurator/headers.c
        src/web/server/h2o/libh2o/lib/handler/configurator/proxy.c
        src/web/server/h2o/libh2o/lib/handler/configurator/redirect.c
        src/web/server/h2o/libh2o/lib/handler/configurator/reproxy.c
        src/web/server/h2o/libh2o/lib/handler/configurator/throttle_resp.c
        src/web/server/h2o/libh2o/lib/handler/configurator/status.c
        src/web/server/h2o/libh2o/lib/handler/configurator/http2_debug_state.c
        src/web/server/h2o/libh2o/lib/handler/configurator/headers_util.c
        src/web/server/h2o/libh2o/lib/http1.c
        src/web/server/h2o/libh2o/lib/tunnel.c
        src/web/server/h2o/libh2o/lib/http2/cache_digests.c
        src/web/server/h2o/libh2o/lib/http2/casper.c
        src/web/server/h2o/libh2o/lib/http2/connection.c
        src/web/server/h2o/libh2o/lib/http2/frame.c
        src/web/server/h2o/libh2o/lib/http2/hpack.c
        src/web/server/h2o/libh2o/lib/http2/scheduler.c
        src/web/server/h2o/libh2o/lib/http2/stream.c
        src/web/server/h2o/libh2o/lib/http2/http2_debug_state.c
)

set(DAEMON_FILES
        src/daemon/buildinfo.c
        src/daemon/buildinfo.h
        src/daemon/common.c
        src/daemon/common.h
        src/daemon/daemon.c
        src/daemon/daemon.h
        src/daemon/event_loop.c
        src/daemon/event_loop.h
        src/daemon/global_statistics.c
        src/daemon/global_statistics.h
        src/daemon/analytics.c
        src/daemon/analytics.h
        src/daemon/main.c
        src/daemon/main.h
        src/daemon/signals.c
        src/daemon/signals.h
        src/daemon/service.c
        src/daemon/watcher.c
        src/daemon/watcher.h
        src/daemon/static_threads.c
        src/daemon/static_threads.h
        src/daemon/commands.c
        src/daemon/commands.h
        src/daemon/pipename.c
        src/daemon/pipename.h
        src/daemon/unit_test.c
        src/daemon/unit_test.h
        src/daemon/config/dyncfg.c
        src/daemon/config/dyncfg.h
        src/daemon/config/dyncfg-files.c
        src/daemon/config/dyncfg-unittest.c
        src/daemon/config/dyncfg-inline.c
        src/daemon/config/dyncfg-echo.c
        src/daemon/config/dyncfg-internals.h
        src/daemon/config/dyncfg-intercept.c
        src/daemon/config/dyncfg-tree.c
)

set(H2O_FILES
        src/web/server/h2o/http_server.c
        src/web/server/h2o/http_server.h
        src/web/server/h2o/h2o_utils.c
        src/web/server/h2o/h2o_utils.h
        src/web/server/h2o/streaming.c
        src/web/server/h2o/streaming.h
        src/web/server/h2o/connlist.c
        src/web/server/h2o/connlist.h
)

if(ENABLE_H2O)
    list(APPEND DAEMON_FILES ${H2O_FILES})
endif()

set(API_PLUGIN_FILES
        src/web/api/web_api.c
        src/web/api/web_api.h
        src/web/api/web_api_v1.c
        src/web/api/web_api_v1.h
        src/web/api/web_api_v2.c
        src/web/api/web_api_v2.h
        src/web/api/http_auth.c
        src/web/api/http_auth.h
        src/web/api/http_header.c
        src/web/api/http_header.h
        src/web/api/badges/web_buffer_svg.c
        src/web/api/badges/web_buffer_svg.h
        src/web/api/exporters/allmetrics.c
        src/web/api/exporters/allmetrics.h
        src/web/api/exporters/shell/allmetrics_shell.c
        src/web/api/exporters/shell/allmetrics_shell.h
        src/web/api/queries/rrdr.c
        src/web/api/queries/rrdr.h
        src/web/api/queries/query.c
        src/web/api/queries/query.h
        src/web/api/queries/average/average.c
        src/web/api/queries/average/average.h
        src/web/api/queries/countif/countif.c
        src/web/api/queries/countif/countif.h
        src/web/api/queries/incremental_sum/incremental_sum.c
        src/web/api/queries/incremental_sum/incremental_sum.h
        src/web/api/queries/max/max.c
        src/web/api/queries/max/max.h
        src/web/api/queries/min/min.c
        src/web/api/queries/min/min.h
        src/web/api/queries/sum/sum.c
        src/web/api/queries/sum/sum.h
        src/web/api/queries/median/median.c
        src/web/api/queries/median/median.h
        src/web/api/queries/percentile/percentile.c
        src/web/api/queries/percentile/percentile.h
        src/web/api/queries/stddev/stddev.c
        src/web/api/queries/stddev/stddev.h
        src/web/api/queries/ses/ses.c
        src/web/api/queries/ses/ses.h
        src/web/api/queries/des/des.c
        src/web/api/queries/des/des.h
        src/web/api/queries/trimmed_mean/trimmed_mean.c
        src/web/api/queries/trimmed_mean/trimmed_mean.h
        src/web/api/queries/weights.c
        src/web/api/queries/weights.h
        src/web/api/formatters/rrd2json.c
        src/web/api/formatters/rrd2json.h
        src/web/api/formatters/csv/csv.c
        src/web/api/formatters/csv/csv.h
        src/web/api/formatters/json/json.c
        src/web/api/formatters/json/json.h
        src/web/api/formatters/ssv/ssv.c
        src/web/api/formatters/ssv/ssv.h
        src/web/api/formatters/value/value.c
        src/web/api/formatters/value/value.h
        src/web/api/formatters/json_wrapper.c
        src/web/api/formatters/json_wrapper.h
        src/web/api/formatters/charts2json.c
        src/web/api/formatters/charts2json.h
        src/web/api/formatters/rrdset2json.c
        src/web/api/formatters/rrdset2json.h
        src/web/api/ilove/ilove.c
        src/web/api/ilove/ilove.h
        src/web/rtc/webrtc.c
        src/web/rtc/webrtc.h
)

set(EXPORTING_ENGINE_FILES
        src/exporting/exporting_engine.c
        src/exporting/exporting_engine.h
        src/exporting/graphite/graphite.c
        src/exporting/graphite/graphite.h
        src/exporting/json/json.c
        src/exporting/json/json.h
        src/exporting/opentsdb/opentsdb.c
        src/exporting/opentsdb/opentsdb.h
        src/exporting/prometheus/prometheus.c
        src/exporting/prometheus/prometheus.h
        src/exporting/read_config.c
        src/exporting/clean_connectors.c
        src/exporting/init_connectors.c
        src/exporting/process_data.c
        src/exporting/check_filters.c
        src/exporting/send_data.c
        src/exporting/send_internal_metrics.c
)

set(HEALTH_PLUGIN_FILES
        src/health/health.c
        src/health/health.h
        src/health/health_config.c
        src/health/health_json.c
        src/health/health_log.c
        src/health/health_prototypes.c
        src/health/health_prototypes.h
        src/health/health_silencers.c
        src/health/health_silencers.h
        src/health/health_internals.h
        src/health/health_notifications.c
        src/health/health_event_loop.c
        src/health/health_dyncfg.c
        src/health/health_variable.c
        src/health/rrdcalc.c
        src/health/rrdcalc.h
        src/health/rrdvar.c
        src/health/rrdvar.h
)

set(IDLEJITTER_PLUGIN_FILES src/collectors/idlejitter.plugin/plugin_idlejitter.c)

if(ENABLE_ML)
        set(ML_FILES
                src/ml/ad_charts.h
                src/ml/ad_charts.cc
                src/ml/Config.cc
                src/ml/dlib/dlib/all/source.cpp
                src/ml/ml.h
                src/ml/ml.cc
                src/ml/ml-private.h
        )
else()
        set(ML_FILES
                src/ml/ml.h
                src/ml/ml-dummy.c
        )
endif()

set(PLUGINSD_PLUGIN_FILES
        src/collectors/plugins.d/plugins_d.c
        src/collectors/plugins.d/plugins_d.h
        src/collectors/plugins.d/pluginsd_dyncfg.c
        src/collectors/plugins.d/pluginsd_dyncfg.h
        src/collectors/plugins.d/pluginsd_functions.c
        src/collectors/plugins.d/pluginsd_functions.h
        src/collectors/plugins.d/pluginsd_internals.c
        src/collectors/plugins.d/pluginsd_internals.h
        src/collectors/plugins.d/pluginsd_parser.c
        src/collectors/plugins.d/pluginsd_parser.h
        src/collectors/plugins.d/pluginsd_replication.c
        src/collectors/plugins.d/pluginsd_replication.h
)

set(RRD_PLUGIN_FILES
        src/database/contexts/api_v1.c
        src/database/contexts/api_v2.c
        src/database/contexts/context.c
        src/database/contexts/instance.c
        src/database/contexts/internal.h
        src/database/contexts/metric.c
        src/database/contexts/query_scope.c
        src/database/contexts/query_target.c
        src/database/contexts/rrdcontext.c
        src/database/contexts/rrdcontext.h
        src/database/contexts/worker.c
        src/database/rrdcollector.c
        src/database/rrdcollector.h
        src/database/rrddim.c
        src/database/rrdfunctions.c
        src/database/rrdfunctions.h
        src/database/rrdfunctions-inline.c
        src/database/rrdfunctions-inline.h
        src/database/rrdfunctions-progress.c
        src/database/rrdfunctions-progress.h
        src/database/rrdfunctions-streaming.c
        src/database/rrdfunctions-streaming.h
        src/database/rrdhost.c
        src/database/rrdlabels.c
        src/database/rrd.c
        src/database/rrd.h
        src/database/rrdset.c
        src/database/storage_engine.c
        src/database/storage_engine.h
        src/database/ram/rrddim_mem.c
        src/database/ram/rrddim_mem.h
        src/database/sqlite/sqlite_metadata.c
        src/database/sqlite/sqlite_metadata.h
        src/database/sqlite/sqlite_functions.c
        src/database/sqlite/sqlite_functions.h
        src/database/sqlite/sqlite_context.c
        src/database/sqlite/sqlite_context.h
        src/database/sqlite/sqlite_db_migration.c
        src/database/sqlite/sqlite_db_migration.h
        src/database/sqlite/sqlite_aclk.c
        src/database/sqlite/sqlite_aclk.h
        src/database/sqlite/sqlite_health.c
        src/database/sqlite/sqlite_health.h
        src/database/sqlite/sqlite_aclk_node.c
        src/database/sqlite/sqlite_aclk_node.h
        src/database/sqlite/sqlite_aclk_alert.c
        src/database/sqlite/sqlite_aclk_alert.h
        src/database/sqlite/sqlite3.c
        src/database/sqlite/sqlite3.h
        src/database/sqlite/sqlite3recover.c
        src/database/sqlite/sqlite3recover.h
        src/database/sqlite/dbdata.c
        src/database/KolmogorovSmirnovDist.c
        src/database/KolmogorovSmirnovDist.h
        src/database/rrdfunctions-inflight.c
        src/database/rrdfunctions-inflight.h
        src/database/rrdfunctions-exporters.c
        src/database/rrdfunctions-exporters.h
        src/database/rrdfunctions-internals.h
        src/database/rrdcollector-internals.h
)

if(ENABLE_DBENGINE)
    list(APPEND RRD_PLUGIN_FILES
            src/database/engine/rrdengine.c
            src/database/engine/rrdengine.h
            src/database/engine/rrddiskprotocol.h
            src/database/engine/datafile.c
            src/database/engine/datafile.h
            src/database/engine/journalfile.c
            src/database/engine/journalfile.h
            src/database/engine/rrdenginelib.c
            src/database/engine/rrdenginelib.h
            src/database/engine/rrdengineapi.c
            src/database/engine/rrdengineapi.h
            src/database/engine/pagecache.c
            src/database/engine/pagecache.h
            src/database/engine/page_test.cc
            src/database/engine/page.c
            src/database/engine/page.h
            src/database/engine/cache.c
            src/database/engine/cache.h
            src/database/engine/metric.c
            src/database/engine/metric.h
            src/database/engine/pdc.c
            src/database/engine/pdc.h
            src/database/engine/dbengine-unittest.c
            src/database/engine/dbengine-stresstest.c
            src/database/engine/dbengine-compression.c
            src/database/engine/dbengine-compression.h
    )
endif()

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
)

set(STATSD_PLUGIN_FILES
        src/collectors/statsd.plugin/statsd.c
)

set(SYSTEMD_JOURNAL_PLUGIN_FILES
        src/collectors/systemd-journal.plugin/systemd-journal.c
        src/collectors/systemd-journal.plugin/systemd-internals.h
        src/collectors/systemd-journal.plugin/systemd-main.c
        src/collectors/systemd-journal.plugin/systemd-units.c
        src/collectors/systemd-journal.plugin/systemd-journal.c
        src/collectors/systemd-journal.plugin/systemd-journal-annotations.c
        src/collectors/systemd-journal.plugin/systemd-journal-files.c
        src/collectors/systemd-journal.plugin/systemd-journal-fstat.c
        src/collectors/systemd-journal.plugin/systemd-journal-watcher.c
        src/collectors/systemd-journal.plugin/systemd-journal-dyncfg.c
        src/libnetdata/maps/system-users.h
        src/libnetdata/maps/system-groups.h
)

set(STREAMING_PLUGIN_FILES
        src/streaming/rrdpush.c
        src/streaming/rrdpush.h
        src/streaming/compression.c
        src/streaming/compression.h
        src/streaming/compression_brotli.c
        src/streaming/compression_brotli.h
        src/streaming/compression_gzip.c
        src/streaming/compression_gzip.h
        src/streaming/compression_lz4.c
        src/streaming/compression_lz4.h
        src/streaming/compression_zstd.c
        src/streaming/compression_zstd.h
        src/streaming/receiver.c
        src/streaming/sender.c
        src/streaming/replication.c
        src/streaming/replication.h
        src/streaming/common.h
)

set(WEB_PLUGIN_FILES
        src/web/server/web_client.c
        src/web/server/web_client.h
        src/web/server/web_server.c
        src/web/server/web_server.h
        src/web/server/static/static-threaded.c
        src/web/server/static/static-threaded.h
        src/web/server/web_client_cache.c
        src/web/server/web_client_cache.h
)

set(CLAIM_PLUGIN_FILES
        src/claim/claim.c
        src/claim/claim.h
)

set(SPAWN_PLUGIN_FILES
        src/spawn/spawn.c
        src/spawn/spawn_server.c
        src/spawn/spawn_client.c
        src/spawn/spawn.h
)

set(ACLK_ALWAYS_BUILD
        src/aclk/aclk_rrdhost_state.h
        src/aclk/aclk_proxy.c
        src/aclk/aclk_proxy.h
        src/aclk/aclk.c
        src/aclk/aclk.h
        src/aclk/aclk_capas.c
        src/aclk/aclk_capas.h
        src/aclk/aclk_util.c
        src/aclk/aclk_util.h
        src/aclk/https_client.c
        src/aclk/https_client.h
)

set(TIMEX_PLUGIN_FILES
        src/collectors/timex.plugin/plugin_timex.c
)

set(PROFILE_PLUGIN_FILES
        src/collectors/profile.plugin/plugin_profile.cc
)

set(CGROUPS_PLUGIN_FILES
        src/collectors/cgroups.plugin/sys_fs_cgroup.c
        src/collectors/cgroups.plugin/sys_fs_cgroup.h
        src/collectors/cgroups.plugin/cgroup-internals.h
        src/collectors/cgroups.plugin/cgroup-discovery.c
        src/collectors/cgroups.plugin/cgroup-charts.c
        src/collectors/cgroups.plugin/cgroup-top.c
)

set(DISKSPACE_PLUGIN_FILES
        src/collectors/diskspace.plugin/plugin_diskspace.c
)

set(MACOS_PLUGIN_FILES
        src/collectors/macos.plugin/plugin_macos.c
        src/collectors/macos.plugin/plugin_macos.h
        src/collectors/macos.plugin/macos_sysctl.c
        src/collectors/macos.plugin/macos_mach_smi.c
        src/collectors/macos.plugin/macos_fw.c
)

set(FREEBSD_PLUGIN_FILES
        src/collectors/freebsd.plugin/plugin_freebsd.c
        src/collectors/freebsd.plugin/plugin_freebsd.h
        src/collectors/freebsd.plugin/freebsd_sysctl.c
        src/collectors/freebsd.plugin/freebsd_getmntinfo.c
        src/collectors/freebsd.plugin/freebsd_getifaddrs.c
        src/collectors/freebsd.plugin/freebsd_devstat.c
        src/collectors/freebsd.plugin/freebsd_kstat_zfs.c
        src/collectors/freebsd.plugin/freebsd_ipfw.c
        src/collectors/proc.plugin/zfs_common.c
        src/collectors/proc.plugin/zfs_common.h
)

set(PROC_PLUGIN_FILES
        src/collectors/proc.plugin/ipc.c
        src/collectors/proc.plugin/plugin_proc.c
        src/collectors/proc.plugin/plugin_proc.h
        src/collectors/proc.plugin/proc_sys_fs_file_nr.c
        src/collectors/proc.plugin/proc_diskstats.c
        src/collectors/proc.plugin/proc_mdstat.c
        src/collectors/proc.plugin/proc_interrupts.c
        src/collectors/proc.plugin/proc_softirqs.c
        src/collectors/proc.plugin/proc_loadavg.c
        src/collectors/proc.plugin/proc_meminfo.c
        src/collectors/proc.plugin/proc_pagetypeinfo.c
        src/collectors/proc.plugin/proc_net_dev.c
        src/collectors/proc.plugin/proc_net_dev_renames.c
        src/collectors/proc.plugin/proc_net_dev_renames.h
        src/collectors/proc.plugin/proc_net_wireless.c
        src/collectors/proc.plugin/proc_net_ip_vs_stats.c
        src/collectors/proc.plugin/proc_net_netstat.c
        src/collectors/proc.plugin/proc_net_rpc_nfs.c
        src/collectors/proc.plugin/proc_net_rpc_nfsd.c
        src/collectors/proc.plugin/proc_net_sctp_snmp.c
        src/collectors/proc.plugin/proc_net_sockstat.c
        src/collectors/proc.plugin/proc_net_sockstat6.c
        src/collectors/proc.plugin/proc_net_softnet_stat.c
        src/collectors/proc.plugin/proc_net_stat_conntrack.c
        src/collectors/proc.plugin/proc_net_stat_synproxy.c
        src/collectors/proc.plugin/proc_self_mountinfo.c
        src/collectors/proc.plugin/proc_self_mountinfo.h
        src/collectors/proc.plugin/zfs_common.c
        src/collectors/proc.plugin/zfs_common.h
        src/collectors/proc.plugin/proc_spl_kstat_zfs.c
        src/collectors/proc.plugin/proc_stat.c
        src/collectors/proc.plugin/proc_sys_kernel_random_entropy_avail.c
        src/collectors/proc.plugin/proc_vmstat.c
        src/collectors/proc.plugin/proc_uptime.c
        src/collectors/proc.plugin/proc_pressure.c
        src/collectors/proc.plugin/proc_pressure.h
        src/collectors/proc.plugin/sys_kernel_mm_ksm.c
        src/collectors/proc.plugin/sys_block_zram.c
        src/collectors/proc.plugin/sys_devices_system_edac_mc.c
        src/collectors/proc.plugin/sys_devices_system_node.c
        src/collectors/proc.plugin/sys_class_infiniband.c
        src/collectors/proc.plugin/sys_fs_btrfs.c
        src/collectors/proc.plugin/sys_class_power_supply.c
        src/collectors/proc.plugin/sys_devices_pci_aer.c
        src/collectors/proc.plugin/sys_class_drm.c
)

set(TC_PLUGIN_FILES
        src/collectors/tc.plugin/plugin_tc.c
)

set(LOGS_MANAGEMENT_PLUGIN_FILES
        src/logsmanagement/circular_buffer.c
        src/logsmanagement/circular_buffer.h
        src/logsmanagement/db_api.c
        src/logsmanagement/db_api.h
        src/logsmanagement/defaults.h
        src/logsmanagement/file_info.h
        src/logsmanagement/flb_plugin.c
        src/logsmanagement/flb_plugin.h
        src/logsmanagement/functions.c
        src/logsmanagement/functions.h
        src/logsmanagement/helper.h
        src/logsmanagement/logsmanag_config.c
        src/logsmanagement/logsmanag_config.h
        src/logsmanagement/logsmanagement.c
        src/logsmanagement/parser.c
        src/logsmanagement/parser.h
        src/logsmanagement/query.c
        src/logsmanagement/query.h
        src/logsmanagement/rrd_api/rrd_api_docker_ev.c
        src/logsmanagement/rrd_api/rrd_api_docker_ev.h
        src/logsmanagement/rrd_api/rrd_api_generic.c
        src/logsmanagement/rrd_api/rrd_api_generic.h
        src/logsmanagement/rrd_api/rrd_api_kernel.c
        src/logsmanagement/rrd_api/rrd_api_kernel.h
        src/logsmanagement/rrd_api/rrd_api_mqtt.c
        src/logsmanagement/rrd_api/rrd_api_mqtt.h
        src/logsmanagement/rrd_api/rrd_api_stats.c
        src/logsmanagement/rrd_api/rrd_api_stats.h
        src/logsmanagement/rrd_api/rrd_api_systemd.c
        src/logsmanagement/rrd_api/rrd_api_systemd.h
        src/logsmanagement/rrd_api/rrd_api_web_log.c
        src/logsmanagement/rrd_api/rrd_api_web_log.h
        src/logsmanagement/rrd_api/rrd_api.h
        src/database/sqlite/sqlite3.c
        src/database/sqlite/sqlite3.h
)

set(NETDATA_FILES
        src/collectors/all.h
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
        ${ACLK_ALWAYS_BUILD}
        ${PROFILE_PLUGIN_FILES}
)

if(LINUX)
        list(APPEND NETDATA_FILES
                src/daemon/static_threads_linux.c
                ${CGROUPS_PLUGIN_FILES}
                ${DISKSPACE_PLUGIN_FILES}
                ${PROC_PLUGIN_FILES}
                ${TC_PLUGIN_FILES}
                ${TIMEX_PLUGIN_FILES}
        )

        if(ENABLE_SENTRY)
            list(APPEND NETDATA_FILES
                    src/daemon/sentry-native/sentry-native.c
                    src/daemon/sentry-native/sentry-native.h)
        endif()
elseif(MACOS)
        list(APPEND NETDATA_FILES
                src/daemon/static_threads_macos.c
                ${MACOS_PLUGIN_FILES}
                ${TIMEX_PLUGIN_FILES}
        )
elseif(FREEBSD)
        list(APPEND NETDATA_FILES
                src/daemon/static_threads_freebsd.c
                ${FREEBSD_PLUGIN_FILES}
                ${TIMEX_PLUGIN_FILES}
        )
endif()

set(MQTT_WEBSOCKETS_FILES
        src/aclk/mqtt_websockets/mqtt_wss_client.c
        src/aclk/mqtt_websockets/mqtt_wss_client.h
        src/aclk/mqtt_websockets/mqtt_wss_log.c
        src/aclk/mqtt_websockets/mqtt_wss_log.h
        src/aclk/mqtt_websockets/ws_client.c
        src/aclk/mqtt_websockets/ws_client.h
        src/aclk/mqtt_websockets/mqtt_ng.c
        src/aclk/mqtt_websockets/mqtt_ng.h
        src/aclk/mqtt_websockets/common_public.c
        src/aclk/mqtt_websockets/common_public.h
        src/aclk/mqtt_websockets/common_internal.h
        src/aclk/mqtt_websockets/c-rbuf/cringbuffer.c
        src/aclk/mqtt_websockets/c-rbuf/cringbuffer.h
        src/aclk/mqtt_websockets/c-rbuf/cringbuffer_internal.h
        src/aclk/mqtt_websockets/c_rhash/c_rhash.c
        src/aclk/mqtt_websockets/c_rhash/c_rhash.h
        src/aclk/mqtt_websockets/c_rhash/c_rhash_internal.h
)

set(ACLK_PROTO_DEFS
        src/aclk/aclk-schemas/proto/aclk/v1/lib.proto
        src/aclk/aclk-schemas/proto/agent/v1/disconnect.proto
        src/aclk/aclk-schemas/proto/agent/v1/connection.proto
        src/aclk/aclk-schemas/proto/alarm/v1/config.proto
        src/aclk/aclk-schemas/proto/alarm/v1/stream.proto
        src/aclk/aclk-schemas/proto/nodeinstance/connection/v1/connection.proto
        src/aclk/aclk-schemas/proto/nodeinstance/create/v1/creation.proto
        src/aclk/aclk-schemas/proto/nodeinstance/info/v1/info.proto
        src/aclk/aclk-schemas/proto/context/v1/context.proto
        src/aclk/aclk-schemas/proto/context/v1/stream.proto
        src/aclk/aclk-schemas/proto/agent/v1/cmds.proto
)

set(ACLK_FILES
        src/aclk/aclk_stats.c
        src/aclk/aclk_stats.h
        src/aclk/aclk_query.c
        src/aclk/aclk_query.h
        src/aclk/aclk_query_queue.c
        src/aclk/aclk_query_queue.h
        src/aclk/aclk_otp.c
        src/aclk/aclk_otp.h
        src/aclk/aclk_tx_msgs.c
        src/aclk/aclk_tx_msgs.h
        src/aclk/aclk_rx_msgs.c
        src/aclk/aclk_rx_msgs.h
        src/aclk/aclk_alarm_api.c
        src/aclk/aclk_alarm_api.h
        src/aclk/aclk_contexts_api.c
        src/aclk/aclk_contexts_api.h
        src/aclk/schema-wrappers/connection.cc
        src/aclk/schema-wrappers/connection.h
        src/aclk/schema-wrappers/node_connection.cc
        src/aclk/schema-wrappers/node_connection.h
        src/aclk/schema-wrappers/node_creation.cc
        src/aclk/schema-wrappers/node_creation.h
        src/aclk/schema-wrappers/alarm_stream.cc
        src/aclk/schema-wrappers/alarm_stream.h
        src/aclk/schema-wrappers/alarm_config.cc
        src/aclk/schema-wrappers/alarm_config.h
        src/aclk/schema-wrappers/node_info.cc
        src/aclk/schema-wrappers/node_info.h
        src/aclk/schema-wrappers/capability.cc
        src/aclk/schema-wrappers/capability.h
        src/aclk/schema-wrappers/proto_2_json.cc
        src/aclk/schema-wrappers/proto_2_json.h
        src/aclk/schema-wrappers/context_stream.cc
        src/aclk/schema-wrappers/context_stream.h
        src/aclk/schema-wrappers/context.cc
        src/aclk/schema-wrappers/context.h
        src/aclk/schema-wrappers/schema_wrappers.h
        src/aclk/schema-wrappers/schema_wrapper_utils.cc
        src/aclk/schema-wrappers/schema_wrapper_utils.h
        src/aclk/schema-wrappers/agent_cmds.cc
        src/aclk/schema-wrappers/agent_cmds.h
        src/aclk/helpers/mqtt_wss_pal.h
        src/aclk/helpers/ringbuffer_pal.h
)


set(MONGODB_EXPORTING_FILES
        src/exporting/mongodb/mongodb.c
        src/exporting/mongodb/mongodb.h
)

set(PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES
        src/exporting/prometheus/remote_write/remote_write.c
        src/exporting/prometheus/remote_write/remote_write.h
        src/exporting/prometheus/remote_write/remote_write_request.cc
        src/exporting/prometheus/remote_write/remote_write_request.h
)

#
# build h2o
#

if(ENABLE_H2O)
        add_library(h2o STATIC ${LIBH2O_FILES})

        target_include_directories(h2o BEFORE PUBLIC
                "${CMAKE_SOURCE_DIR}/src/web/server/h2o/libh2o/include"
                "${CMAKE_SOURCE_DIR}/src/web/server/h2o/libh2o/deps/cloexec"
                "${CMAKE_SOURCE_DIR}/src/web/server/h2o/libh2o/deps/brotli/enc"
                "${CMAKE_SOURCE_DIR}/src/web/server/h2o/libh2o/deps/golombset"
                "${CMAKE_SOURCE_DIR}/src/web/server/h2o/libh2o/deps/libgkc"
                "${CMAKE_SOURCE_DIR}/src/web/server/h2o/libh2o/deps/libyrmcds"
                "${CMAKE_SOURCE_DIR}/src/web/server/h2o/libh2o/deps/klib"
                "${CMAKE_SOURCE_DIR}/src/web/server/h2o/libh2o/deps/neverbleed"
                "${CMAKE_SOURCE_DIR}/src/web/server/h2o/libh2o/deps/picohttpparser"
                "${CMAKE_SOURCE_DIR}/src/web/server/h2o/libh2o/deps/picotest"
                "${CMAKE_SOURCE_DIR}/src/web/server/h2o/libh2o/deps/yaml/include"
                "${CMAKE_SOURCE_DIR}/src/web/server/h2o/libh2o/deps/yoml"
        )

        target_compile_options(h2o PRIVATE
                -Wno-all -Wno-extra
                -Wno-shadow
                -Wno-deprecated-declarations
                -Wformat
        )

        target_compile_options(h2o PUBLIC -DH2O_USE_LIBUV=0)

        target_include_directories(h2o BEFORE PRIVATE ${OPENSSL_INCLUDE_DIRS})
        target_compile_options(h2o PRIVATE ${OPENSSL_CFLAGS_OTHER})
        target_link_libraries(h2o PRIVATE ${OPENSSL_LIBRARIES})
endif()

#
# build libjudy
#

add_library(judy STATIC ${LIBJUDY_SOURCES})

target_compile_options(judy PRIVATE
        -Wno-all -Wno-extra
        -Wno-shadow
        -Wformat
)

target_compile_definitions(judy PRIVATE
        JUDYL
        $<$<EQUAL:${CMAKE_SIZEOF_VOID_P},8>:JU_64BIT>
)

target_include_directories(judy PUBLIC
        src/libnetdata/libjudy/src
        src/libnetdata/libjudy/src/JudyCommon
)

set_source_files_properties(${LIBJUDY_PREV_FILES} PROPERTIES COMPILE_OPTIONS "-DJUDYPREV")
set_source_files_properties(${LIBJUDY_NEXT_FILES} PROPERTIES COMPILE_OPTIONS "-DJUDYNEXT")
set_source_files_properties(src/libnetdata/libjudy/src/JudyL/j__udyLGet.c PROPERTIES COMPILE_OPTIONS "-DJUDYGETINLINE")
set_source_files_properties(src/libnetdata/libjudy/src/JudyL/JudyLByCount.c PROPERTIES COMPILE_OPTIONS "-DNOSMARTJBB -DNOSMARTJBU -DNOSMARTJLB")
set_source_files_properties(JudyLTables.c PROPERTIES COMPILE_OPTIONS "-I${CMAKE_SOURCE_DIR}/src/libnetdata/libjudy/src/JudyL")

#
# build libnetdata
#

if(LINUX)
        include(NetdataDetectSystemd)
        detect_systemd()
endif()

add_library(libnetdata STATIC ${LIBNETDATA_FILES})

target_include_directories(libnetdata BEFORE PUBLIC ${CONFIG_H_DIR} ${CMAKE_SOURCE_DIR}/src)

# pthread (FIXME: use find_package for this)

# set(CMAKE_THREAD_PREFER_PTHREAD TRUE)
# set(THREADS_PREFER_PTHREAD_FLAG TRUE)
# find_package(Threads REQUIRED)

# add_executable(test test.cpp)
# target_link_libraries(test Threads::Threads)

target_link_libraries(libnetdata PUBLIC
        "$<$<NOT:$<BOOL:${HAVE_BUILTIN_ATOMICS}>>:atomic>"
        "$<$<OR:$<BOOL:${LINUX}>,$<BOOL:${FREEBSD}>>:pthread;rt>"
        "$<$<BOOL:${LINK_LIBM}>:m>"
        "${SYSTEMD_LDFLAGS}")

# ebpf
if(ENABLE_PLUGIN_EBPF)
        netdata_add_libbpf_to_target(libnetdata)
endif()

# judy
target_link_libraries(libnetdata PUBLIC judy)

netdata_add_jsonc_to_target(libnetdata)

netdata_add_libyaml_to_target(libnetdata)

# zlib
if(MACOS)
        find_package(ZLIB REQUIRED)
        target_include_directories(libnetdata BEFORE PUBLIC ${ZLIB_INCLUDE_DIRS})
        target_link_libraries(libnetdata PUBLIC ZLIB::ZLIB)
else()
        pkg_check_modules(ZLIB REQUIRED zlib)
        target_include_directories(libnetdata BEFORE PUBLIC ${ZLIB_INCLUDE_DIRS})
        target_compile_definitions(libnetdata PUBLIC ${ZLIB_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${ZLIB_LDFLAGS})
endif()

# lz4 - try to find a version that is compatible with streaming compression
# otherwise pick whichever one we can find to support dbengine but don't set
# ENABLE_LZ4.
pkg_check_modules(LIBLZ4 liblz4>=1.9.0)
if(LIBLZ4_FOUND)
        set(ENABLE_LZ4 On)
else()
        pkg_check_modules(LIBLZ4 REQUIRED liblz4)
endif()

target_include_directories(libnetdata BEFORE PUBLIC ${LIBLZ4_INCLUDE_DIRS})
target_compile_definitions(libnetdata PUBLIC ${LIBLZ4_CFLAGS_OTHER})
target_link_libraries(libnetdata PUBLIC ${LIBLZ4_LDFLAGS})

# zstd
pkg_check_modules(LIBZSTD libzstd)
if(LIBZSTD_FOUND)
        set(ENABLE_ZSTD On)
        target_include_directories(libnetdata BEFORE PUBLIC ${LIBZSTD_INCLUDE_DIRS})
        target_compile_definitions(libnetdata PUBLIC ${LIBZSTD_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${LIBZSTD_LDFLAGS})
endif()

# brotli
pkg_check_modules(LIBBROTLI libbrotlidec libbrotlienc libbrotlicommon)
if(LIBBROTLI_FOUND)
        set(ENABLE_BROTLI On)
        target_include_directories(libnetdata PUBLIC ${LIBBROTLI_INCLUDE_DIRS})
        target_compile_definitions(libnetdata PUBLIC ${LIBBROTLI_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${LIBBROTLI_LDFLAGS})
endif()

# uuid
if(MACOS)
        # UUID functionality is part of the system libraries here, so no extra
        # stuff needed.
else()
        pkg_check_modules(UUID REQUIRED uuid)
        target_include_directories(libnetdata BEFORE PUBLIC ${UUID_INCLUDE_DIRS})
        target_compile_definitions(libnetdata PUBLIC ${UUID_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${UUID_LDFLAGS})
endif()

# uv
pkg_check_modules(LIBUV REQUIRED libuv)
target_include_directories(libnetdata BEFORE PUBLIC ${LIBUV_INCLUDE_DIRS})
target_compile_definitions(libnetdata PUBLIC ${LIBUV_CFLAGS_OTHER})
target_link_libraries(libnetdata PUBLIC ${LIBUV_LDFLAGS})

# crypto
target_include_directories(libnetdata BEFORE PUBLIC ${CRYPTO_INCLUDE_DIRS})
target_compile_options(libnetdata PUBLIC ${CRYPTO_CFLAGS_OTHER})
target_link_libraries(libnetdata PUBLIC ${CRYPTO_LDFLAGS})

# openssl
target_include_directories(libnetdata BEFORE PUBLIC ${OPENSSL_INCLUDE_DIRS})
target_compile_options(libnetdata PUBLIC ${OPENSSL_CFLAGS_OTHER})
target_link_libraries(libnetdata PUBLIC ${OPENSSL_LDFLAGS})

# mnl
if(NOT MACOS)
    pkg_check_modules(MNL libmnl)
    if(MNL_FOUND)
        set(HAVE_LIBMNL True)
    endif()
endif()

#
# mqtt library
#
if (ENABLE_H2O OR ENABLE_ACLK)
        set(ENABLE_MQTTWEBSOCKETS True)
endif()

if(ENABLE_MQTTWEBSOCKETS)
        # include_directories(BEFORE
        #         ${CMAKE_SOURCE_DIR}/aclk/mqtt_websockets/src/include
        #         ${CMAKE_SOURCE_DIR}/aclk/mqtt_websockets/c-rbuf/include
        #         ${CMAKE_SOURCE_DIR}/aclk/mqtt_websockets/c_rhash/include
        # )

        add_library(mqttwebsockets STATIC ${MQTT_WEBSOCKETS_FILES})

        target_compile_options(mqttwebsockets PUBLIC -DMQTT_WSS_CUSTOM_ALLOC
                                                     -DRBUF_CUSTOM_MALLOC
                                                     -DMQTT_WSS_CPUSTATS)

        target_include_directories(mqttwebsockets PUBLIC ${CMAKE_SOURCE_DIR}/aclk/helpers
                                                         ${CMAKE_SOURCE_DIR}/src/web/server/h2o/libh2o/include)

        target_link_libraries(mqttwebsockets PRIVATE libnetdata)

endif()

if(ENABLE_ACLK)
        #
        # proto definitions
        #
        netdata_protoc_generate_cpp("${CMAKE_SOURCE_DIR}/src/aclk/aclk-schemas"
                                    "${CMAKE_SOURCE_DIR}/src/aclk/aclk-schemas"
                                    ACLK_PROTO_BUILT_SRCS
                                    ACLK_PROTO_BUILT_HDRS
                                    ${ACLK_PROTO_DEFS})

        list(APPEND ACLK_FILES ${ACLK_PROTO_BUILT_SRCS}
                               ${ACLK_PROTO_BUILT_HDRS})

endif()

#
# build plugins
#

if(ENABLE_PLUGIN_DEBUGFS)
    pkg_check_modules(CAP QUIET libcap)

    set(DEBUGFS_PLUGIN_FILES src/collectors/debugfs.plugin/debugfs_plugin.c
                             src/collectors/debugfs.plugin/debugfs_plugin.h
                             src/collectors/debugfs.plugin/debugfs_extfrag.c
                             src/collectors/debugfs.plugin/debugfs_zswap.c
                             src/collectors/debugfs.plugin/sys_devices_virtual_powercap.c)

    add_executable(debugfs.plugin ${DEBUGFS_PLUGIN_FILES})
    target_link_libraries(debugfs.plugin libnetdata ${CAP_LIBRARIES})
    target_include_directories(debugfs.plugin PRIVATE ${CAP_INCLUDE_DIRS})
    target_compile_options(debugfs.plugin PRIVATE ${CAP_CFLAGS_OTHER})

    install(TARGETS debugfs.plugin
            COMPONENT debugfs_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_APPS)
    pkg_check_modules(CAP QUIET libcap)

    set(APPS_PLUGIN_FILES
            src/collectors/apps.plugin/apps_plugin.c
            src/collectors/apps.plugin/apps_plugin.h
            src/collectors/apps.plugin/apps_functions.c
            src/collectors/apps.plugin/apps_targets.c
            src/collectors/apps.plugin/apps_users_and_groups.c
            src/collectors/apps.plugin/apps_output.c
            src/collectors/apps.plugin/apps_proc_pid_status.c
            src/collectors/apps.plugin/apps_proc_pid_limits.c
            src/collectors/apps.plugin/apps_proc_pid_stat.c
            src/collectors/apps.plugin/apps_proc_pid_cmdline.c
            src/collectors/apps.plugin/apps_proc_pid_io.c
            src/collectors/apps.plugin/apps_proc_stat.c
            src/collectors/apps.plugin/apps_proc_pid_fd.c
            src/collectors/apps.plugin/apps_proc_pids.c
            src/collectors/apps.plugin/apps_proc_meminfo.c
    )

    add_executable(apps.plugin ${APPS_PLUGIN_FILES})
    target_link_libraries(apps.plugin libnetdata ${CAP_LIBRARIES})
    target_include_directories(apps.plugin PRIVATE ${CAP_INCLUDE_DIRS})
    target_compile_options(apps.plugin PRIVATE ${CAP_CFLAGS_OTHER})

    install(TARGETS apps.plugin
            COMPONENT apps_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
    install(FILES src/collectors/apps.plugin/apps_groups.conf
            COMPONENT apps_plugin
            DESTINATION usr/lib/netdata/conf.d)
endif()

if(CAP_FOUND)
    set(HAVE_CAPABILITY True)
endif()

if(ENABLE_PLUGIN_FREEIPMI)
    pkg_check_modules(IPMI REQUIRED libipmimonitoring)

    set(FREEIPMI_PLUGIN_FILES src/collectors/freeipmi.plugin/freeipmi_plugin.c)

    add_executable(freeipmi.plugin ${FREEIPMI_PLUGIN_FILES})
    target_link_libraries (freeipmi.plugin libnetdata ${IPMI_LIBRARIES})
    target_include_directories(freeipmi.plugin PRIVATE ${IPMI_INCLUDE_DIRS})
    target_compile_options(freeipmi.plugin PRIVATE ${IPMI_CFLAGS_OTHER})

    install(TARGETS freeipmi.plugin
            COMPONENT freeipmi_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_NFACCT)
    if (NOT MNL_FOUND)
        message(FATAL_ERROR "Can not build nfacct.plugin because MNL library could not be found.")
    endif()

    pkg_check_modules(NFACCT REQUIRED libnetfilter_acct)

    set(NFACCT_PLUGIN_FILES src/collectors/nfacct.plugin/plugin_nfacct.c)

    add_executable(nfacct.plugin ${NFACCT_PLUGIN_FILES})
    target_link_libraries (nfacct.plugin libnetdata ${MNL_LIBRARIES} ${NFACCT_LIBRARIES})
    target_include_directories(nfacct.plugin PRIVATE ${MNL_INCLUDE_DIRS} ${NFACCT_INCLUDE_DIRS})
    target_compile_options(nfacct.plugin PRIVATE ${MNL_CFLAGS_OTHER} ${NFACCT_CFLAGS_OTHER})

    install(TARGETS nfacct.plugin
            COMPONENT nfacct_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_XENSTAT)
    pkg_check_modules(XENSTAT REQUIRED xenstat)
    pkg_check_modules(XENLIGHT REQUIRED xenlight)

    set(XENSTAT_PLUGIN_FILES src/collectors/xenstat.plugin/xenstat_plugin.c)

    add_executable(xenstat.plugin ${XENSTAT_PLUGIN_FILES})
    target_link_libraries (xenstat.plugin libnetdata ${XENLIGHT_LIBRARIES} ${XENSTAT_LIBRARIES})
    target_include_directories(xenstat.plugin PRIVATE ${XENLIGHT_INCLUDE_DIRS} ${XENSTAT_INCLUDE_DIRS})
    target_compile_options(xenstat.plugin PRIVATE ${XENLIGHT_CFLAGS_OTHER} ${XENSTAT_CFLAGS_OTHER})

    install(TARGETS xenstat.plugin
            COMPONENT xenstat_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_PERF)
    set(PERF_PLUGIN_FILES src/collectors/perf.plugin/perf_plugin.c)

    add_executable(perf.plugin ${PERF_PLUGIN_FILES})
    target_link_libraries(perf.plugin libnetdata)

    install(TARGETS perf.plugin
            COMPONENT perf_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_SLABINFO)
    set(SLABINFO_PLUGIN_FILES src/collectors/slabinfo.plugin/slabinfo.c)

    add_executable(slabinfo.plugin ${SLABINFO_PLUGIN_FILES})
    target_link_libraries(slabinfo.plugin libnetdata)

    install(TARGETS slabinfo.plugin
            COMPONENT slabinfo_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_LOGS_MANAGEMENT)
    set(ENABLE_LOGSMANAGEMENT True)

    if(ENABLE_LOGS_MANAGEMENT_TESTS)
        list(APPEND LOGS_MANAGEMENT_PLUGIN_FILES src/logsmanagement/unit_test/unit_test.c
                                                 src/logsmanagement/unit_test/unit_test.h)
        set(ENABLE_LOGSMANAGEMENT_TESTS True)
    endif()

    add_executable(logs-management.plugin ${LOGS_MANAGEMENT_PLUGIN_FILES})
    target_link_libraries(logs-management.plugin libnetdata)

    install(TARGETS logs-management.plugin
            COMPONENT logs_management_plugin
            DESTINATION usr/libexec/netdata/plugins.d)

    install(DIRECTORY src/logsmanagement/stock_conf/logsmanagement.d
            COMPONENT logs_management_plugin
            DESTINATION usr/lib/netdata/conf.d)

    install(DIRECTORY DESTINATION etc/netdata/logsmanagement.d)
endif()

if(ENABLE_PLUGIN_CUPS)
    pkg_check_modules(CUPS libcups)
    if(NOT CUPS_LIBRARIES)
        pkg_check_modules(CUPS cups)
        if(NOT CUPS_LIBRARIES)
                find_program(CUPS_CONFIG cups-config)
                if(CUPS_CONFIG)
                        execute_process(COMMAND ${CUPS_CONFIG} --api-version OUTPUT_VARIABLE CUPS_API_VERSION OUTPUT_STRIP_TRAILING_WHITESPACE)
                        if(CUPS_API_VERSION VERSION_LESS "1.7")
                                set(CUPS_FOUND False)
                        else()
                                execute_process(COMMAND ${CUPS_CONFIG} --cflags OUTPUT_VARIABLE CUPS_CFLAGS_OTHER OUTPUT_STRIP_TRAILING_WHITESPACE)
                                execute_process(COMMAND ${CUPS_CONFIG} --libs OUTPUT_VARIABLE CUPS_LIBRARIES OUTPUT_STRIP_TRAILING_WHITESPACE)
                                set(CUPS_FOUND True)
                        endif()
                endif()
        endif()
    endif()

    if(NOT CUPS_FOUND)
        message(WARNING "Could not find cups cflags and libs.")
    else()
        set(CUPS_PLUGIN_FILES src/collectors/cups.plugin/cups_plugin.c)
        add_executable(cups.plugin ${CUPS_PLUGIN_FILES})
        target_link_libraries (cups.plugin libnetdata ${CUPS_LIBRARIES})
        target_compile_options(cups.plugin PRIVATE ${CUPS_CFLAGS_OTHER})

        install(TARGETS cups.plugin
                COMPONENT cups_plugin
                DESTINATION usr/libexec/netdata/plugins.d)
    endif()
endif()

set(NDSUDO_FILES src/collectors/plugins.d/ndsudo.c)

add_executable(ndsudo ${NDSUDO_FILES})

install(TARGETS ndsudo
        COMPONENT ndsudo
        DESTINATION usr/libexec/netdata/plugins.d)

if(ENABLE_PLUGIN_CGROUP_NETWORK)
    set(CGROUP_NETWORK_FILES src/collectors/cgroups.plugin/cgroup-network.c)

    add_executable(cgroup-network ${CGROUP_NETWORK_FILES})
    target_link_libraries(cgroup-network libnetdata)

    install(TARGETS cgroup-network
            COMPONENT cgroup_network_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_SYSTEMD_JOURNAL)
        if(NOT SYSTEMD_FOUND)
                message(FATAL_ERROR "Systemd journal plugin requires systemd, but systemd was not found.")
        endif()

        add_executable(systemd-journal.plugin ${SYSTEMD_JOURNAL_PLUGIN_FILES})
        target_link_libraries(systemd-journal.plugin libnetdata)

        install(TARGETS systemd-journal.plugin
                COMPONENT systemd_journal_plugin
                DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_EBPF)
    set(EBPF_PLUGIN_FILES
            src/collectors/ebpf.plugin/ebpf.c
            src/collectors/ebpf.plugin/ebpf.h
            src/collectors/ebpf.plugin/ebpf_cachestat.c
            src/collectors/ebpf.plugin/ebpf_cachestat.h
            src/collectors/ebpf.plugin/ebpf_dcstat.c
            src/collectors/ebpf.plugin/ebpf_dcstat.h
            src/collectors/ebpf.plugin/ebpf_disk.c
            src/collectors/ebpf.plugin/ebpf_disk.h
            src/collectors/ebpf.plugin/ebpf_fd.c
            src/collectors/ebpf.plugin/ebpf_fd.h
            src/collectors/ebpf.plugin/ebpf_hardirq.c
            src/collectors/ebpf.plugin/ebpf_hardirq.h
            src/collectors/ebpf.plugin/ebpf_mdflush.c
            src/collectors/ebpf.plugin/ebpf_mdflush.h
            src/collectors/ebpf.plugin/ebpf_mount.c
            src/collectors/ebpf.plugin/ebpf_mount.h
            src/collectors/ebpf.plugin/ebpf_filesystem.c
            src/collectors/ebpf.plugin/ebpf_filesystem.h
            src/collectors/ebpf.plugin/ebpf_oomkill.c
            src/collectors/ebpf.plugin/ebpf_oomkill.h
            src/collectors/ebpf.plugin/ebpf_process.c
            src/collectors/ebpf.plugin/ebpf_process.h
            src/collectors/ebpf.plugin/ebpf_shm.c
            src/collectors/ebpf.plugin/ebpf_shm.h
            src/collectors/ebpf.plugin/ebpf_socket.c
            src/collectors/ebpf.plugin/ebpf_socket.h
            src/collectors/ebpf.plugin/ebpf_softirq.c
            src/collectors/ebpf.plugin/ebpf_softirq.h
            src/collectors/ebpf.plugin/ebpf_sync.c
            src/collectors/ebpf.plugin/ebpf_sync.h
            src/collectors/ebpf.plugin/ebpf_swap.c
            src/collectors/ebpf.plugin/ebpf_swap.h
            src/collectors/ebpf.plugin/ebpf_vfs.c
            src/collectors/ebpf.plugin/ebpf_vfs.h
            src/collectors/ebpf.plugin/ebpf_apps.c
            src/collectors/ebpf.plugin/ebpf_apps.h
            src/collectors/ebpf.plugin/ebpf_cgroup.c
            src/collectors/ebpf.plugin/ebpf_cgroup.h
            src/collectors/ebpf.plugin/ebpf_unittest.c
            src/collectors/ebpf.plugin/ebpf_unittest.h
            src/collectors/ebpf.plugin/ebpf_functions.c
            src/collectors/ebpf.plugin/ebpf_functions.h
    )

    add_executable(ebpf.plugin ${EBPF_PLUGIN_FILES})
    target_link_libraries(ebpf.plugin libnetdata)

    netdata_add_ebpf_co_re_to_target(ebpf.plugin)

    install(TARGETS ebpf.plugin
            COMPONENT ebpf_plugin
            DESTINATION usr/libexec/netdata/plugins.d)

    if(ENABLE_LEGACY_EBPF_PROGRAMS)
        netdata_install_legacy_ebpf_code()
    endif()
endif()

if(ENABLE_PLUGIN_LOCAL_LISTENERS)
        set(LOCAL_LISTENERS_FILES
                src/collectors/plugins.d/local_listeners.c
                src/libnetdata/maps/local-sockets.h
        )

        add_executable(local-listeners ${LOCAL_LISTENERS_FILES})

        target_compile_options(local-listeners PRIVATE
                               "$<$<BOOL:${MNL_FOUND}>:${MNL_CFLAGS_OTHER}>")
        target_include_directories(local-listeners PRIVATE
                                   "$<$<BOOL:${MNL_FOUND}>:${MNL_INCLUDE_DIRS}>")
        target_link_libraries(local-listeners libnetdata
                              "$<$<BOOL:${MNL_FOUND}>:${MNL_LIBRARIES}>")

        install(TARGETS local-listeners
                COMPONENT local_listeners
                DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_NETWORK_VIEWER)
        set(NETWORK_VIEWER_FILES
                src/libnetdata/maps/local-sockets.h
                src/libnetdata/maps/system-users.h
                src/collectors/network-viewer.plugin/network-viewer.c
        )

        add_executable(network-viewer.plugin ${NETWORK_VIEWER_FILES})

        target_compile_options(network-viewer.plugin PRIVATE
                               "$<$<BOOL:${MNL_FOUND}>:${MNL_CFLAGS_OTHER}>")
        target_include_directories(network-viewer.plugin PRIVATE
                                   "$<$<BOOL:${MNL_FOUND}>:${MNL_INCLUDE_DIRS}>")
        target_link_libraries(network-viewer.plugin libnetdata
                              "$<$<BOOL:${MNL_FOUND}>:${MNL_LIBRARIES}>")


        install(TARGETS network-viewer.plugin
                COMPONENT network_viewer_plugin
                DESTINATION usr/libexec/netdata/plugins.d)
endif()

#
# exporters
#

if(ENABLE_EXPORTER_MONGODB)
        pkg_check_modules(MONGOC libmongoc-1.0>=1.7)

        if(MONGOC_FOUND)
                SET(HAVE_MONGOC True)
        else()
                SET(ENABLE_EXPORTER_MONGODB False)
        endif()
endif()

if(ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE)
        pkg_check_modules(SNAPPY snappy)
        if (NOT SNAPPY_FOUND)
                include(CheckLibraryExists)
                check_library_exists(snappy snappy_compress "" HAVE_SNAPPY_LIB)

                if(HAVE_SNAPPY_LIB)
                        set(SNAPPY_INCLUDE_DIRS "")
                        set(SNAPPY_CFLAGS_OTHER "")
                        set(SNAPPY_LIBRARIES "-lsnappy")
                else()
                        message(FATAL_ERROR "Could not find snappy libraries with pkg-config or internal cmake checks.")
                endif()
        endif()

        netdata_protoc_generate_cpp("${CMAKE_SOURCE_DIR}/src/exporting/prometheus/remote_write"
                                    "${CMAKE_SOURCE_DIR}/src/exporting/prometheus/remote_write"
                                    PROMETHEUS_REMOTE_WRITE_BUILT_SRCS
                                    PROMETHEUS_REMOTE_WRITE_BUILT_HDRS
                                    "src/exporting/prometheus/remote_write/remote_write.proto")

        list(APPEND PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES
                    ${PROMETHEUS_REMOTE_WRITE_BUILT_SRCS}
                    ${PROMETHEUS_REMOTE_WRITE_BUILT_HDRS})

        set(ENABLE_PROMETHEUS_REMOTE_WRITE True)
endif()

#
# build netdata (only Linux ATM)
#

add_executable(netdata
        ${NETDATA_FILES}
        "$<$<BOOL:${ENABLE_ACLK}>:${ACLK_FILES}>"
        "$<$<BOOL:${ENABLE_H2O}>:${H2O_FILES}>"
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGODB_EXPORTING_FILES}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES}>"
)

target_compile_definitions(netdata PRIVATE
        "$<$<BOOL:${ENABLE_ML}>:DLIB_NO_GUI_SUPPORT>"
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGOC_CFLAGS_OTHER}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${SNAPPY_CFLAGS_OTHER}>"
)

target_include_directories(netdata PRIVATE
        "$<$<BOOL:${ENABLE_ACLK}>:${CMAKE_SOURCE_DIR}/src/aclk/aclk-schemas>"
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGOC_INCLUDE_DIRS}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${SNAPPY_INCLUDE_DIRS}>"
)

target_link_libraries(netdata PRIVATE
        m
        libnetdata
        "$<$<BOOL:${LINUX}>:rt>"
        "$<$<BOOL:${ENABLE_MQTTWEBSOCKETS}>:mqttwebsockets>"
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGOC_LIBRARIES}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${SNAPPY_LIBRARIES}>"
        "$<$<BOOL:${MACOS}>:${IOKIT};${FOUNDATION}>"
        "$<$<BOOL:${ENABLE_SENTRY}>:sentry>"
        "$<$<BOOL:${ENABLE_WEBRTC}>:LibDataChannel::LibDataChannelStatic>"
        "$<$<BOOL:${ENABLE_H2O}>:h2o>"
)

if(NEED_PROTOBUF)
        netdata_add_protobuf(netdata)
endif()

#
# build systemd-cat-native
#
set(SYSTEMD_CAT_NATIVE_FILES src/libnetdata/log/systemd-cat-native.c
                             src/libnetdata/log/systemd-cat-native.h)

add_executable(systemd-cat-native ${SYSTEMD_CAT_NATIVE_FILES})
target_link_libraries(systemd-cat-native libnetdata)

install(TARGETS systemd-cat-native
        COMPONENT systemd-cat-native
        DESTINATION usr/sbin)

#
# build log2journal
#

pkg_check_modules(PCRE2 libpcre2-8)

if(PCRE2_FOUND)
        set(LOG2JOURNAL_FILES
                ${CONFIG_H}
                src/collectors/log2journal/log2journal.h
                src/collectors/log2journal/log2journal.c
                src/collectors/log2journal/log2journal-help.c
                src/collectors/log2journal/log2journal-yaml.c
                src/collectors/log2journal/log2journal-json.c
                src/collectors/log2journal/log2journal-logfmt.c
                src/collectors/log2journal/log2journal-pcre2.c
                src/collectors/log2journal/log2journal-params.c
                src/collectors/log2journal/log2journal-inject.c
                src/collectors/log2journal/log2journal-pattern.c
                src/collectors/log2journal/log2journal-replace.c
                src/collectors/log2journal/log2journal-rename.c
                src/collectors/log2journal/log2journal-rewrite.c
        )

        add_executable(log2journal ${LOG2JOURNAL_FILES})
        target_include_directories(log2journal BEFORE PUBLIC ${CONFIG_H_DIR} ${CMAKE_SOURCE_DIR}/src ${PCRE2_INCLUDE_DIRS})
        target_compile_definitions(log2journal PUBLIC ${PCRE2_CFLAGS_OTHER})
        target_link_libraries(log2journal PUBLIC "${PCRE2_LDFLAGS}")

        netdata_add_libyaml_to_target(log2journal)

        install(TARGETS log2journal
                COMPONENT log2journal
                DESTINATION usr/sbin)

        install(DIRECTORY src/collectors/log2journal/log2journal.d
                COMPONENT log2journal
                DESTINATION usr/lib/netdata/conf.d)
endif()

#
# build netdatacli
#

set(NETDATACLI_FILES
        src/daemon/commands.h
        src/daemon/pipename.c
        src/daemon/pipename.h
        src/cli/cli.c
)

add_executable(netdatacli ${NETDATACLI_FILES})
target_link_libraries(netdatacli libnetdata)

install(TARGETS netdatacli
        COMPONENT netdatacli
        DESTINATION usr/sbin)

#
# Build go.d.plugin
#

if(ENABLE_PLUGIN_GO)
    add_go_target(go-plugin go.d.plugin src/go/collectors/go.d.plugin cmd/godplugin)

    install(PROGRAMS ${CMAKE_BINARY_DIR}/go.d.plugin
            COMPONENT go.d.plugin
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

#
# Generate config file
#

if(NOT CMAKE_INSTALL_PREFIX STREQUAL "")
  string(REGEX REPLACE "/$" "" CMAKE_INSTALL_PREFIX "${CMAKE_INSTALL_PREFIX}")
endif()

set(CACHE_DIR "${CMAKE_INSTALL_PREFIX}/var/cache/netdata")
set(CONFIG_DIR "${CMAKE_INSTALL_PREFIX}/etc/netdata")
set(LIBCONFIG_DIR "${CMAKE_INSTALL_PREFIX}/usr/lib/netdata/conf.d")
set(LOG_DIR "${CMAKE_INSTALL_PREFIX}/var/log/netdata")
set(PLUGINS_DIR "${CMAKE_INSTALL_PREFIX}/usr/libexec/netdata/plugins.d")
set(VARLIB_DIR "${CMAKE_INSTALL_PREFIX}/var/lib/netdata")

# A non-default value is only used when building Debian packages (/var/lib/netdata/www)
if(NOT DEFINED WEB_DIR)
  set(WEB_DIR "usr/share/netdata/web")
else()
  string(REGEX REPLACE "^/" "" WEB_DIR "${WEB_DIR}")
endif()
set(WEB_DEST "${WEB_DIR}")
set(WEB_DIR "${CMAKE_INSTALL_PREFIX}/${WEB_DEST}")


set(CONFIGURE_COMMAND "dummy-configure-command")
if (NOT NETDATA_USER)
        set(NETDATA_USER "netdata")
endif()
set(VERSION "${GIT_DESCRIBE_OUTPUT}")

configure_file(packaging/cmake/config.cmake.h.in config.h)

#
# install
#

install(TARGETS netdata DESTINATION usr/sbin)

install(DIRECTORY DESTINATION var/cache/netdata)
install(DIRECTORY DESTINATION var/log/netdata)
install(DIRECTORY DESTINATION var/lib/netdata/registry)
install(DIRECTORY DESTINATION var/lib/netdata/cloud.d)
install(DIRECTORY DESTINATION var/run)
install(DIRECTORY DESTINATION etc/netdata)
install(DIRECTORY DESTINATION etc/netdata/charts.d)
install(DIRECTORY DESTINATION etc/netdata/custom-plugins.d)
install(DIRECTORY DESTINATION etc/netdata/ebpf.d)
install(DIRECTORY DESTINATION etc/netdata/go.d)
install(DIRECTORY DESTINATION etc/netdata/health.d)
install(DIRECTORY DESTINATION etc/netdata/python.d)
install(DIRECTORY DESTINATION etc/netdata/ssl)
install(DIRECTORY DESTINATION etc/netdata/statsd.d)
install(DIRECTORY DESTINATION usr/lib/netdata/conf.d)
install(DIRECTORY DESTINATION usr/lib/netdata/conf.d/schema.d)
install(DIRECTORY DESTINATION usr/libexec/netdata/plugins.d)
install(DIRECTORY DESTINATION ${WEB_DEST})

set(libsysdir_POST "${CMAKE_INSTALL_PREFIX}/usr/lib/netdata/system")
set(pkglibexecdir_POST "${CMAKE_INSTALL_PREFIX}/usr/libexec/netdata")
set(localstatedir_POST "${CMAKE_INSTALL_PREFIX}/var")
set(sbindir_POST "${CMAKE_INSTALL_PREFIX}/usr/sbin")
set(configdir_POST "${CMAKE_INSTALL_PREFIX}/etc/netdata")
set(libconfigdir_POST "${CMAKE_INSTALL_PREFIX}/usr/lib/netdata/conf.d")
set(cachedir_POST "${CMAKE_INSTALL_PREFIX}/var/cache/netdata")
set(registrydir_POST "${CMAKE_INSTALL_PREFIX}/var/lib/netdata/registry")
set(varlibdir_POST "${CMAKE_INSTALL_PREFIX}/var/lib/netdata")
set(netdata_user_POST "${NETDATA_USER}")

# netdata-claim.sh
if(ENABLE_CLOUD)
        set(enable_cloud_POST "yes")
else()
        set(enable_cloud_POST "no")
endif()

if(ENABLE_ACLK)
        set(enable_aclk_POST "yes")
else()
        set(enable_aclk_POST "no")
endif()

configure_file(src/claim/netdata-claim.sh.in src/claim/netdata-claim.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/claim/netdata-claim.sh
        DESTINATION usr/sbin)

#
# We don't check ENABLE_PLUGIN_CGROUP_NETWORK because rpm builds assume
# the files exists unconditionally.
#
configure_file(src/collectors/cgroups.plugin/cgroup-network-helper.sh.in
               src/collectors/cgroups.plugin/cgroup-network-helper.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/collectors/cgroups.plugin/cgroup-network-helper.sh
        COMPONENT cgroup_network_plugin
        DESTINATION usr/libexec/netdata/plugins.d)

configure_file(src/collectors/cgroups.plugin/cgroup-name.sh.in
               src/collectors/cgroups.plugin/cgroup-name.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/collectors/cgroups.plugin/cgroup-name.sh
        DESTINATION usr/libexec/netdata/plugins.d)

#
# statsd
#
install(FILES
        src/collectors/statsd.plugin/asterisk.conf
        src/collectors/statsd.plugin/example.conf
        src/collectors/statsd.plugin/k6.conf
        DESTINATION usr/lib/netdata/conf.d/statsd.d)

#
# exporting
#
install(FILES
        src/exporting/exporting.conf
        DESTINATION usr/lib/netdata/conf.d)

#
# ioping.plugin
#
install(FILES
        src/collectors/ioping.plugin/ioping.conf
        DESTINATION usr/lib/netdata/conf.d)

#
# streaming
#
install(FILES
        src/streaming/stream.conf
        DESTINATION usr/lib/netdata/conf.d)

#
# swagger
#
install(FILES
        src/web/api/netdata-swagger.json
        src/web/api/netdata-swagger.yaml
        DESTINATION ${WEB_DEST})

#
# service files
#

configure_file(system/install-service.sh.in system/install-service.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/system/install-service.sh
        DESTINATION usr/libexec/netdata)

configure_file(system/cron/netdata-updater-daily.in system/cron/netdata-updater-daily @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/cron/netdata-updater-daily
        DESTINATION usr/lib/netdata/system/cron)

configure_file(system/launchd/netdata.plist.in system/launchd/netdata.plist @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/launchd/netdata.plist
        DESTINATION usr/lib/netdata/system/launchd)

configure_file(system/freebsd/rc.d/netdata.in system/freebsd/rc.d/netdata @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/system/freebsd/rc.d/netdata
        DESTINATION usr/lib/netdata/system/freebsd/rc.d)

configure_file(system/initd/init.d/netdata.in system/initd/init.d/netdata @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/system/initd/init.d/netdata
        DESTINATION usr/lib/netdata/system/initd/init.d)

configure_file(system/logrotate/netdata.in system/logrotate/netdata @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/logrotate/netdata
        DESTINATION usr/lib/netdata/system/logrotate)

configure_file(system/lsb/init.d/netdata.in system/lsb/init.d/netdata @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/system/lsb/init.d/netdata
        DESTINATION usr/lib/netdata/system/lsb/init.d)

configure_file(system/openrc/conf.d/netdata.in system/openrc/conf.d/netdata @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/openrc/conf.d/netdata
        DESTINATION usr/lib/netdata/system/openrc/conf.d)

configure_file(system/openrc/init.d/netdata.in system/openrc/init.d/netdata @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/system/openrc/init.d/netdata
        DESTINATION usr/lib/netdata/system/openrc/init.d)

configure_file(system/runit/run.in system/runit/run @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/system/runit/run
        DESTINATION usr/lib/netdata/system/runit)

configure_file(system/dinit/netdata.in system/dinit/netdata @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/dinit/netdata
        DESTINATION usr/lib/netdata/system/dinit)

configure_file(system/systemd/netdata.service.in system/systemd/netdata.service @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/systemd/netdata.service
        DESTINATION usr/lib/netdata/system/systemd)

configure_file(system/systemd/netdata.service.v235.in system/systemd/netdata.service.v235 @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/systemd/netdata.service.v235
        DESTINATION usr/lib/netdata/system/systemd)

configure_file(system/systemd/netdata-updater.service.in system/systemd/netdata-updater.service @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/systemd/netdata-updater.service
        DESTINATION usr/lib/netdata/system/systemd)

install(FILES
        system/systemd/netdata-updater.timer
        DESTINATION usr/lib/netdata/system/systemd)
install(FILES
        system/systemd/50-netdata.preset
        DESTINATION usr/lib/netdata/system/systemd)

install(FILES
        system/vnodes/vnodes.conf
        DESTINATION usr/lib/netdata/conf.d/vnodes)

install(FILES
        system/.install-type
        DESTINATION etc/netdata)
install(FILES
        system/netdata-updater.conf
        DESTINATION etc/netdata)

install(PROGRAMS
        system/edit-config
        DESTINATION etc/netdata)

#
# TODO: check the following files for correct substitutions
#
configure_file(src/daemon/anonymous-statistics.sh.in src/daemon/anonymous-statistics.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/daemon/anonymous-statistics.sh
        DESTINATION usr/libexec/netdata/plugins.d)

configure_file(src/daemon/get-kubernetes-labels.sh.in src/daemon/get-kubernetes-labels.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/daemon/get-kubernetes-labels.sh
        DESTINATION usr/libexec/netdata/plugins.d)

install(PROGRAMS
        src/daemon/system-info.sh
        DESTINATION usr/libexec/netdata/plugins.d)

#
# health files
#

file(GLOB_RECURSE HEALTH_CONF_FILES "src/health/health.d/*.conf")
install(FILES
        ${HEALTH_CONF_FILES}
        DESTINATION usr/lib/netdata/conf.d/health.d)

configure_file(src/health/notifications/alarm-notify.sh.in src/health/notifications/alarm-notify.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/health/notifications/alarm-notify.sh
        DESTINATION usr/libexec/netdata/plugins.d)

install(PROGRAMS
        src/health/notifications/alarm-email.sh
        src/health/notifications/alarm-test.sh
        DESTINATION usr/libexec/netdata/plugins.d)

install(FILES
        src/health/notifications/health_alarm_notify.conf
        src/health/notifications/health_email_recipients.conf
        DESTINATION usr/lib/netdata/conf.d)
#
# test/ files
#

configure_file(tests/health_mgmtapi/health-cmdapi-test.sh.in tests/health_mgmtapi/health-cmdapi-test.sh @ONLY)
configure_file(tests/acls/acl.sh.in tests/acls/acl.sh @ONLY)
configure_file(tests/urls/request.sh.in tests/urls/request.sh @ONLY)
configure_file(tests/alarm_repetition/alarm.sh.in tests/alarm_repetition/alarm.sh @ONLY)
configure_file(tests/template_dimension/template_dim.sh.in tests/template_dimension/template_dim.sh @ONLY)
configure_file(tests/ebpf/ebpf_thread_function.sh.in tests/ebpf/ebpf_thread_function.sh @ONLY)

install(FILES
        ${CMAKE_BINARY_DIR}/tests/health_mgmtapi/health-cmdapi-test.sh
        ${CMAKE_BINARY_DIR}/tests/acls/acl.sh
        ${CMAKE_BINARY_DIR}/tests/urls/request.sh
        ${CMAKE_BINARY_DIR}/tests/alarm_repetition/alarm.sh
        ${CMAKE_BINARY_DIR}/tests/template_dimension/template_dim.sh
        ${CMAKE_BINARY_DIR}/tests/ebpf/ebpf_thread_function.sh
        DESTINATION usr/libexec/netdata/plugins.d)

#
# charts.d files
#

configure_file(src/collectors/charts.d.plugin/charts.d.plugin.in src/collectors/charts.d.plugin/charts.d.plugin @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/collectors/charts.d.plugin/charts.d.plugin
        DESTINATION usr/libexec/netdata/plugins.d)

install(FILES
        src/collectors/charts.d.plugin/charts.d.dryrun-helper.sh
        src/collectors/charts.d.plugin/loopsleepms.sh.inc
        DESTINATION usr/libexec/netdata/plugins.d)

install(FILES
        src/collectors/charts.d.plugin/charts.d.conf
        DESTINATION usr/lib/netdata/conf.d)

#
# tc-qos-helper
#

configure_file(src/collectors/tc.plugin/tc-qos-helper.sh.in src/collectors/tc.plugin/tc-qos-helper.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/collectors/tc.plugin/tc-qos-helper.sh
        DESTINATION usr/libexec/netdata/plugins.d)

# scripts

install(FILES
        src/collectors/charts.d.plugin/ap/ap.chart.sh
        src/collectors/charts.d.plugin/apcupsd/apcupsd.chart.sh
        src/collectors/charts.d.plugin/example/example.chart.sh
        src/collectors/charts.d.plugin/libreswan/libreswan.chart.sh
        src/collectors/charts.d.plugin/opensips/opensips.chart.sh
        src/collectors/charts.d.plugin/sensors/sensors.chart.sh
        DESTINATION usr/libexec/netdata/charts.d)

# confs

install(FILES
        src/collectors/charts.d.plugin/ap/ap.conf
        src/collectors/charts.d.plugin/apcupsd/apcupsd.conf
        src/collectors/charts.d.plugin/example/example.conf
        src/collectors/charts.d.plugin/libreswan/libreswan.conf
        src/collectors/charts.d.plugin/opensips/opensips.conf
        src/collectors/charts.d.plugin/sensors/sensors.conf
        DESTINATION usr/lib/netdata/conf.d/charts.d)

install(FILES
        src/collectors/systemd-journal.plugin/schema.d/systemd-journal:monitored-directories.json
        src/health/schema.d/health:alert:prototype.json
        DESTINATION usr/lib/netdata/conf.d/schema.d)

#
# ebpf files
#

if(ENABLE_PLUGIN_EBPF)
        install(FILES
                src/collectors/ebpf.plugin/ebpf.d.conf
                DESTINATION usr/lib/netdata/conf.d)

        install(FILES
                src/collectors/ebpf.plugin/ebpf.d/cachestat.conf
                src/collectors/ebpf.plugin/ebpf.d/dcstat.conf
                src/collectors/ebpf.plugin/ebpf.d/disk.conf
                src/collectors/ebpf.plugin/ebpf.d/ebpf_kernel_reject_list.txt
                src/collectors/ebpf.plugin/ebpf.d/fd.conf
                src/collectors/ebpf.plugin/ebpf.d/filesystem.conf
                src/collectors/ebpf.plugin/ebpf.d/hardirq.conf
                src/collectors/ebpf.plugin/ebpf.d/mdflush.conf
                src/collectors/ebpf.plugin/ebpf.d/mount.conf
                src/collectors/ebpf.plugin/ebpf.d/network.conf
                src/collectors/ebpf.plugin/ebpf.d/oomkill.conf
                src/collectors/ebpf.plugin/ebpf.d/process.conf
                src/collectors/ebpf.plugin/ebpf.d/shm.conf
                src/collectors/ebpf.plugin/ebpf.d/softirq.conf
                src/collectors/ebpf.plugin/ebpf.d/swap.conf
                src/collectors/ebpf.plugin/ebpf.d/sync.conf
                src/collectors/ebpf.plugin/ebpf.d/vfs.conf
                DESTINATION usr/lib/netdata/conf.d/ebpf.d)
endif()

#
# python.d files
#

configure_file(src/collectors/python.d.plugin/python.d.plugin.in src/collectors/python.d.plugin/python.d.plugin @ONLY)
install(PROGRAMS ${CMAKE_BINARY_DIR}/src/collectors/python.d.plugin/python.d.plugin
        DESTINATION usr/libexec/netdata/plugins.d)

install(DIRECTORY src/collectors/python.d.plugin/python_modules
        DESTINATION usr/libexec/netdata/python.d)

install(FILES src/collectors/python.d.plugin/python.d.conf
        DESTINATION usr/lib/netdata/conf.d)

# conf files

install(FILES
        src/collectors/python.d.plugin/alarms/alarms.conf
        src/collectors/python.d.plugin/am2320/am2320.conf
        src/collectors/python.d.plugin/anomalies/anomalies.conf
        src/collectors/python.d.plugin/beanstalk/beanstalk.conf
        src/collectors/python.d.plugin/bind_rndc/bind_rndc.conf
        src/collectors/python.d.plugin/boinc/boinc.conf
        src/collectors/python.d.plugin/ceph/ceph.conf
        src/collectors/python.d.plugin/changefinder/changefinder.conf
        src/collectors/python.d.plugin/dovecot/dovecot.conf
        src/collectors/python.d.plugin/example/example.conf
        src/collectors/python.d.plugin/exim/exim.conf
        src/collectors/python.d.plugin/gearman/gearman.conf
        src/collectors/python.d.plugin/go_expvar/go_expvar.conf
        src/collectors/python.d.plugin/haproxy/haproxy.conf
        src/collectors/python.d.plugin/hpssa/hpssa.conf
        src/collectors/python.d.plugin/icecast/icecast.conf
        src/collectors/python.d.plugin/ipfs/ipfs.conf
        src/collectors/python.d.plugin/litespeed/litespeed.conf
        src/collectors/python.d.plugin/memcached/memcached.conf
        src/collectors/python.d.plugin/monit/monit.conf
        src/collectors/python.d.plugin/nsd/nsd.conf
        src/collectors/python.d.plugin/nvidia_smi/nvidia_smi.conf
        src/collectors/python.d.plugin/openldap/openldap.conf
        src/collectors/python.d.plugin/oracledb/oracledb.conf
        src/collectors/python.d.plugin/pandas/pandas.conf
        src/collectors/python.d.plugin/postfix/postfix.conf
        src/collectors/python.d.plugin/puppet/puppet.conf
        src/collectors/python.d.plugin/rethinkdbs/rethinkdbs.conf
        src/collectors/python.d.plugin/retroshare/retroshare.conf
        src/collectors/python.d.plugin/riakkv/riakkv.conf
        src/collectors/python.d.plugin/samba/samba.conf
        src/collectors/python.d.plugin/smartd_log/smartd_log.conf
        src/collectors/python.d.plugin/spigotmc/spigotmc.conf
        src/collectors/python.d.plugin/squid/squid.conf
        src/collectors/python.d.plugin/tomcat/tomcat.conf
        src/collectors/python.d.plugin/tor/tor.conf
        src/collectors/python.d.plugin/traefik/traefik.conf
        src/collectors/python.d.plugin/uwsgi/uwsgi.conf
        src/collectors/python.d.plugin/varnish/varnish.conf
        src/collectors/python.d.plugin/w1sensor/w1sensor.conf
        src/collectors/python.d.plugin/zscores/zscores.conf
        DESTINATION usr/lib/netdata/conf.d/python.d)

# scripts

install(FILES
        src/collectors/python.d.plugin/alarms/alarms.chart.py
        src/collectors/python.d.plugin/am2320/am2320.chart.py
        src/collectors/python.d.plugin/anomalies/anomalies.chart.py
        src/collectors/python.d.plugin/beanstalk/beanstalk.chart.py
        src/collectors/python.d.plugin/bind_rndc/bind_rndc.chart.py
        src/collectors/python.d.plugin/boinc/boinc.chart.py
        src/collectors/python.d.plugin/ceph/ceph.chart.py
        src/collectors/python.d.plugin/changefinder/changefinder.chart.py
        src/collectors/python.d.plugin/dovecot/dovecot.chart.py
        src/collectors/python.d.plugin/example/example.chart.py
        src/collectors/python.d.plugin/exim/exim.chart.py
        src/collectors/python.d.plugin/gearman/gearman.chart.py
        src/collectors/python.d.plugin/go_expvar/go_expvar.chart.py
        src/collectors/python.d.plugin/haproxy/haproxy.chart.py
        src/collectors/python.d.plugin/hpssa/hpssa.chart.py
        src/collectors/python.d.plugin/icecast/icecast.chart.py
        src/collectors/python.d.plugin/ipfs/ipfs.chart.py
        src/collectors/python.d.plugin/litespeed/litespeed.chart.py
        src/collectors/python.d.plugin/memcached/memcached.chart.py
        src/collectors/python.d.plugin/monit/monit.chart.py
        src/collectors/python.d.plugin/nsd/nsd.chart.py
        src/collectors/python.d.plugin/nvidia_smi/nvidia_smi.chart.py
        src/collectors/python.d.plugin/openldap/openldap.chart.py
        src/collectors/python.d.plugin/oracledb/oracledb.chart.py
        src/collectors/python.d.plugin/pandas/pandas.chart.py
        src/collectors/python.d.plugin/postfix/postfix.chart.py
        src/collectors/python.d.plugin/puppet/puppet.chart.py
        src/collectors/python.d.plugin/rethinkdbs/rethinkdbs.chart.py
        src/collectors/python.d.plugin/retroshare/retroshare.chart.py
        src/collectors/python.d.plugin/riakkv/riakkv.chart.py
        src/collectors/python.d.plugin/samba/samba.chart.py
        src/collectors/python.d.plugin/smartd_log/smartd_log.chart.py
        src/collectors/python.d.plugin/spigotmc/spigotmc.chart.py
        src/collectors/python.d.plugin/squid/squid.chart.py
        src/collectors/python.d.plugin/tomcat/tomcat.chart.py
        src/collectors/python.d.plugin/tor/tor.chart.py
        src/collectors/python.d.plugin/traefik/traefik.chart.py
        src/collectors/python.d.plugin/uwsgi/uwsgi.chart.py
        src/collectors/python.d.plugin/varnish/varnish.chart.py
        src/collectors/python.d.plugin/w1sensor/w1sensor.chart.py
        src/collectors/python.d.plugin/zscores/zscores.chart.py
        DESTINATION usr/libexec/netdata/python.d)

# FIXME: don't install this unconditionally
configure_file(src/collectors/ioping.plugin/ioping.plugin.in src/collectors/ioping.plugin/ioping.plugin @ONLY)
install(PROGRAMS ${CMAKE_BINARY_DIR}/src/collectors/ioping.plugin/ioping.plugin DESTINATION usr/libexec/netdata/plugins.d)

#
# logs management
#
if (ENABLE_PLUGIN_LOGS_MANAGEMENT)
        configure_file(src/logsmanagement/stock_conf/logsmanagement.d.conf.in src/logsmanagement/stock_conf/logsmanagement.d.conf @ONLY)
        install(FILES ${CMAKE_BINARY_DIR}/src/logsmanagement/stock_conf/logsmanagement.d.conf
                COMPONENT logs_management_plugin
                DESTINATION usr/lib/netdata/conf.d)
endif()

#
# go.d.plugin
#
if(ENABLE_PLUGIN_GO)
    install(FILES src/go/collectors/go.d.plugin/config/go.d.conf
            COMPONENT go.d.plugin
            DESTINATION usr/lib/netdata/conf.d)

    install(DIRECTORY
            COMPONENT go.d.plugin
            DESTINATION usr/lib/netdata/conf.d/go.d)
    file(GLOB GO_CONF_FILES src/go/collectors/go.d.plugin/config/go.d/*.conf)
    install(FILES ${GO_CONF_FILES}
            COMPONENT go.d.plugin
            DESTINATION usr/lib/netdata/conf.d/go.d)

    install(DIRECTORY
            COMPONENT go.d.plugin
            DESTINATION usr/lib/netdata/conf.d/go.d/sd)
    file(GLOB GO_SD_CONF_FILES src/go/collectors/go.d.plugin/config/go.d/sd/*.conf)
    install(FILES ${GO_SD_CONF_FILES}
            COMPONENT go.d.plugin
            DESTINATION usr/lib/netdata/conf.d/go.d/sd)
endif()

#
# dashboard
#

include(src/web/gui/v1/dashboard_v1.cmake)
include(src/web/gui/v2/dashboard_v2.cmake)
include(src/web/gui/gui.cmake)

function(cat IN_FILE OUT_FILE)
        file(READ ${IN_FILE} CONTENTS)
        file(APPEND ${OUT_FILE} "${CONTENTS}")
endfunction()

file(WRITE ${CMAKE_BINARY_DIR}/src/web/gui/dashboard.js.in "")
foreach(JS_FILE ${DASHBOARD_JS_FILES})
  cat(${JS_FILE} ${CMAKE_BINARY_DIR}/dashboard.js.in)
endforeach()
configure_file(${CMAKE_BINARY_DIR}/dashboard.js.in
               ${CMAKE_BINARY_DIR}/dashboard.js COPYONLY)

install(FILES
        ${CMAKE_BINARY_DIR}/dashboard.js
        DESTINATION ${WEB_DEST})

install(FILES
        src/web/gui/dashboard_info_custom_example.js
        src/web/gui/dashboard_info.js
        src/web/gui/index.html
        src/web/gui/main.css
        src/web/gui/main.js
        src/web/gui/registry-access.html
        src/web/gui/registry-alert-redirect.html
        src/web/gui/registry-hello.html
        src/web/gui/switch.html
        src/web/gui/ilove.html
        DESTINATION ${WEB_DEST})

install(FILES
        src/web/gui/old/index.html
        DESTINATION ${WEB_DEST}/old)

install(FILES
        src/web/gui/static/img/netdata-logomark.svg
        DESTINATION ${WEB_DEST}/static/img)

install(FILES
        src/web/gui/css/morris-0.5.1.css
        src/web/gui/css/c3-0.4.18.min.css
        DESTINATION ${WEB_DEST}/css)

install(FILES
        src/web/gui/.well-known/dnt/cookies
        DESTINATION ${WEB_DEST}/.well-known/dnt)

# v0 dashboard
install(FILES
        src/web/gui/v0/index.html
        DESTINATION ${WEB_DEST}/v0)
