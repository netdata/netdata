// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

int os_waitid(idtype_t idtype, id_t id, siginfo_t *infop, int options) {
#if defined(HAVE_WAITID)
    return waitid(idtype, id, infop, options);
#else
    // emulate waitid() using waitpid()

    // a cache for WNOWAIT
    static const struct pid_status empty = { 0, 0 };
    static __thread struct pid_status last = { 0, 0 }; // the cache
    struct pid_status current = { 0, 0 };

    // zero the infop structure
    memset(infop, 0, sizeof(*infop));

    // from the infop structure we use only 3 fields:
    // - si_pid
    // - si_code
    // - si_status
    // so, we update only these 3

    switch(idtype) {
        case P_ALL:
            current.pid = waitpid((pid_t)-1, &current.status, options);
            if(options & WNOWAIT)
                last = current;
            else
                last = empty;
            break;

        case P_PID:
            if(last.pid == (pid_t)id) {
                current = last;
                last = empty;
            }
            else
                current.pid = waitpid((pid_t)id, &current.status, options);

            break;

        default:
            errno = ENOSYS;
            return -1;
    }

    if (current.pid > 0) {
        if (WIFEXITED(current.status)) {
            infop->si_code = CLD_EXITED;
            infop->si_status = WEXITSTATUS(current.status);
        } else if (WIFSIGNALED(current.status)) {
            infop->si_code = WTERMSIG(current.status) == SIGABRT ? CLD_DUMPED : CLD_KILLED;
            infop->si_status = WTERMSIG(current.status);
        } else if (WIFSTOPPED(current.status)) {
            infop->si_code = CLD_STOPPED;
            infop->si_status = WSTOPSIG(current.status);
        } else if (WIFCONTINUED(current.status)) {
            infop->si_code = CLD_CONTINUED;
            infop->si_status = SIGCONT;
        }
        infop->si_pid = current.pid;
        return 0;
    } else if (current.pid == 0) {
        // No change in state, depends on WNOHANG
        return 0;
    }

    return -1;
#endif
}
