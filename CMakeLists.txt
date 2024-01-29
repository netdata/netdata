# SPDX-License-Identifier: GPL-3.0-or-later

cmake_minimum_required(VERSION 3.13.0)

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

#
# project
#

project(netdata
        VERSION ${FIELD_MAJOR}.${FIELD_MINOR}.${FIELD_PATCH}.${FIELD_TWEAK}
        DESCRIPTION "Netdata real-time monitoring"
        HOMEPAGE_URL "https://www.netdata.cloud"
        LANGUAGES C CXX)

find_package(PkgConfig REQUIRED)

set(CMAKE_C_STANDARD 11)
set(CMAKE_CXX_STANDARD 11)

set(CMAKE_C_STANDARD_REQUIRED On)
set(CMAKE_CXX_STANDARD_REQUIRED On)

set(SIZEOF_VOID_P ${CMAKE_SIZEOF_VOID_P})

if(NOT CMAKE_BUILD_TYPE)
        set(CMAKE_BUILD_TYPE "Release")
endif()

option(ENABLE_ADDRESS_SANITIZER "enable address sanitizer" False)

if(ENABLE_ADDRESS_SANITIZER)
        set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -fsanitize=address")
endif()

set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -fexceptions")
set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} ${CMAKE_C_FLAGS}")

set(CMAKE_EXPORT_COMPILE_COMMANDS On)

set(CONFIG_H_DIR ${CMAKE_BINARY_DIR})
set(CONFIG_H ${CONFIG_H_DIR}/config.h)

option(ENABLE_CLOUD "enable cloud" True)
option(ENABLE_ACLK "enable aclk" True)
option(ENABLE_ML "enable machine learning" True)
option(ENABLE_H2O "enable h2o" True)
option(ENABLE_DBENGINE "enable dbengine" True)

option(ENABLE_PLUGIN_DEBUGFS "enable debugfs.plugin" True)
option(ENABLE_PLUGIN_APPS "enable apps.plugin" True)
option(ENABLE_PLUGIN_FREEIPMI "enable freeipmi.plugin" True)
option(ENABLE_PLUGIN_NFACCT "enable nfacct.plugin" True)
option(ENABLE_PLUGIN_XENSTAT "enable xenstat.plugin" True)
option(ENABLE_PLUGIN_PERF "enable perf.plugin" True)
option(ENABLE_PLUGIN_SLABINFO "enable slabinfo.plugin" True)
option(ENABLE_PLUGIN_CUPS "enable cups.plugin" True)
option(ENABLE_PLUGIN_CGROUP_NETWORK "enable cgroup-network plugin" True)
option(ENABLE_PLUGIN_EBPF "enable ebpf.plugin" True)
option(ENABLE_PLUGIN_LOCAL_LISTENERS "enable local-listeners" True)
option(ENABLE_PLUGIN_SYSTEMD_JOURNAL "enable systemd-journal.plugin" True)
option(ENABLE_PLUGIN_LOGS_MANAGEMENT "enable logs-management.plugin" True)

option(ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE "enable prometheus remote write exporter" True)
option(ENABLE_EXPORTER_MONGODB "enable mongodb exporter" True)

option(ENABLE_BUNDLED_JSONC "enable bundled json-c" False)
option(ENABLE_BUNDLED_YAML "enable bundled yaml" False)
option(ENABLE_BUNDLED_PROTOBUF "enable bundled protobuf" False)

option(ENABLE_LOGS_MANAGEMENT_TESTS "enable logs management tests" True)

option(ENABLE_WEBRTC "enable webrtc" False)

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

#
# handling of extra compiler flags
#

include(CheckCCompilerFlag)
include(CheckCXXCompilerFlag)

# Disable hardening for debug builds by default.
if(CMAKE_BUILD_TYPE STREQUAL "Debug")
        option(DISABLE_HARDENING "disable adding extra compiler flags for hardening" TRUE)
else()
        option(DISABLE_HARDENING "disable adding extra compiler flags for hardening" FALSE)
endif()

# Construct a pre-processor safe name
function(make_cpp_safe_name value target)
        string(REPLACE "-" "_" tmp "${value}")
        string(REPLACE "=" "_" tmp "${tmp}")
        set(${target} "${tmp}" PARENT_SCOPE)
endfunction()

# Conditionally add an extra compiler flag to C and C++ flags.
#
# If the language flags already match the `match` argument, skip this flag.
# Otherwise, check for support for `flag` and if support is found, add it to
# the language-specific `target` flag group.
function(add_simple_extra_compiler_flag match flag target)
        set(CMAKE_REQUIRED_FLAGS "-Werror")

        make_cpp_safe_name("${flag}" flag_name)

        if(NOT ${CMAKE_C_FLAGS} MATCHES ${match})
                check_c_compiler_flag("${flag}" HAVE_C_${flag_name})
                if(HAVE_C_${flag_name})
                        set(${target}_C_FLAGS "${${target}_C_FLAGS} ${flag}" PARENT_SCOPE)
                endif()
        endif()

        if(NOT ${CMAKE_CXX_FLAGS} MATCHES ${match})
                check_cxx_compiler_flag("${flag}" HAVE_CXX_${flag_name})
                if(HAVE_CXX_${flag_name})
                        set(${target}_CXX_FLAGS "${${target}_CXX_FLAGS} ${flag}" PARENT_SCOPE)
                endif()
        endif()
endfunction()

# Same as add_simple_extra_compiler_flag, but check for a second flag if the
# first one is unsupported.
function(add_double_extra_compiler_flag match flag1 flag2 target)
        set(CMAKE_REQUIRED_FLAGS "-Werror")

        make_cpp_safe_name("${flag1}" flag1_name)
        make_cpp_safe_name("${flag2}" flag2_name)

        if(NOT ${CMAKE_C_FLAGS} MATCHES ${match})
                check_c_compiler_flag("${flag1}" HAVE_C_${flag1_name})
                if(HAVE_C_${flag1_name})
                        set(${target}_C_FLAGS "${${target}_C_FLAGS} ${flag1}" PARENT_SCOPE)
                else()
                        check_c_compiler_flag("${flag2}" HAVE_C_${flag2_name})
                        if(HAVE_C_${flag2_name})
                                set(${target}_C_FLAGS "${${target}_C_FLAGS} ${flag2}" PARENT_SCOPE)
                        endif()
                endif()
        endif()

        if(NOT ${CMAKE_CXX_FLAGS} MATCHES ${match})
                check_cxx_compiler_flag("${flag1}" HAVE_CXX_${flag1_name})
                if(HAVE_CXX_${flag1_name})
                        set(${target}_CXX_FLAGS "${${target}_CXX_FLAGS} ${flag1}" PARENT_SCOPE)
                else()
                        check_cxx_compiler_flag("${flag2}" HAVE_CXX_${flag2_name})
                        if(HAVE_CXX_${flag2_name})
                                set(${target}_CXX_FLAGS "${${target}_CXX_FLAGS} ${flag2}" PARENT_SCOPE)
                        endif()
                endif()
        endif()
endfunction()

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
                set(LINK_LIBM True CACHE BOOL "" FORCE)
        else()
                message(FATAL_ERROR "Can not use log10 with/without libm.")
        endif()
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
# check libraries we need
#

include(CheckLibraryExists)

check_library_exists(snappy snappy_compress "" HAVE_SNAPPY_LIB)
#check_include_file("snappy.h" HAVE_SNAPPY_H)

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
if(NOT MACOS)
        pkg_check_modules(CRYPTO libcrypto)
        pkg_check_modules(OPENSSL REQUIRED openssl)
else()
        # I think this is moot because we can only use openssl 3.x
        execute_process(COMMAND
                        brew --prefix --installed openssl
                        RESULT_VARIABLE BREW_OPENSSL
                        OUTPUT_VARIABLE BREW_OPENSSL_PREFIX
                        OUTPUT_STRIP_TRAILING_WHITESPACE)

        if((BREW_OPENSSL NOT EQUAL 0) OR (NOT EXISTS "${BREW_OPENSSL_PREFIX}"))
                execute_process(COMMAND
                                brew --prefix --installed openssl@3
                                RESULT_VARIABLE BREW_OPENSSL
                                OUTPUT_VARIABLE BREW_OPENSSL_PREFIX
                                OUTPUT_STRIP_TRAILING_WHITESPACE)

                if((BREW_OPENSSL NOT EQUAL 0) OR (NOT EXISTS "${BREW_OPENSSL_PREFIX}"))
                        execute_process(COMMAND
                                        brew --prefix --installed openssl@1.1
                                        RESULT_VARIABLE BREW_OPENSSL
                                        OUTPUT_VARIABLE BREW_OPENSSL_PREFIX
                                        OUTPUT_STRIP_TRAILING_WHITESPACE)

                        if((BREW_OPENSSL NOT EQUAL 0) OR (NOT EXISTS "${BREW_OPENSSL_PREFIX}"))
                                message(FATAL_ERROR "Could not find openssl prefix with brew")
                        endif()
                endif()
        endif()

        set(OPENSSL_INCLUDE_DIRS "${BREW_OPENSSL_PREFIX}/include")
        set(OPENSSL_CFLAGS_OTHER "")
        set(OPENSSL_LDFLAGS "-L${BREW_OPENSSL_PREFIX}/lib;-lssl;-lcrypto")
