// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WAITID_H
#define NETDATA_WAITID_H

#include <sys/wait.h>

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

struct pid_status {
    pid_t pid;
    int status;
};
#endif

int os_waitid(idtype_t idtype, id_t id, siginfo_t *infop, int options);

#endif //NETDATA_WAITID_H
