// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_POPEN_H
#define NETDATA_POPEN_H 1

#include "../libnetdata.h"

#define PIPE_READ 0
#define PIPE_WRITE 1

/* custom_popene flag definitions */
#define POPEN_FLAG_CREATE_PIPE 1 // Create a pipe like popen() when set, otherwise set stdout to /dev/null
#define POPEN_FLAG_CLOSE_FD 2 // Close all file descriptors other than STDIN_FILENO, STDOUT_FILENO, STDERR_FILENO

extern int custom_popene(volatile pid_t *pidptr, char **env, uint8_t flags, FILE **fpp, const char *command, ...);

extern FILE *mypopen(const char *command, volatile pid_t *pidptr);
extern FILE *mypopene(const char *command, volatile pid_t *pidptr, char **env);
extern int mypclose(FILE *fp, pid_t pid);
extern int netdata_spawn(const char *command, volatile pid_t *pidptr);
extern int netdata_spawn_waitpid(pid_t pid);
extern void myp_init(void);
extern void myp_free(void);
extern int myp_reap(pid_t pid);

extern void signals_unblock(void);
extern void signals_reset(void);

#endif /* NETDATA_POPEN_H */
