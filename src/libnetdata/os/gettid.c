// SPDX-License-Identifier: GPL-3.0-or-later

#include "config.h"
#include "gettid.h"

#if defined(COMPILED_FOR_CYGWIN) || defined(COMPILED_FOR_MSYS)
#include <windows.h>
#else
#include <pthread.h>
#endif

pid_t os_gettid(void) {
#if defined(HAVE_GETTID)
    return gettid();
#elif defined(HAVE_PTHREAD_GETTHREADID_NP)
    return (pid_t)pthread_getthreadid_np();
#elif defined(HAVE_PTHREAD_THREADID_NP)
    uint64_t curthreadid;
    pthread_threadid_np(NULL, &curthreadid);
    return curthreadid;
#elif defined(CCOMPILED_FOR_CYGWIN) || defined(CCOMPILED_FOR_MSYS)
    return GetCurrentThreadId();
#elif defined(COMPILED_FOR_LINUX)
    return (pid_t)syscall(SYS_gettid);
#else
    return (pid_t)pthread_self();
#endif
}

static __thread pid_t gettid_cached_tid = 0;
pid_t gettid_cached(void) {
    if(unlikely(gettid_cached_tid == 0))
        gettid_cached_tid = os_gettid();

    return gettid_cached_tid;
}