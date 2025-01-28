// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

pid_t os_gettid(void) {
#if defined(HAVE_GETTID)
    return gettid();
#elif defined(HAVE_PTHREAD_GETTHREADID_NP)
    return (pid_t)pthread_getthreadid_np();
#elif defined(HAVE_PTHREAD_THREADID_NP)
    uint64_t curthreadid;
    pthread_threadid_np(NULL, &curthreadid);
    return curthreadid;
#elif defined(OS_WINDOWS)
    return (pid_t)GetCurrentThreadId();
#elif defined(OS_LINUX)
    return (pid_t)syscall(SYS_gettid);
#else
    return (pid_t)pthread_self();
#endif
}

static __thread pid_t gettid_cached_tid = 0;
ALWAYS_INLINE pid_t gettid_cached(void) {
    if(unlikely(gettid_cached_tid == 0))
        gettid_cached_tid = os_gettid();

    return gettid_cached_tid;
}

pid_t gettid_uncached(void) {
    gettid_cached_tid = 0;
    return gettid_cached();
}
