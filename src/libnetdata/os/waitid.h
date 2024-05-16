// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WAITID_H
#define NETDATA_WAITID_H

#include "config.h"
#include <sys/types.h>
#include <signal.h>

#ifdef HAVE_SYS_WAIT_H
#include <sys/wait.h>
#endif

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

#if defined(COMPILED_FOR_WINDOWS) && !defined(__CYGWIN__)
typedef uint32_t id_t;
typedef struct {
    int si_code;	/* Signal code.  */
    int si_status;	/* Exit value or signal.  */
    pid_t si_pid;	/* Sending process ID.  */
} siginfo_t;
#endif
#endif

int os_waitid(idtype_t idtype, id_t id, siginfo_t *infop, int options);

#endif //NETDATA_WAITID_H
