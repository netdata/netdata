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

#ifndef WNOWAIT
#define WNOWAIT 0x01000000
#endif

#ifndef WEXITED
#define WEXITED 4
#endif

#if !defined(HAVE_WAITID)
typedef enum
{
    P_ALL,     /* Wait for any child.  */
    P_PID,     /* Wait for specified process.  */
    P_PGID,    /* Wait for members of process group.  */
    P_PIDFD,   /* Wait for the child referred by the PID file descriptor.  */
} idtype_t;
#endif

static inline int os_waitid(idtype_t idtype, id_t id, siginfo_t *infop, int options) {
#if defined(HAVE_WAITID)
    return waitid(idtype, id, infop, options);
#else
    static __thread struct {
        pid_t pid;
        int status;
    } last_p_all = { 0, 0 };

    memset(infop, 0, sizeof(*infop));
    int status;
    pid_t pid;

    switch(idtype) {
        case P_ALL:
            pid = waitpid((pid_t)-1, &status, options);
            if(options & WNOWAIT) {
                last_p_all.pid = pid;           // the cache is updated
                last_p_all.status = status;     // the cache is updated
            }
            else {
                last_p_all.pid = 0;             // the cache is empty
                last_p_all.status = 0;          // the cache is empty
            }
            break;

        case P_PID:
            if(last_p_all.pid == pid) {
                pid = last_p_all.pid;
                status = last_p_all.status;
                last_p_all.pid = 0;             // the cache is used
                last_p_all.status = 0;          // the cache is used
            }
            else
                pid = waitpid((pid_t)id, &status, options);

            break;

        default:
            errno = ENOSYS;
            return -1;
    }

    if (pid > 0) {
        if (WIFEXITED(status)) {
            infop->si_code = CLD_EXITED;
            infop->si_status = WEXITSTATUS(status);
        } else if (WIFSIGNALED(status)) {
            infop->si_code = WTERMSIG(status) == SIGABRT ? CLD_DUMPED : CLD_KILLED;
            infop->si_status = WTERMSIG(status);
        } else if (WIFSTOPPED(status)) {
            infop->si_code = CLD_STOPPED;
            infop->si_status = WSTOPSIG(status);
        } else if (WIFCONTINUED(status)) {
            infop->si_code = CLD_CONTINUED;
            infop->si_status = SIGCONT;
        }
        infop->si_pid = pid;
        return 0;
    } else if (pid == 0) {
        // No change in state, depends on WNOHANG
        return 0;
    }

    // error case
    return -1;
#endif
}

// --------------------------------------------------------------------------------------------------------------------
// gettid

pid_t gettid_cached(void);
pid_t os_gettid(void);

#endif //LIBNETDATA_OS_COMPATIBILITY_H
