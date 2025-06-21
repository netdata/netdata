// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"

#if !defined(HAVE_RUST_PROVIDER)

// ----------------------------------------------------------------------------
// fstat64 overloading to speed up libsystemd
// https://github.com/systemd/systemd/pull/29261

#include <dlfcn.h>
#include <sys/stat.h>

#define FSTAT_CACHE_MAX 1024
struct fdstat64_cache_entry {
    bool enabled;
    bool updated;
    int err_no;
    struct stat64 stat;
    int ret;
    size_t cached_count;
    size_t session;
};

struct fdstat64_cache_entry fstat64_cache[FSTAT_CACHE_MAX] = {0};
__thread size_t fstat_thread_calls = 0;
__thread size_t fstat_thread_cached_responses = 0;
static __thread bool enable_thread_fstat = false;
static __thread size_t fstat_caching_thread_session = 0;
static size_t fstat_caching_global_session = 0;

void fstat_cache_enable_on_thread(void)
{
    fstat_caching_thread_session = __atomic_add_fetch(&fstat_caching_global_session, 1, __ATOMIC_ACQUIRE);
    enable_thread_fstat = true;
}

void fstat_cache_disable_on_thread(void)
{
    fstat_caching_thread_session = __atomic_add_fetch(&fstat_caching_global_session, 1, __ATOMIC_RELEASE);
    enable_thread_fstat = false;
}

int fstat64(int fd, struct stat64 *buf)
{
    static int (*real_fstat)(int, struct stat64 *) = NULL;
    if (!real_fstat)
        real_fstat = dlsym(RTLD_NEXT, "fstat64");

    fstat_thread_calls++;

    if (fd >= 0 && fd < FSTAT_CACHE_MAX) {
        if (enable_thread_fstat && fstat64_cache[fd].session != fstat_caching_thread_session) {
            fstat64_cache[fd].session = fstat_caching_thread_session;
            fstat64_cache[fd].enabled = true;
            fstat64_cache[fd].updated = false;
        }

        if (fstat64_cache[fd].enabled && fstat64_cache[fd].updated &&
            fstat64_cache[fd].session == fstat_caching_thread_session) {
            fstat_thread_cached_responses++;
            errno = fstat64_cache[fd].err_no;
            *buf = fstat64_cache[fd].stat;
            fstat64_cache[fd].cached_count++;
            return fstat64_cache[fd].ret;
        }
    }

    int ret = real_fstat(fd, buf);

    if (fd >= 0 && fd < FSTAT_CACHE_MAX && fstat64_cache[fd].enabled &&
        fstat64_cache[fd].session == fstat_caching_thread_session) {
        fstat64_cache[fd].ret = ret;
        fstat64_cache[fd].updated = true;
        fstat64_cache[fd].err_no = errno;
        fstat64_cache[fd].stat = *buf;
    }

    return ret;
}

#else // HAVE_RUST_PROVIDER

// When using Rust provider, disable fstat caching entirely since
// we will not rely on libsystemd.

__thread size_t fstat_thread_calls = 0;
__thread size_t fstat_thread_cached_responses = 0;

void fstat_cache_enable_on_thread(void) { }
void fstat_cache_disable_on_thread(void) { }

#endif