endif()

#
# figure out if we need protoc/protobuf
#

if(ENABLE_ACLK OR ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE)
        if(ENABLE_BUNDLED_PROTOBUF)
                set(PROTOBUF_PROTOC_EXECUTABLE "${CMAKE_SOURCE_DIR}/externaldeps/protobuf/src/protoc")
                set(PROTOBUF_INCLUDE_DIRS "${CMAKE_SOURCE_DIR}/externaldeps/protobuf/src")
                set(PROTOBUF_LIBRARIES "${CMAKE_SOURCE_DIR}/externaldeps/protobuf/src/.libs/libprotobuf.a")
        else()
                if (NOT BUILD_SHARED_LIBS)
                        set(Protobuf_USE_STATIC_LIBS On)
                endif()

                find_package(Protobuf)
        endif()

        set(ENABLE_PROTOBUF True)
        set(HAVE_PROTOBUF True)
        if (ENABLE_BUNDLED_PROTOBUF)
                set(BUNDLED_PROTOBUF True)
        endif()
endif()

#
# source files
#

set(LIBJUDY_PREV_FILES
        libnetdata/libjudy/src/JudyL/JudyLPrev.c
        libnetdata/libjudy/src/JudyL/JudyLPrevEmpty.c
)

set(LIBJUDY_NEXT_FILES
        libnetdata/libjudy/src/JudyL/JudyLNext.c
        libnetdata/libjudy/src/JudyL/JudyLNextEmpty.c
)

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
        libnetdata/libjudy/src/JudyL/JudyLTables.c
        libnetdata/libjudy/src/JudyHS/JudyHS.c
        ${LIBJUDY_PREV_FILES}
        ${LIBJUDY_NEXT_FILES}
)

set(LIBNETDATA_FILES
        ${CONFIG_H}
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
        libnetdata/datetime/iso8601.c
        libnetdata/datetime/iso8601.h
        libnetdata/datetime/rfc7231.c
        libnetdata/datetime/rfc7231.h
        libnetdata/datetime/rfc3339.c
        libnetdata/datetime/rfc3339.h
        libnetdata/dictionary/dictionary.c
        libnetdata/dictionary/dictionary.h
        libnetdata/eval/eval.c
        libnetdata/eval/eval.h
        libnetdata/facets/facets.c
        libnetdata/facets/facets.h
        libnetdata/functions_evloop/functions_evloop.c
        libnetdata/functions_evloop/functions_evloop.h
        libnetdata/gorilla/gorilla.cc
        libnetdata/gorilla/gorilla.h
        libnetdata/july/july.c
        libnetdata/july/july.h
        libnetdata/inlined.h
        libnetdata/json/json.c
        libnetdata/json/json.h
        libnetdata/json/jsmn.c
        libnetdata/json/jsmn.h
        libnetdata/libnetdata.c
        libnetdata/libnetdata.h
        libnetdata/line_splitter/line_splitter.c
        libnetdata/line_splitter/line_splitter.h
        libnetdata/libnetdata.h
        libnetdata/locks/locks.c
        libnetdata/locks/locks.h
        libnetdata/log/journal.c
        libnetdata/log/journal.h
        libnetdata/log/log.c
        libnetdata/log/log.h
        libnetdata/os.c
        libnetdata/os.h
        libnetdata/simple_hashtable.h
        libnetdata/endian.h
        libnetdata/onewayalloc/onewayalloc.c
        libnetdata/onewayalloc/onewayalloc.h
        libnetdata/popen/popen.c
        libnetdata/popen/popen.h
        libnetdata/procfile/procfile.c
        libnetdata/procfile/procfile.h
        libnetdata/query_progress/progress.c
        libnetdata/query_progress/progress.h
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
        libnetdata/uuid/uuid.c
        libnetdata/uuid/uuid.h
        libnetdata/string/utf8.h
        libnetdata/worker_utilization/worker_utilization.c
        libnetdata/worker_utilization/worker_utilization.h
        libnetdata/http/http_access.c
        libnetdata/http/http_access.h
        libnetdata/http/http_defs.c
        libnetdata/http/http_defs.h
        libnetdata/http/content_type.c
        libnetdata/http/content_type.h
        libnetdata/config/dyncfg.c
        libnetdata/config/dyncfg.h
        libnetdata/json/json-c-parser-inline.h
        libnetdata/template-enum.h
)

if(ENABLE_PLUGIN_EBPF)
    list(APPEND LIBNETDATA_FILES
            libnetdata/ebpf/ebpf.c
            libnetdata/ebpf/ebpf.h
    )
endif()

set(LIBH2O_FILES
        web/server/h2o/libh2o/deps/cloexec/cloexec.c
        web/server/h2o/libh2o/deps/libgkc/gkc.c
        web/server/h2o/libh2o/deps/libyrmcds/close.c
        web/server/h2o/libh2o/deps/libyrmcds/connect.c
        web/server/h2o/libh2o/deps/libyrmcds/recv.c
        web/server/h2o/libh2o/deps/libyrmcds/send.c
        web/server/h2o/libh2o/deps/libyrmcds/send_text.c
        web/server/h2o/libh2o/deps/libyrmcds/socket.c
        web/server/h2o/libh2o/deps/libyrmcds/strerror.c
        web/server/h2o/libh2o/deps/libyrmcds/text_mode.c
        web/server/h2o/libh2o/deps/picohttpparser/picohttpparser.c
        web/server/h2o/libh2o/lib/common/cache.c
        web/server/h2o/libh2o/lib/common/file.c
        web/server/h2o/libh2o/lib/common/filecache.c
        web/server/h2o/libh2o/lib/common/hostinfo.c
        web/server/h2o/libh2o/lib/common/http1client.c
        web/server/h2o/libh2o/lib/common/memcached.c
        web/server/h2o/libh2o/lib/common/memory.c
        web/server/h2o/libh2o/lib/common/multithread.c
        web/server/h2o/libh2o/lib/common/serverutil.c
        web/server/h2o/libh2o/lib/common/socket.c
        web/server/h2o/libh2o/lib/common/socketpool.c
        web/server/h2o/libh2o/lib/common/string.c
        web/server/h2o/libh2o/lib/common/time.c
        web/server/h2o/libh2o/lib/common/timeout.c
        web/server/h2o/libh2o/lib/common/url.c
        web/server/h2o/libh2o/lib/core/config.c
        web/server/h2o/libh2o/lib/core/configurator.c
        web/server/h2o/libh2o/lib/core/context.c
        web/server/h2o/libh2o/lib/core/headers.c
        web/server/h2o/libh2o/lib/core/logconf.c
        web/server/h2o/libh2o/lib/core/proxy.c
        web/server/h2o/libh2o/lib/core/request.c
        web/server/h2o/libh2o/lib/core/token.c
        web/server/h2o/libh2o/lib/core/util.c
        web/server/h2o/libh2o/lib/handler/access_log.c
        web/server/h2o/libh2o/lib/handler/chunked.c
        web/server/h2o/libh2o/lib/handler/compress.c
        web/server/h2o/libh2o/lib/handler/compress/gzip.c
        web/server/h2o/libh2o/lib/handler/errordoc.c
        web/server/h2o/libh2o/lib/handler/expires.c
        web/server/h2o/libh2o/lib/handler/fastcgi.c
        web/server/h2o/libh2o/lib/handler/file.c
        web/server/h2o/libh2o/lib/handler/headers.c
        web/server/h2o/libh2o/lib/handler/mimemap.c
        web/server/h2o/libh2o/lib/handler/proxy.c
        web/server/h2o/libh2o/lib/handler/redirect.c
        web/server/h2o/libh2o/lib/handler/reproxy.c
        web/server/h2o/libh2o/lib/handler/throttle_resp.c
        web/server/h2o/libh2o/lib/handler/status.c
        web/server/h2o/libh2o/lib/handler/headers_util.c
        web/server/h2o/libh2o/lib/handler/status/events.c
        web/server/h2o/libh2o/lib/handler/status/requests.c
        web/server/h2o/libh2o/lib/handler/http2_debug_state.c
        web/server/h2o/libh2o/lib/handler/status/durations.c
        web/server/h2o/libh2o/lib/handler/configurator/access_log.c
        web/server/h2o/libh2o/lib/handler/configurator/compress.c
        web/server/h2o/libh2o/lib/handler/configurator/errordoc.c
        web/server/h2o/libh2o/lib/handler/configurator/expires.c
        web/server/h2o/libh2o/lib/handler/configurator/fastcgi.c
        web/server/h2o/libh2o/lib/handler/configurator/file.c
        web/server/h2o/libh2o/lib/handler/configurator/headers.c
        web/server/h2o/libh2o/lib/handler/configurator/proxy.c
        web/server/h2o/libh2o/lib/handler/configurator/redirect.c
        web/server/h2o/libh2o/lib/handler/configurator/reproxy.c
        web/server/h2o/libh2o/lib/handler/configurator/throttle_resp.c
        web/server/h2o/libh2o/lib/handler/configurator/status.c
        web/server/h2o/libh2o/lib/handler/configurator/http2_debug_state.c
        web/server/h2o/libh2o/lib/handler/configurator/headers_util.c
        web/server/h2o/libh2o/lib/http1.c
        web/server/h2o/libh2o/lib/tunnel.c
        web/server/h2o/libh2o/lib/http2/cache_digests.c
        web/server/h2o/libh2o/lib/http2/casper.c
        web/server/h2o/libh2o/lib/http2/connection.c
        web/server/h2o/libh2o/lib/http2/frame.c
        web/server/h2o/libh2o/lib/http2/hpack.c
        web/server/h2o/libh2o/lib/http2/scheduler.c
        web/server/h2o/libh2o/lib/http2/stream.c
        web/server/h2o/libh2o/lib/http2/http2_debug_state.c
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
        daemon/pipename.c
        daemon/pipename.h
        daemon/unit_test.c
        daemon/unit_test.h
        daemon/config/dyncfg.c
        daemon/config/dyncfg.h
        daemon/config/dyncfg-files.c
        daemon/config/dyncfg-unittest.c
        daemon/config/dyncfg-inline.c
        daemon/config/dyncfg-echo.c
        daemon/config/dyncfg-internals.h
        daemon/config/dyncfg-intercept.c
        daemon/config/dyncfg-tree.c
)

