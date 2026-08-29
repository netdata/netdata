# SPDX-License-Identifier: GPL-3.0-or-later
# Platform capability probes: header, symbol, function and source-compile checks.
#
# Originally relocated from the root CMakeLists.txt, and since then reduced to the
# probes alone - the netdata_bundle_* calls and the stack-trace flag block it used
# to carry are back in the root file, where their order relative to each other is
# explained. include()d rather than add_subdirectory()d so
# CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR keep pointing at the
# repository and build roots. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Self-contained: every check_* command used below is made available by an include() in this file. It relies on nothing the including file happens to have done, which is what lets it be included this early.
#
# Nothing here creates a target or touches a directory property, so its position is free. It reads only CMAKE_SYSTEM_NAME (via the OS_* variables) and CMAKE_C_COMPILER_ID. The one thing it must stay ahead of is any consumer of the HAVE_* results, the earliest of which is the libnetdata link line.
#

include_guard()

#
# librt
#

include(CheckLibraryExists)

# clock_gettime / shm_open / sem_* live in librt on old glibc (< 2.34) and on
# FreeBSD, but in libc on musl (Alpine) and modern glibc (>= 2.34). Every consumer -
# netipc, libnetdata, netdata and CGO_LDFLAGS - keys off HAVE_LIBRT, so none of them
# unconditionally links a library that may not exist.
check_library_exists(rt clock_gettime "" HAVE_LIBRT)

#
# Libm
#

include(CheckFunctionExists)
include(CMakePushCheckState)

# CMP0075 (check_* honours CMAKE_REQUIRED_LIBRARIES) used to be set to NEW here
# explicitly, which is redundant: cmake_minimum_required(VERSION 3.16.0...3.30)
# already sets every policy up to 3.30 to NEW, and CMP0075 dates from 3.12.
# Confirmed NEW before any explicit set on both CMakes this project builds with,
# 3.30.3 and 4.1.6.
check_function_exists(log10 HAVE_LOG10)
if(NOT HAVE_LOG10)
        unset(HAVE_LOG10 CACHE)
        # -lm is wanted for this probe only. CMAKE_REQUIRED_LIBRARIES is global to
        # every later check_*, so restore it here rather than leaving the ~76 probes
        # in this file, and the ones in NetdataDetectSystemd, silently linking libm.
        cmake_push_check_state()
        list(APPEND CMAKE_REQUIRED_LIBRARIES m)
        check_function_exists(log10 HAVE_LOG10)
        cmake_pop_check_state()
        if(HAVE_LOG10)
                set(LINK_LIBM True)
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
check_include_file("arpa/inet.h" HAVE_ARPA_INET_H)
check_include_file("netinet/tcp.h" HAVE_NETINET_TCP_H)
check_include_file("sys/ioctl.h" HAVE_SYS_IOCTL_H)
check_include_file("grp.h" HAVE_GRP_H)
check_include_file("pwd.h" HAVE_PWD_H)
check_include_file("net/if.h" HAVE_NET_IF_H)
check_include_file("poll.h" HAVE_POLL_H)
check_include_file("syslog.h" HAVE_SYSLOG_H)
check_include_file("sys/mman.h" HAVE_SYS_MMAN_H)
check_include_file("sys/resource.h" HAVE_SYS_RESOURCE_H)
check_include_file("sys/socket.h" HAVE_SYS_SOCKET_H)
check_include_file("sys/wait.h" HAVE_SYS_WAIT_H)
check_include_file("sys/un.h" HAVE_SYS_UN_H)
check_include_file("spawn.h" HAVE_SPAWN_H)

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

check_function_exists(setresuid HAVE_SETRESUID)
check_function_exists(setresgid HAVE_SETRESGID)

check_function_exists(pthread_getthreadid_np HAVE_PTHREAD_GETTHREADID_NP)
check_function_exists(pthread_threadid_np HAVE_PTHREAD_THREADID_NP)
check_function_exists(gettid HAVE_GETTID)
check_function_exists(waitid HAVE_WAITID)
check_function_exists(nice HAVE_NICE)
check_function_exists(recvmmsg HAVE_RECVMMSG)
check_function_exists(getpriority HAVE_GETPRIORITY)
check_function_exists(setenv HAVE_SETENV)
check_function_exists(strndup HAVE_STRNDUP)

check_function_exists(sched_getscheduler HAVE_SCHED_GETSCHEDULER)
check_function_exists(sched_setscheduler HAVE_SCHED_SETSCHEDULER)
check_function_exists(sched_get_priority_min HAVE_SCHED_GET_PRIORITY_MIN)
check_function_exists(sched_get_priority_max HAVE_SCHED_GET_PRIORITY_MAX)

check_function_exists(close_range HAVE_CLOSE_RANGE)
check_function_exists(backtrace HAVE_BACKTRACE)

check_function_exists(arc4random_buf HAVE_ARC4RANDOM_BUF)
check_function_exists(arc4random_uniform HAVE_ARC4RANDOM_UNIFORM)
# Tests the DECLARATION, not just linkability, because the consumer in
# src/libnetdata/os/random.c guards an #include <sys/random.h> with this macro. A
# link-only test would say yes on a platform without the header and then fail to
# compile. It also matches the test bundled json-c runs under the same cache name,
# so whichever project configures first, the answer is the same one.
check_symbol_exists(getrandom "sys/random.h" HAVE_GETRANDOM)
check_function_exists(sysinfo HAVE_SYSINFO)

check_function_exists(timegm HAVE_TIMEGM)
check_function_exists(tzalloc HAVE_TZALLOC)
check_function_exists(localtime_rz HAVE_LOCALTIME_RZ)
check_function_exists(tzfree HAVE_TZFREE)

#
# check source compilation
#

include(CheckCSourceCompiles)
include(CheckCXXSourceCompiles)

check_c_source_compiles("
#include <time.h>
int main(void) {
    struct tm t;
    (void)t.tm_gmtoff;
    return 0;
}
" HAVE_TM_GMTOFF)

cmake_push_check_state()
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
cmake_pop_check_state()

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
#include <malloc.h>
int main() {
        malloc_trim(0);
        return 0;
}
" HAVE_C_MALLOC_TRIM)

check_c_source_compiles("
#include <malloc.h>
int main() {
        malloc_info(0, stdout);
        return 0;
}
" HAVE_C_MALLOC_INFO)

check_c_source_compiles("
#include <malloc.h>
int main() {
        struct mallinfo2 m = mallinfo2();
        return 0;
}
" HAVE_C_MALLINFO2)

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
void my_printf(char const *s, ...) __attribute__((format(gnu_printf, 1, 2)));
int main() { return 0; }
" HAVE_FUNC_ATTRIBUTE_FORMAT_GNU_PRINTF FAIL_REGEX "warning:")

check_c_source_compiles("
void my_printf(char const *s, ...) __attribute__((format(printf, 1, 2)));
int main() { return 0; }
" HAVE_FUNC_ATTRIBUTE_FORMAT_PRINTF FAIL_REGEX "warning:")

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
#include <stdlib.h>
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

# Windows MSVCRT random number generator
# used only when compiling natively (not MSYS/CYGWIN)
check_c_source_compiles("
    #define _CRT_RAND_S
    #include <stdlib.h>
    int main() {
        unsigned int x;
        return rand_s(&x);
    }
" HAVE_RAND_S)

if(OS_FREEBSD OR OS_MACOS)
        set(HAVE_BUILTIN_ATOMICS True)
endif()
