// SPDX-License-Identifier: GPL-3.0-or-later

#include "config.h"
#include "compatibility.h"

#if defined(COMPILED_FOR_CYGWIN) || defined(COMPILED_FOR_MSYS)
#include <windows.h>
#endif

// --------------------------------------------------------------------------------------------------------------------

int os_adjtimex(struct timex *buf __maybe_unused) {
#if defined(COMPILED_FOR_MACOS) || defined(COMPILED_FOR_FREEBSD)
    return ntp_adjtime(buf);
#endif

#if defined(COMPILED_FOR_LINUX)
    return adjtimex(buf);
#endif

    errno = ENOSYS;
    return -1;
}

// --------------------------------------------------------------------------------------------------------------------

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

// --------------------------------------------------------------------------------------------------------------------