set(H2O_FILES
        web/server/h2o/http_server.c
        web/server/h2o/http_server.h
        web/server/h2o/h2o_utils.c
        web/server/h2o/h2o_utils.h
        web/server/h2o/streaming.c
        web/server/h2o/streaming.h
        web/server/h2o/connlist.c
        web/server/h2o/connlist.h
)

if(ENABLE_H2O)
    list(APPEND DAEMON_FILES ${H2O_FILES})
endif()

set(API_PLUGIN_FILES
        web/api/web_api.c
        web/api/web_api.h
        web/api/web_api_v1.c
        web/api/web_api_v1.h
        web/api/web_api_v2.c
        web/api/web_api_v2.h
        web/api/http_auth.c
        web/api/http_auth.h
        web/api/http_header.c
        web/api/http_header.h
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
        web/api/ilove/ilove.c
        web/api/ilove/ilove.h
        web/rtc/webrtc.c
        web/rtc/webrtc.h
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

set(HEALTH_PLUGIN_FILES
        health/health.c
        health/health.h
        health/health_config.c
        health/health_json.c
        health/health_log.c
        health/health_prototypes.c
        health/health_prototypes.h
        health/health_silencers.c
        health/health_silencers.h
        health/health_internals.h
        health/health_notifications.c
        health/health_event_loop.c
        health/health_dyncfg.c
        health/health_variable.c
        health/rrdcalc.c
        health/rrdcalc.h
        health/rrdvar.c
        health/rrdvar.h
)

set(IDLEJITTER_PLUGIN_FILES collectors/idlejitter.plugin/plugin_idlejitter.c)

if(ENABLE_ML)
        set(ML_FILES
                ml/ad_charts.h
                ml/ad_charts.cc
                ml/Config.cc
                ml/dlib/dlib/all/source.cpp
                ml/ml.h
                ml/ml.cc
                ml/ml-private.h
        )
else()
        set(ML_FILES
                ml/ml.h
                ml/ml-dummy.c
        )
endif()

set(PLUGINSD_PLUGIN_FILES
        collectors/plugins.d/plugins_d.c
        collectors/plugins.d/plugins_d.h
        collectors/plugins.d/pluginsd_dyncfg.c
        collectors/plugins.d/pluginsd_dyncfg.h
        collectors/plugins.d/pluginsd_functions.c
        collectors/plugins.d/pluginsd_functions.h
        collectors/plugins.d/pluginsd_internals.c
        collectors/plugins.d/pluginsd_internals.h
        collectors/plugins.d/pluginsd_parser.c
        collectors/plugins.d/pluginsd_parser.h
        collectors/plugins.d/pluginsd_replication.c
        collectors/plugins.d/pluginsd_replication.h
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
        database/rrdcollector.c
        database/rrdcollector.h
        database/rrddim.c
        database/rrdfunctions.c
        database/rrdfunctions.h
        database/rrdfunctions-inline.c
        database/rrdfunctions-inline.h
        database/rrdfunctions-progress.c
        database/rrdfunctions-progress.h
        database/rrdfunctions-streaming.c
        database/rrdfunctions-streaming.h
        database/rrdhost.c
        database/rrdlabels.c
        database/rrd.c
        database/rrd.h
        database/rrdset.c
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
        database/sqlite/sqlite3recover.c
        database/sqlite/sqlite3recover.h
        database/sqlite/dbdata.c
        database/KolmogorovSmirnovDist.c
        database/KolmogorovSmirnovDist.h
        database/rrdfunctions-inflight.c
        database/rrdfunctions-inflight.h
        database/rrdfunctions-exporters.c
        database/rrdfunctions-exporters.h
        database/rrdfunctions-internals.h
        database/rrdcollector-internals.h
)

if(ENABLE_DBENGINE)
    list(APPEND RRD_PLUGIN_FILES
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
            database/engine/page_test.cc
            database/engine/page.c
            database/engine/page.h
            database/engine/cache.c
            database/engine/cache.h
            database/engine/metric.c
            database/engine/metric.h
            database/engine/pdc.c
            database/engine/pdc.h
    )
endif()

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
)

set(STATSD_PLUGIN_FILES
        collectors/statsd.plugin/statsd.c
)

set(SYSTEMD_JOURNAL_PLUGIN_FILES
        collectors/systemd-journal.plugin/systemd-journal.c
        collectors/systemd-journal.plugin/systemd-internals.h
        collectors/systemd-journal.plugin/systemd-main.c
        collectors/systemd-journal.plugin/systemd-units.c
        collectors/systemd-journal.plugin/systemd-journal.c
        collectors/systemd-journal.plugin/systemd-journal-annotations.c
        collectors/systemd-journal.plugin/systemd-journal-files.c
        collectors/systemd-journal.plugin/systemd-journal-fstat.c
        collectors/systemd-journal.plugin/systemd-journal-watcher.c
        collectors/systemd-journal.plugin/systemd-journal-dyncfg.c
)

