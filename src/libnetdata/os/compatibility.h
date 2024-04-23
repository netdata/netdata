// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNETDATA_OS_COMPATIBILITY_H
#define LIBNETDATA_OS_COMPATIBILITY_H

#include "config.h"

#if defined(COMPILED_FOR_LINUX) || defined(COMPILED_FOR_FREEBSD) || defined(COMPILED_FOR_MACOS)
#include <sys/syscall.h>
#include <sys/timex.h>
#endif

#include <errno.h>
#include <pthread.h>
#include <unistd.h>
#include <sys/wait.h>
#include <grp.h>

// --------------------------------------------------------------------------------------------------------------------
// adjtimex

struct timex;
int os_adjtimex(struct timex *buf __maybe_unused);

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
pid_t os_gettid(void);

#endif //LIBNETDATA_OS_COMPATIBILITY_H
