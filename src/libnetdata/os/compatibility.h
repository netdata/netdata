// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNETDATA_OS_COMPATIBILITY_H
#define LIBNETDATA_OS_COMPATIBILITY_H

#include "config.h"

#if defined(COMPILED_FOR_LINUX) || defined(COMPILED_FOR_FREEBSD) || defined(COMPILED_FOR_MACOS)
#include <sys/syscall.h>
#include <sys/timex.h>
#endif

#if defined(COMPILED_FOR_CYGWIN) || defined(COMPILED_FOR_MSYS)
#include <windows.h>
#endif

#include <unistd.h>
#include <sys/wait.h>

// --------------------------------------------------------------------------------------------------------------------
// adjtimex

struct timex;
static inline int os_adjtimex(struct timex *buf __maybe_unused) {
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
// setresgid

static inline int os_setresgid(gid_t gid __maybe_unused, gid_t egid __maybe_unused, gid_t sgid __maybe_unused) {
#if defined(COMPILED_FOR_LINUX) || defined(COMPILED_FOR_FREEBSD)
    return setresgid(gid, egid, sgid);
#endif

#if defined(COMPILED_FOR_MACOS)
    return setregid(gid, egid);
#endif

    errno = ENOSYS;
    return -1;
}

// --------------------------------------------------------------------------------------------------------------------
// setresuid

static inline int os_setresuid(uid_t uid __maybe_unused, uid_t euid __maybe_unused, uid_t suid __maybe_unused) {
#if defined(COMPILED_FOR_LINUX) || defined(COMPILED_FOR_FREEBSD)
    return setresuid(uid, euid, suid);
#endif

#if defined(COMPILED_FOR_MACOS)
    return setreuid(uid, euid);
#endif

    errno = ENOSYS;
    return -1;
}

// --------------------------------------------------------------------------------------------------------------------
// getgrouplist

static inline int os_getgrouplist(const char *username __maybe_unused, gid_t gid __maybe_unused, gid_t *supplementary_groups __maybe_unused, int *ngroups __maybe_unused) {
#if defined(COMPILED_FOR_LINUX) || defined(COMPILED_FOR_FREEBSD)
    return getgrouplist(username, gid, supplementary_groups, ngroups);
#endif

#if defined(COMPILED_FOR_MACOS)
    return getgrouplist(username, gid, (int *)supplementary_groups, ngroups);
#endif

    errno = ENOSYS;
    return -1;
}

// --------------------------------------------------------------------------------------------------------------------
// waitid

#if defined(COMPILED_FOR_CYGWIN)
# define WSTOPPED	2	/* Report stopped child (same as WUNTRACED). */
# define WEXITED	4	/* Report dead child.  */
# define WCONTINUED	8	/* Report continued child.  */
# define WNOWAIT	0x01000000 /* Don't reap, just poll status.  */

typedef enum
{
    P_ALL,		/* Wait for any child.  */
    P_PID,		/* Wait for specified process.  */
    P_PGID,		/* Wait for members of process group.  */
    P_PIDFD,		/* Wait for the child referred by the PID file
			   descriptor.  */
} idtype_t;
#endif

static inline int os_waitid(idtype_t idtype __maybe_unused, id_t id __maybe_unused, siginfo_t *infop __maybe_unused, int options __maybe_unused) {
#if defined(COMPILED_FOR_LINUX) || defined(COMPILED_FOR_FREEBSD) || defined(COMPILED_FOR_MACOS)
    return waitid(idtype, id, infop, options);
#endif

    errno = ENOSYS;
    return -1;
}

// --------------------------------------------------------------------------------------------------------------------
// gettid

pid_t gettid_cached(void);

static inline pid_t os_gettid(void) {
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

#endif //LIBNETDATA_OS_COMPATIBILITY_H