set(STREAMING_PLUGIN_FILES
        streaming/rrdpush.c
        streaming/rrdpush.h
        streaming/compression.c
        streaming/compression.h
        streaming/compression_brotli.c
        streaming/compression_brotli.h
        streaming/compression_gzip.c
        streaming/compression_gzip.h
        streaming/compression_lz4.c
        streaming/compression_lz4.h
        streaming/compression_zstd.c
        streaming/compression_zstd.h
        streaming/receiver.c
        streaming/sender.c
        streaming/replication.c
        streaming/replication.h
        streaming/common.h
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

set(CLAIM_PLUGIN_FILES
        claim/claim.c
        claim/claim.h
)

set(SPAWN_PLUGIN_FILES
        spawn/spawn.c
        spawn/spawn_server.c
        spawn/spawn_client.c
        spawn/spawn.h
)

set(ACLK_ALWAYS_BUILD
        aclk/aclk_rrdhost_state.h
        aclk/aclk_proxy.c
        aclk/aclk_proxy.h
        aclk/aclk.c
        aclk/aclk.h
        aclk/aclk_capas.c
        aclk/aclk_capas.h
        aclk/aclk_util.c
        aclk/aclk_util.h
        aclk/https_client.c
        aclk/https_client.h
)

set(TIMEX_PLUGIN_FILES
        collectors/timex.plugin/plugin_timex.c
)

set(PROFILE_PLUGIN_FILES
        collectors/profile.plugin/plugin_profile.cc
)

set(CGROUPS_PLUGIN_FILES
        collectors/cgroups.plugin/sys_fs_cgroup.c
        collectors/cgroups.plugin/sys_fs_cgroup.h
        collectors/cgroups.plugin/cgroup-internals.h
        collectors/cgroups.plugin/cgroup-discovery.c
        collectors/cgroups.plugin/cgroup-charts.c
        collectors/cgroups.plugin/cgroup-top.c
)

set(DISKSPACE_PLUGIN_FILES
        collectors/diskspace.plugin/plugin_diskspace.c
)

set(MACOS_PLUGIN_FILES
        collectors/macos.plugin/plugin_macos.c
        collectors/macos.plugin/plugin_macos.h
        collectors/macos.plugin/macos_sysctl.c
        collectors/macos.plugin/macos_mach_smi.c
        collectors/macos.plugin/macos_fw.c
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

set(PROC_PLUGIN_FILES
        collectors/proc.plugin/ipc.c
        collectors/proc.plugin/plugin_proc.c
        collectors/proc.plugin/plugin_proc.h
        collectors/proc.plugin/proc_sys_fs_file_nr.c
        collectors/proc.plugin/proc_diskstats.c
        collectors/proc.plugin/proc_mdstat.c
        collectors/proc.plugin/proc_interrupts.c
        collectors/proc.plugin/proc_softirqs.c
        collectors/proc.plugin/proc_loadavg.c
        collectors/proc.plugin/proc_meminfo.c
        collectors/proc.plugin/proc_pagetypeinfo.c
        collectors/proc.plugin/proc_net_dev.c
        collectors/proc.plugin/proc_net_dev_renames.c
        collectors/proc.plugin/proc_net_dev_renames.h
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
        collectors/proc.plugin/sys_devices_pci_aer.c
        collectors/proc.plugin/sys_class_drm.c
)

set(TC_PLUGIN_FILES
        collectors/tc.plugin/plugin_tc.c
)

set(LOGS_MANAGEMENT_PLUGIN_FILES
        logsmanagement/circular_buffer.c
        logsmanagement/circular_buffer.h
        logsmanagement/db_api.c
        logsmanagement/db_api.h
        logsmanagement/defaults.h
        logsmanagement/file_info.h
        logsmanagement/flb_plugin.c
        logsmanagement/flb_plugin.h
        logsmanagement/functions.c
        logsmanagement/functions.h
        logsmanagement/helper.h
        logsmanagement/logsmanag_config.c
        logsmanagement/logsmanag_config.h
        logsmanagement/logsmanagement.c
        logsmanagement/parser.c
        logsmanagement/parser.h
        logsmanagement/query.c
        logsmanagement/query.h
        logsmanagement/rrd_api/rrd_api_docker_ev.c
        logsmanagement/rrd_api/rrd_api_docker_ev.h
        logsmanagement/rrd_api/rrd_api_generic.c
        logsmanagement/rrd_api/rrd_api_generic.h
        logsmanagement/rrd_api/rrd_api_kernel.c
        logsmanagement/rrd_api/rrd_api_kernel.h
        logsmanagement/rrd_api/rrd_api_mqtt.c
        logsmanagement/rrd_api/rrd_api_mqtt.h
        logsmanagement/rrd_api/rrd_api_stats.c
        logsmanagement/rrd_api/rrd_api_stats.h
        logsmanagement/rrd_api/rrd_api_systemd.c
        logsmanagement/rrd_api/rrd_api_systemd.h
        logsmanagement/rrd_api/rrd_api_web_log.c
        logsmanagement/rrd_api/rrd_api_web_log.h
        logsmanagement/rrd_api/rrd_api.h
        database/sqlite/sqlite3.c
        database/sqlite/sqlite3.h
)

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
        ${ACLK_ALWAYS_BUILD}
        ${TIMEX_PLUGIN_FILES}
        ${PROFILE_PLUGIN_FILES}
)

if(LINUX)
        list(APPEND NETDATA_FILES
                daemon/static_threads_linux.c
                ${CGROUPS_PLUGIN_FILES}
                ${DISKSPACE_PLUGIN_FILES}
                ${PROC_PLUGIN_FILES}
                ${TC_PLUGIN_FILES}
        )
elseif(MACOS)
        list(APPEND NETDATA_FILES
                daemon/static_threads_macos.c
                ${MACOS_PLUGIN_FILES}
        )
elseif(FREEBSD)
        list(APPEND NETDATA_FILES
                daemon/static_threads_freebsd.c
                ${FREEBSD_PLUGIN_FILES}
        )
endif()

set(MQTT_WEBSOCKETS_FILES
        aclk/mqtt_websockets/mqtt_wss_client.c
        aclk/mqtt_websockets/mqtt_wss_client.h
        aclk/mqtt_websockets/mqtt_wss_log.c
        aclk/mqtt_websockets/mqtt_wss_log.h
        aclk/mqtt_websockets/ws_client.c
        aclk/mqtt_websockets/ws_client.h
        aclk/mqtt_websockets/mqtt_ng.c
        aclk/mqtt_websockets/mqtt_ng.h
        aclk/mqtt_websockets/common_public.c
        aclk/mqtt_websockets/common_public.h
        aclk/mqtt_websockets/common_internal.h
        aclk/mqtt_websockets/c-rbuf/cringbuffer.c
        aclk/mqtt_websockets/c-rbuf/cringbuffer.h
        aclk/mqtt_websockets/c-rbuf/cringbuffer_internal.h
        aclk/mqtt_websockets/c_rhash/c_rhash.c
        aclk/mqtt_websockets/c_rhash/c_rhash.h
        aclk/mqtt_websockets/c_rhash/c_rhash_internal.h
)

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

set(ACLK_FILES
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


set(MONGODB_EXPORTING_FILES
        exporting/mongodb/mongodb.c
        exporting/mongodb/mongodb.h
)

set(PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES
        exporting/prometheus/remote_write/remote_write.c
        exporting/prometheus/remote_write/remote_write.h
        exporting/prometheus/remote_write/remote_write_request.cc
        exporting/prometheus/remote_write/remote_write_request.h
)

#
# build h2o
#

if(ENABLE_H2O)
        add_library(h2o STATIC ${LIBH2O_FILES})

        target_include_directories(h2o BEFORE PUBLIC
                "${CMAKE_SOURCE_DIR}/web/server/h2o/libh2o/include"
                "${CMAKE_SOURCE_DIR}/web/server/h2o/libh2o/deps/cloexec"
                "${CMAKE_SOURCE_DIR}/web/server/h2o/libh2o/deps/brotli/enc"
                "${CMAKE_SOURCE_DIR}/web/server/h2o/libh2o/deps/golombset"
                "${CMAKE_SOURCE_DIR}/web/server/h2o/libh2o/deps/libgkc"
                "${CMAKE_SOURCE_DIR}/web/server/h2o/libh2o/deps/libyrmcds"
                "${CMAKE_SOURCE_DIR}/web/server/h2o/libh2o/deps/klib"
                "${CMAKE_SOURCE_DIR}/web/server/h2o/libh2o/deps/neverbleed"
                "${CMAKE_SOURCE_DIR}/web/server/h2o/libh2o/deps/picohttpparser"
                "${CMAKE_SOURCE_DIR}/web/server/h2o/libh2o/deps/picotest"
                "${CMAKE_SOURCE_DIR}/web/server/h2o/libh2o/deps/yaml/include"
                "${CMAKE_SOURCE_DIR}/web/server/h2o/libh2o/deps/yoml"
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
        libnetdata/libjudy/src
        libnetdata/libjudy/src/JudyCommon
)

set_source_files_properties(${LIBJUDY_PREV_FILES} PROPERTIES COMPILE_OPTIONS "-DJUDYPREV")
set_source_files_properties(${LIBJUDY_NEXT_FILES} PROPERTIES COMPILE_OPTIONS "-DJUDYNEXT")
set_source_files_properties(libnetdata/libjudy/src/JudyL/j__udyLGet.c PROPERTIES COMPILE_OPTIONS "-DJUDYGETINLINE")
set_source_files_properties(libnetdata/libjudy/src/JudyL/JudyLByCount.c PROPERTIES COMPILE_OPTIONS "-DNOSMARTJBB -DNOSMARTJBU -DNOSMARTJLB")
set_source_files_properties(JudyLTables.c PROPERTIES COMPILE_OPTIONS "-I${CMAKE_SOURCE_DIR}/libnetdata/libjudy/src/JudyL")

#
# build libnetdata
#

include(systemd.cmake)

add_library(libnetdata STATIC ${LIBNETDATA_FILES})

target_include_directories(libnetdata BEFORE PUBLIC ${CONFIG_H_DIR} ${CMAKE_SOURCE_DIR})

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
        target_link_libraries(libnetdata PUBLIC ${CMAKE_SOURCE_DIR}/externaldeps/libbpf/libbpf.a)
        target_include_directories(libnetdata BEFORE PUBLIC
                ${CMAKE_SOURCE_DIR}/externaldeps/libbpf/include
                ${CMAKE_SOURCE_DIR}/externaldeps/libbpf/include/uapi
        )

        pkg_check_modules(ELF REQUIRED libelf)
        target_include_directories(libnetdata BEFORE PUBLIC ${ELF_INCLUDE_DIRS})
        target_compile_options(libnetdata PUBLIC ${ELF_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${ELF_LIBRARIES})
endif()

# judy
target_link_libraries(libnetdata PUBLIC judy)

# json-c
if(ENABLE_BUNDLED_JSONC)
        add_library(jsonc STATIC IMPORTED)
        set_property(TARGET jsonc PROPERTY
                     IMPORTED_LOCATION "${CMAKE_SOURCE_DIR}/externaldeps/jsonc/libjson-c.a")

        target_include_directories(libnetdata BEFORE PUBLIC "${CMAKE_SOURCE_DIR}/externaldeps/jsonc")
        target_link_libraries(libnetdata PUBLIC jsonc)
else()
        pkg_check_modules(JSONC REQUIRED json-c)
        target_include_directories(libnetdata BEFORE PUBLIC ${JSONC_INCLUDE_DIRS})
        target_compile_definitions(libnetdata PUBLIC ${JSONC_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${JSONC_LDFLAGS})
endif()

# message(FATAL_ERROR "jsonc libraries: ${JSONC_LIBRARIES}")
# message(FATAL_ERROR "jsonc ldflags: ${JSONC_LDFLAGS}")

# yaml
set(HAVE_LIBYAML  True)
if(ENABLE_BUNDLED_YAML)
        add_library(yaml STATIC IMPORTED)
        set_property(TARGET yaml PROPERTY
                     IMPORTED_LOCATION "${CMAKE_SOURCE_DIR}/externaldeps/libyaml/libyaml.a")

        target_include_directories(libnetdata BEFORE PUBLIC "${CMAKE_SOURCE_DIR}/externaldeps/libyaml")
        target_link_libraries(libnetdata PUBLIC yaml)
else()
        pkg_check_modules(YAML REQUIRED yaml-0.1)
        target_include_directories(libnetdata BEFORE PUBLIC ${YAML_INCLUDE_DIRS})
        target_compile_definitions(libnetdata PUBLIC ${YAML_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${YAML_LDFLAGS})
endif()

# zlib
pkg_check_modules(ZLIB REQUIRED zlib)
target_include_directories(libnetdata BEFORE PUBLIC ${ZLIB_INCLUDE_DIRS})
target_compile_definitions(libnetdata PUBLIC ${ZLIB_CFLAGS_OTHER})
target_link_libraries(libnetdata PUBLIC ${ZLIB_LDFLAGS})

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
        target_include_directories(libnetdata BEFORE PUBLIC ${LIBBROTLI_INCLUDE_DIRS})
        target_compile_definitions(libnetdata PUBLIC ${LIBBROTLI_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${LIBBROTLI_LDFLAGS})
endif()

# uuid
pkg_check_modules(UUID REQUIRED uuid)
target_include_directories(libnetdata BEFORE PUBLIC ${UUID_INCLUDE_DIRS})
target_compile_definitions(libnetdata PUBLIC ${UUID_CFLAGS_OTHER})
target_link_libraries(libnetdata PUBLIC ${UUID_LDFLAGS})

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

# h2o
target_link_libraries(libnetdata PUBLIC "$<$<BOOL:${ENABLE_H2O}>:h2o>")

#
# helper function to build protos
#

function(protoc_generate_cpp INC_DIR OUT_DIR SRCS HDRS)
        if(NOT ARGN)
                message(SEND_ERROR "Error: protoc_generate_cpp() called without any proto files")
                return()
        endif()

        set(${INC_DIR})
        set(${OUT_DIR})
        set(${SRCS})
        set(${HDRS})

        foreach(FIL ${ARGN})
                get_filename_component(ABS_FIL ${FIL} ABSOLUTE)
                get_filename_component(DIR ${ABS_FIL} DIRECTORY)
                get_filename_component(FIL_WE ${FIL} NAME_WE)

                set(GENERATED_PB_CC "${DIR}/${FIL_WE}.pb.cc")
                list(APPEND ${SRCS} ${GENERATED_PB_CC})

                set(GENERATED_PB_H "${DIR}/${FIL_WE}.pb.h")
                list(APPEND ${HDRS} ${GENERATED_PB_H})

                add_custom_command(OUTPUT ${GENERATED_PB_CC} ${GENERATED_PB_H}
                                   COMMAND ${PROTOBUF_PROTOC_EXECUTABLE} ARGS -I=${INC_DIR} --cpp_out=${OUT_DIR} ${ABS_FIL}
                                   DEPENDS ${ABS_FIL} ${PROTOBUF_PROTOC_EXECUTABLE}
                                   COMMENT "Running C++ protocol buffer compiler on ${FIL}"
                                   VERBATIM)
        endforeach()

        set_source_files_properties(${${SRCS}} ${${HDRS}} PROPERTIES GENERATED TRUE)
        set_source_files_properties(${${SRCS}} ${${HDRS}} PROPERTIES COMPILE_OPTIONS -Wno-deprecated-declarations)

        set(${SRCS} ${${SRCS}} PARENT_SCOPE)
        set(${HDRS} ${${HDRS}} PARENT_SCOPE)
endfunction()

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
                                                         ${CMAKE_SOURCE_DIR}/web/server/h2o/libh2o/include)

        target_link_libraries(mqttwebsockets PRIVATE libnetdata)

endif()

if(ENABLE_ACLK)
        #
        # proto definitions
        #
        protoc_generate_cpp("${CMAKE_SOURCE_DIR}/aclk/aclk-schemas"
                            "${CMAKE_SOURCE_DIR}/aclk/aclk-schemas"
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

    set(DEBUGFS_PLUGIN_FILES collectors/debugfs.plugin/debugfs_plugin.c
                             collectors/debugfs.plugin/debugfs_plugin.h
                             collectors/debugfs.plugin/debugfs_extfrag.c
                             collectors/debugfs.plugin/debugfs_zswap.c
                             collectors/debugfs.plugin/sys_devices_virtual_powercap.c)

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

    set(APPS_PLUGIN_FILES collectors/apps.plugin/apps_plugin.c)

    add_executable(apps.plugin ${APPS_PLUGIN_FILES})
    target_link_libraries(apps.plugin libnetdata ${CAP_LIBRARIES})
    target_include_directories(apps.plugin PRIVATE ${CAP_INCLUDE_DIRS})
    target_compile_options(apps.plugin PRIVATE ${CAP_CFLAGS_OTHER})

    install(TARGETS apps.plugin
            COMPONENT apps_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
    install(FILES collectors/apps.plugin/apps_groups.conf
            COMPONENT apps_plugin
            DESTINATION usr/lib/netdata/conf.d)
endif()

if(CAP_FOUND)
    set(HAVE_CAPABILITY True)
endif()

if(ENABLE_PLUGIN_FREEIPMI)
    pkg_check_modules(IPMI REQUIRED libipmimonitoring)

    set(FREEIPMI_PLUGIN_FILES collectors/freeipmi.plugin/freeipmi_plugin.c)

    add_executable(freeipmi.plugin ${FREEIPMI_PLUGIN_FILES})
    target_link_libraries (freeipmi.plugin libnetdata ${IPMI_LIBRARIES})
    target_include_directories(freeipmi.plugin PRIVATE ${IPMI_INCLUDE_DIRS})
    target_compile_options(freeipmi.plugin PRIVATE ${IPMI_CFLAGS_OTHER})

    install(TARGETS freeipmi.plugin
            COMPONENT freeipmi_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_NFACCT)
    pkg_check_modules(NFACCT REQUIRED libnetfilter_acct)
    pkg_check_modules(MNL REQUIRED libmnl)

    set(NFACCT_PLUGIN_FILES collectors/nfacct.plugin/plugin_nfacct.c)

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

    set(XENSTAT_PLUGIN_FILES collectors/xenstat.plugin/xenstat_plugin.c)

    add_executable(xenstat.plugin ${XENSTAT_PLUGIN_FILES})
    target_link_libraries (xenstat.plugin libnetdata ${XENLIGHT_LIBRARIES} ${XENSTAT_LIBRARIES})
    target_include_directories(xenstat.plugin PRIVATE ${XENLIGHT_INCLUDE_DIRS} ${XENSTAT_INCLUDE_DIRS})
    target_compile_options(xenstat.plugin PRIVATE ${XENLIGHT_CFLAGS_OTHER} ${XENSTAT_CFLAGS_OTHER})

    install(TARGETS xenstat.plugin
            COMPONENT xenstat_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_PERF)
    set(PERF_PLUGIN_FILES collectors/perf.plugin/perf_plugin.c)

    add_executable(perf.plugin ${PERF_PLUGIN_FILES})
    target_link_libraries(perf.plugin libnetdata)

    install(TARGETS perf.plugin
            COMPONENT perf_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_SLABINFO)
    set(SLABINFO_PLUGIN_FILES collectors/slabinfo.plugin/slabinfo.c)

    add_executable(slabinfo.plugin ${SLABINFO_PLUGIN_FILES})
    target_link_libraries(slabinfo.plugin libnetdata)

    install(TARGETS slabinfo.plugin
            COMPONENT slabinfo_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_LOGS_MANAGEMENT)
    set(ENABLE_LOGSMANAGEMENT True)

    if(ENABLE_LOGS_MANAGEMENT_TESTS)
        list(APPEND LOGS_MANAGEMENT_PLUGIN_FILES logsmanagement/unit_test/unit_test.c
                                                 logsmanagement/unit_test/unit_test.h)
        set(ENABLE_LOGSMANAGEMENT_TESTS True)
    endif()

    add_executable(logs-management.plugin ${LOGS_MANAGEMENT_PLUGIN_FILES})
    target_link_libraries(logs-management.plugin libnetdata)

    install(TARGETS logs-management.plugin
            COMPONENT logs_management_plugin
            DESTINATION usr/libexec/netdata/plugins.d)

    install(DIRECTORY logsmanagement/stock_conf/logsmanagement.d
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
        set(CUPS_PLUGIN_FILES collectors/cups.plugin/cups_plugin.c)
        add_executable(cups.plugin ${CUPS_PLUGIN_FILES})
        target_link_libraries (cups.plugin libnetdata ${CUPS_LIBRARIES})
        target_compile_options(cups.plugin PRIVATE ${CUPS_CFLAGS_OTHER})

        install(TARGETS cups.plugin
                COMPONENT cups_plugin
                DESTINATION usr/libexec/netdata/plugins.d)
    endif()
endif()

set(NDSUDO_FILES collectors/plugins.d/ndsudo.c)

add_executable(ndsudo ${NDSUDO_FILES})

install(TARGETS ndsudo
        COMPONENT ndsudo
        DESTINATION usr/libexec/netdata/plugins.d)

if(ENABLE_PLUGIN_CGROUP_NETWORK)
    set(CGROUP_NETWORK_FILES collectors/cgroups.plugin/cgroup-network.c)

    add_executable(cgroup-network ${CGROUP_NETWORK_FILES})
    target_link_libraries(cgroup-network libnetdata)

    install(TARGETS cgroup-network
            COMPONENT cgroup_network_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_SYSTEMD_JOURNAL)
        add_executable(systemd-journal.plugin ${SYSTEMD_JOURNAL_PLUGIN_FILES})
        target_link_libraries(systemd-journal.plugin libnetdata)

        install(TARGETS systemd-journal.plugin
                COMPONENT systemd_journal_plugin
                DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_EBPF)
    set(EBPF_PLUGIN_FILES
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
            collectors/ebpf.plugin/ebpf_functions.c
            collectors/ebpf.plugin/ebpf_functions.h
    )

    add_executable(ebpf.plugin ${EBPF_PLUGIN_FILES})
    target_link_libraries(ebpf.plugin libnetdata)

    install(TARGETS ebpf.plugin
            COMPONENT ebpf_plugin
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_LOCAL_LISTENERS)
        set(LOCAL_LISTENERS_FILES collectors/plugins.d/local_listeners.c
                collectors/plugins.d/local-sockets.h)

        add_executable(local-listeners ${LOCAL_LISTENERS_FILES})
        target_link_libraries(local-listeners libnetdata)

        install(TARGETS local-listeners
                COMPONENT local_listeners
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
                if(HAVE_SNAPPY_LIB)
                        set(SNAPPY_INCLUDE_DIRS "")
                        set(SNAPPY_CFLAGS_OTHER "")
                        set(SNAPPY_LIBRARIES "-lsnappy")
                else()
                        message(FATAL_ERROR "Could not find snappy libraries with pkg-config or internal cmake checks.")
                endif()
        endif()

        protoc_generate_cpp("${CMAKE_SOURCE_DIR}/exporting/prometheus/remote_write"
                            "${CMAKE_SOURCE_DIR}/exporting/prometheus/remote_write"
                            PROMETHEUS_REMOTE_WRITE_BUILT_SRCS
                            PROMETHEUS_REMOTE_WRITE_BUILT_HDRS
                            "exporting/prometheus/remote_write/remote_write.proto")

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
        "$<$<BOOL:${ENABLE_PROTOBUF}>:${PROTOBUF_CFLAGS_OTHER}>"
        "$<$<BOOL:${ENABLE_ML}>:DLIB_NO_GUI_SUPPORT>"
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGOC_CFLAGS_OTHER}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${SNAPPY_CFLAGS_OTHER}>"
)

target_include_directories(netdata PRIVATE
        "$<$<BOOL:${ENABLE_PROTOBUF}>:${PROTOBUF_INCLUDE_DIRS}>"
        "$<$<BOOL:${ENABLE_ACLK}>:${CMAKE_SOURCE_DIR}/aclk/aclk-schemas>"
        "$<$<BOOL:${ENABLE_ML}>:${CMAKE_SOURCE_DIR}/ml/dlib>"
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGOC_INCLUDE_DIRS}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${SNAPPY_INCLUDE_DIRS}>"
)

target_link_libraries(netdata PRIVATE
        m
        libnetdata
        "$<$<BOOL:${LINUX}>:rt>"
        "$<$<BOOL:${ENABLE_MQTTWEBSOCKETS}>:mqttwebsockets>"
        "$<$<BOOL:${ENABLE_PROTOBUF}>:${PROTOBUF_LIBRARIES}>"
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGOC_LIBRARIES}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${SNAPPY_LIBRARIES}>"
        "$<$<BOOL:${MACOS}>:${IOKIT};${FOUNDATION}>"
        "$<$<BOOL:${ENABLE_WEBRTC}>:LibDataChannel::LibDataChannelStatic>"
)

#
# build systemd-cat-native
#
set(SYSTEMD_CAT_NATIVE_FILES libnetdata/log/systemd-cat-native.c
                             libnetdata/log/systemd-cat-native.h)

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
                collectors/log2journal/log2journal.h
                collectors/log2journal/log2journal.c
                collectors/log2journal/log2journal-help.c
                collectors/log2journal/log2journal-yaml.c
                collectors/log2journal/log2journal-json.c
                collectors/log2journal/log2journal-logfmt.c
                collectors/log2journal/log2journal-pcre2.c
                collectors/log2journal/log2journal-params.c
                collectors/log2journal/log2journal-inject.c
                collectors/log2journal/log2journal-pattern.c
                collectors/log2journal/log2journal-replace.c
                collectors/log2journal/log2journal-rename.c
                collectors/log2journal/log2journal-rewrite.c
        )

        add_executable(log2journal ${LOG2JOURNAL_FILES})
        target_include_directories(log2journal BEFORE PUBLIC ${CONFIG_H_DIR} ${PCRE2_INCLUDE_DIRS})
        target_compile_definitions(log2journal PUBLIC ${PCRE2_CFLAGS_OTHER})
        target_link_libraries(log2journal PUBLIC "${PCRE2_LDFLAGS}")

        if(ENABLE_BUNDLED_YAML)
                target_include_directories(log2journal BEFORE PUBLIC "${CMAKE_SOURCE_DIR}/externaldeps/libyaml")
                target_link_libraries(log2journal PUBLIC yaml)
        else()
                target_include_directories(log2journal BEFORE PUBLIC ${YAML_INCLUDE_DIRS})
                target_compile_definitions(log2journal PUBLIC ${YAML_CFLAGS_OTHER})
                target_link_libraries(log2journal PUBLIC ${YAML_LDFLAGS})
        endif()

        install(TARGETS log2journal
                COMPONENT log2journal
                DESTINATION usr/sbin)

        install(DIRECTORY collectors/log2journal/log2journal.d
                COMPONENT log2journal
                DESTINATION usr/lib/netdata/conf.d)
endif()

#
# build netdatacli
#

set(NETDATACLI_FILES
        daemon/commands.h
        daemon/pipename.c
        daemon/pipename.h
        cli/cli.c
)

add_executable(netdatacli ${NETDATACLI_FILES})
target_link_libraries(netdatacli libnetdata)

install(TARGETS netdatacli
        COMPONENT netdatacli
        DESTINATION usr/sbin)

#
# Generate config file
#

add_definitions(-DHAVE_CONFIG_H)

set(STORAGE_WITH_MATH On)

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

configure_file(config.cmake.h.in config.h)

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

configure_file(claim/netdata-claim.sh.in claim/netdata-claim.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/claim/netdata-claim.sh
        DESTINATION usr/sbin)

#
# We don't check ENABLE_PLUGIN_CGROUP_NETWORK because rpm builds assume
# the files exists unconditionally.
#
configure_file(collectors/cgroups.plugin/cgroup-network-helper.sh.in
               collectors/cgroups.plugin/cgroup-network-helper.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/collectors/cgroups.plugin/cgroup-network-helper.sh
        COMPONENT cgroup_network_plugin
        DESTINATION usr/libexec/netdata/plugins.d)

configure_file(collectors/cgroups.plugin/cgroup-name.sh.in
               collectors/cgroups.plugin/cgroup-name.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/collectors/cgroups.plugin/cgroup-name.sh
        DESTINATION usr/libexec/netdata/plugins.d)

#
# statsd
#
install(FILES
        collectors/statsd.plugin/asterisk.conf
        collectors/statsd.plugin/example.conf
        collectors/statsd.plugin/k6.conf
        DESTINATION usr/lib/netdata/conf.d/statsd.d)

#
# exporting
#
install(FILES
        exporting/exporting.conf
        DESTINATION usr/lib/netdata/conf.d)

#
# ioping.plugin
#
install(FILES
        collectors/ioping.plugin/ioping.conf
        DESTINATION usr/lib/netdata/conf.d)

#
# streaming
#
install(FILES
        streaming/stream.conf
        DESTINATION usr/lib/netdata/conf.d)

#
# swagger
#
install(FILES
        web/api/netdata-swagger.json
        web/api/netdata-swagger.yaml
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
configure_file(daemon/anonymous-statistics.sh.in daemon/anonymous-statistics.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/daemon/anonymous-statistics.sh
        DESTINATION usr/libexec/netdata/plugins.d)

configure_file(daemon/get-kubernetes-labels.sh.in daemon/get-kubernetes-labels.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/daemon/get-kubernetes-labels.sh
        DESTINATION usr/libexec/netdata/plugins.d)

install(PROGRAMS
        daemon/system-info.sh
        DESTINATION usr/libexec/netdata/plugins.d)

#
# health files
#

file(GLOB_RECURSE HEALTH_CONF_FILES "health/health.d/*.conf")
install(FILES
        ${HEALTH_CONF_FILES}
        DESTINATION usr/lib/netdata/conf.d/health.d)

configure_file(health/notifications/alarm-notify.sh.in health/notifications/alarm-notify.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/health/notifications/alarm-notify.sh
        DESTINATION usr/libexec/netdata/plugins.d)

install(PROGRAMS
        health/notifications/alarm-email.sh
        health/notifications/alarm-test.sh
        DESTINATION usr/libexec/netdata/plugins.d)

install(FILES
        health/notifications/health_alarm_notify.conf
        health/notifications/health_email_recipients.conf
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

configure_file(collectors/charts.d.plugin/charts.d.plugin.in collectors/charts.d.plugin/charts.d.plugin @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/collectors/charts.d.plugin/charts.d.plugin
        DESTINATION usr/libexec/netdata/plugins.d)

install(FILES
        collectors/charts.d.plugin/charts.d.dryrun-helper.sh
        collectors/charts.d.plugin/loopsleepms.sh.inc
        DESTINATION usr/libexec/netdata/plugins.d)

install(FILES
        collectors/charts.d.plugin/charts.d.conf
        DESTINATION usr/lib/netdata/conf.d)

#
# tc-qos-helper
#

configure_file(collectors/tc.plugin/tc-qos-helper.sh.in collectors/tc.plugin/tc-qos-helper.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/collectors/tc.plugin/tc-qos-helper.sh
        DESTINATION usr/libexec/netdata/plugins.d)

# scripts

install(FILES
        collectors/charts.d.plugin/ap/ap.chart.sh
        collectors/charts.d.plugin/apcupsd/apcupsd.chart.sh
        collectors/charts.d.plugin/example/example.chart.sh
        collectors/charts.d.plugin/libreswan/libreswan.chart.sh
        collectors/charts.d.plugin/opensips/opensips.chart.sh
        collectors/charts.d.plugin/sensors/sensors.chart.sh
        DESTINATION usr/libexec/netdata/charts.d)

# confs

install(FILES
        collectors/charts.d.plugin/ap/ap.conf
        collectors/charts.d.plugin/apcupsd/apcupsd.conf
        collectors/charts.d.plugin/example/example.conf
        collectors/charts.d.plugin/libreswan/libreswan.conf
        collectors/charts.d.plugin/opensips/opensips.conf
        collectors/charts.d.plugin/sensors/sensors.conf
        DESTINATION usr/lib/netdata/conf.d/charts.d)

install(FILES
        collectors/systemd-journal.plugin/schema.d/systemd-journal:monitored-directories.json
        health/schema.d/health:alert:prototype.json
        DESTINATION usr/lib/netdata/conf.d/schema.d)

#
# ebpf files
#

if(ENABLE_PLUGIN_EBPF)
        install(FILES
                collectors/ebpf.plugin/ebpf.d.conf
                DESTINATION usr/lib/netdata/conf.d)

        install(FILES
                collectors/ebpf.plugin/ebpf.d/cachestat.conf
                collectors/ebpf.plugin/ebpf.d/dcstat.conf
                collectors/ebpf.plugin/ebpf.d/disk.conf
                collectors/ebpf.plugin/ebpf.d/ebpf_kernel_reject_list.txt
                collectors/ebpf.plugin/ebpf.d/fd.conf
                collectors/ebpf.plugin/ebpf.d/filesystem.conf
                collectors/ebpf.plugin/ebpf.d/hardirq.conf
                collectors/ebpf.plugin/ebpf.d/mdflush.conf
                collectors/ebpf.plugin/ebpf.d/mount.conf
                collectors/ebpf.plugin/ebpf.d/network.conf
                collectors/ebpf.plugin/ebpf.d/oomkill.conf
                collectors/ebpf.plugin/ebpf.d/process.conf
                collectors/ebpf.plugin/ebpf.d/shm.conf
                collectors/ebpf.plugin/ebpf.d/softirq.conf
                collectors/ebpf.plugin/ebpf.d/swap.conf
                collectors/ebpf.plugin/ebpf.d/sync.conf
                collectors/ebpf.plugin/ebpf.d/vfs.conf
                DESTINATION usr/lib/netdata/conf.d/ebpf.d)
endif()

#
# python.d files
#

configure_file(collectors/python.d.plugin/python.d.plugin.in collectors/python.d.plugin/python.d.plugin @ONLY)
install(PROGRAMS ${CMAKE_BINARY_DIR}/collectors/python.d.plugin/python.d.plugin
        DESTINATION usr/libexec/netdata/plugins.d)

install(DIRECTORY collectors/python.d.plugin/python_modules
        DESTINATION usr/libexec/netdata/python.d)

install(FILES collectors/python.d.plugin/python.d.conf
        DESTINATION usr/lib/netdata/conf.d)

# conf files

install(FILES
        collectors/python.d.plugin/adaptec_raid/adaptec_raid.conf
        collectors/python.d.plugin/alarms/alarms.conf
        collectors/python.d.plugin/am2320/am2320.conf
        collectors/python.d.plugin/anomalies/anomalies.conf
        collectors/python.d.plugin/beanstalk/beanstalk.conf
        collectors/python.d.plugin/bind_rndc/bind_rndc.conf
        collectors/python.d.plugin/boinc/boinc.conf
        collectors/python.d.plugin/ceph/ceph.conf
        collectors/python.d.plugin/changefinder/changefinder.conf
        collectors/python.d.plugin/dovecot/dovecot.conf
        collectors/python.d.plugin/example/example.conf
        collectors/python.d.plugin/exim/exim.conf
        collectors/python.d.plugin/fail2ban/fail2ban.conf
        collectors/python.d.plugin/gearman/gearman.conf
        collectors/python.d.plugin/go_expvar/go_expvar.conf
        collectors/python.d.plugin/haproxy/haproxy.conf
        collectors/python.d.plugin/hddtemp/hddtemp.conf
        collectors/python.d.plugin/hpssa/hpssa.conf
        collectors/python.d.plugin/icecast/icecast.conf
        collectors/python.d.plugin/ipfs/ipfs.conf
        collectors/python.d.plugin/litespeed/litespeed.conf
        collectors/python.d.plugin/megacli/megacli.conf
        collectors/python.d.plugin/memcached/memcached.conf
        collectors/python.d.plugin/monit/monit.conf
        collectors/python.d.plugin/nsd/nsd.conf
        collectors/python.d.plugin/nvidia_smi/nvidia_smi.conf
        collectors/python.d.plugin/openldap/openldap.conf
        collectors/python.d.plugin/oracledb/oracledb.conf
        collectors/python.d.plugin/pandas/pandas.conf
        collectors/python.d.plugin/postfix/postfix.conf
        collectors/python.d.plugin/puppet/puppet.conf
        collectors/python.d.plugin/rethinkdbs/rethinkdbs.conf
        collectors/python.d.plugin/retroshare/retroshare.conf
        collectors/python.d.plugin/riakkv/riakkv.conf
        collectors/python.d.plugin/samba/samba.conf
        collectors/python.d.plugin/sensors/sensors.conf
        collectors/python.d.plugin/smartd_log/smartd_log.conf
        collectors/python.d.plugin/spigotmc/spigotmc.conf
        collectors/python.d.plugin/squid/squid.conf
        collectors/python.d.plugin/tomcat/tomcat.conf
        collectors/python.d.plugin/tor/tor.conf
        collectors/python.d.plugin/traefik/traefik.conf
        collectors/python.d.plugin/uwsgi/uwsgi.conf
        collectors/python.d.plugin/varnish/varnish.conf
        collectors/python.d.plugin/w1sensor/w1sensor.conf
        collectors/python.d.plugin/zscores/zscores.conf
        DESTINATION usr/lib/netdata/conf.d/python.d)

# scripts

install(FILES
        collectors/python.d.plugin/adaptec_raid/adaptec_raid.chart.py
        collectors/python.d.plugin/alarms/alarms.chart.py
        collectors/python.d.plugin/am2320/am2320.chart.py
        collectors/python.d.plugin/anomalies/anomalies.chart.py
        collectors/python.d.plugin/beanstalk/beanstalk.chart.py
        collectors/python.d.plugin/bind_rndc/bind_rndc.chart.py
        collectors/python.d.plugin/boinc/boinc.chart.py
        collectors/python.d.plugin/ceph/ceph.chart.py
        collectors/python.d.plugin/changefinder/changefinder.chart.py
        collectors/python.d.plugin/dovecot/dovecot.chart.py
        collectors/python.d.plugin/example/example.chart.py
        collectors/python.d.plugin/exim/exim.chart.py
        collectors/python.d.plugin/fail2ban/fail2ban.chart.py
        collectors/python.d.plugin/gearman/gearman.chart.py
        collectors/python.d.plugin/go_expvar/go_expvar.chart.py
        collectors/python.d.plugin/haproxy/haproxy.chart.py
        collectors/python.d.plugin/hddtemp/hddtemp.chart.py
        collectors/python.d.plugin/hpssa/hpssa.chart.py
        collectors/python.d.plugin/icecast/icecast.chart.py
        collectors/python.d.plugin/ipfs/ipfs.chart.py
        collectors/python.d.plugin/litespeed/litespeed.chart.py
        collectors/python.d.plugin/megacli/megacli.chart.py
        collectors/python.d.plugin/memcached/memcached.chart.py
        collectors/python.d.plugin/monit/monit.chart.py
        collectors/python.d.plugin/nsd/nsd.chart.py
        collectors/python.d.plugin/nvidia_smi/nvidia_smi.chart.py
        collectors/python.d.plugin/openldap/openldap.chart.py
        collectors/python.d.plugin/oracledb/oracledb.chart.py
        collectors/python.d.plugin/pandas/pandas.chart.py
        collectors/python.d.plugin/postfix/postfix.chart.py
        collectors/python.d.plugin/puppet/puppet.chart.py
        collectors/python.d.plugin/rethinkdbs/rethinkdbs.chart.py
        collectors/python.d.plugin/retroshare/retroshare.chart.py
        collectors/python.d.plugin/riakkv/riakkv.chart.py
        collectors/python.d.plugin/samba/samba.chart.py
        collectors/python.d.plugin/sensors/sensors.chart.py
        collectors/python.d.plugin/smartd_log/smartd_log.chart.py
        collectors/python.d.plugin/spigotmc/spigotmc.chart.py
        collectors/python.d.plugin/squid/squid.chart.py
        collectors/python.d.plugin/tomcat/tomcat.chart.py
        collectors/python.d.plugin/tor/tor.chart.py
        collectors/python.d.plugin/traefik/traefik.chart.py
        collectors/python.d.plugin/uwsgi/uwsgi.chart.py
        collectors/python.d.plugin/varnish/varnish.chart.py
        collectors/python.d.plugin/w1sensor/w1sensor.chart.py
        collectors/python.d.plugin/zscores/zscores.chart.py
        DESTINATION usr/libexec/netdata/python.d)

# FIXME: don't install this unconditionally
configure_file(collectors/ioping.plugin/ioping.plugin.in collectors/ioping.plugin/ioping.plugin @ONLY)
install(PROGRAMS ${CMAKE_BINARY_DIR}/collectors/ioping.plugin/ioping.plugin DESTINATION usr/libexec/netdata/plugins.d)

#
# logs management
#
if (ENABLE_PLUGIN_LOGS_MANAGEMENT)
        configure_file(logsmanagement/stock_conf/logsmanagement.d.conf.in logsmanagement/stock_conf/logsmanagement.d.conf @ONLY)
        install(FILES ${CMAKE_BINARY_DIR}/logsmanagement/stock_conf/logsmanagement.d.conf
                COMPONENT logs_management_plugin
                DESTINATION usr/lib/netdata/conf.d)
endif()

#
# dashboard
#

include(web/gui/v1/dashboard_v1.cmake)
include(web/gui/v2/dashboard_v2.cmake)

include(web/gui/gui.cmake)

function(cat IN_FILE OUT_FILE)
        file(READ ${IN_FILE} CONTENTS)
        file(APPEND ${OUT_FILE} "${CONTENTS}")
endfunction()

file(WRITE ${CMAKE_BINARY_DIR}/web/gui/dashboard.js.in "")
foreach(JS_FILE ${DASHBOARD_JS_FILES})
  cat(${JS_FILE} ${CMAKE_BINARY_DIR}/dashboard.js.in)
endforeach()
configure_file(${CMAKE_BINARY_DIR}/dashboard.js.in
               ${CMAKE_BINARY_DIR}/dashboard.js COPYONLY)

install(FILES
        ${CMAKE_BINARY_DIR}/dashboard.js
        DESTINATION ${WEB_DEST})

install(FILES
        web/gui/dashboard_info_custom_example.js
        web/gui/dashboard_info.js
        web/gui/index.html
        web/gui/main.css
        web/gui/main.js
        web/gui/registry-access.html
        web/gui/registry-alert-redirect.html
        web/gui/registry-hello.html
        web/gui/switch.html
        web/gui/ilove.html
        DESTINATION ${WEB_DEST})

install(FILES
        web/gui/old/index.html
        DESTINATION ${WEB_DEST}/old)

install(FILES
        web/gui/static/img/netdata-logomark.svg
        DESTINATION ${WEB_DEST}/static/img)

install(FILES
        web/gui/css/morris-0.5.1.css
        web/gui/css/c3-0.4.18.min.css
        DESTINATION ${WEB_DEST}/css)

install(FILES
        web/gui/.well-known/dnt/cookies
        DESTINATION ${WEB_DEST}/.well-known/dnt)

# v0 dashboard
install(FILES
        web/gui/v0/index.html
        DESTINATION ${WEB_DEST}/v0)
