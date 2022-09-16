// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_POPEN_H
#define NETDATA_POPEN_H 1

#include "../libnetdata.h"

#define PIPE_READ 0
#define PIPE_WRITE 1

/* custom_popene_variadic_internal_dont_use_directly flag definitions */
#define POPEN_FLAG_NONE        0
#define POPEN_FLAG_CREATE_PIPE 1 // Create a pipe like popen() when set, otherwise set stdout to /dev/null
#define POPEN_FLAG_CLOSE_FD    2 // Close all file descriptors other than STDIN_FILENO, STDOUT_FILENO, STDERR_FILENO

// the flags to be used by default
#define POPEN_FLAGS_DEFAULT (POPEN_FLAG_CREATE_PIPE|POPEN_FLAG_CLOSE_FD)

// mypopen_raw is the interface to use instead of custom_popene_variadic_internal_dont_use_directly()
// mypopen_raw will add the terminating NULL at the arguments list
// we append the parameter 'command' twice - this is because the underlying call needs the command to execute and the argv[0] to pass to it
#define mypopen_raw_default_flags_and_environment(pidptr, fpp, command, args...) custom_popene_variadic_internal_dont_use_directly(pidptr, environ, POPEN_FLAGS_DEFAULT, fpp, command, command, ##args, NULL)
#define mypopen_raw_default_flags(pidptr, env, fpp, command, args...) custom_popene_variadic_internal_dont_use_directly(pidptr, env, POPEN_FLAGS_DEFAULT, fpp, command, command, ##args, NULL)
#define mypopen_raw(pidptr, env, flags, fpp, command, args...) custom_popene_variadic_internal_dont_use_directly(pidptr, env, flags, fpp, command, command, ##args, NULL)

extern int custom_popene_variadic_internal_dont_use_directly(volatile pid_t *pidptr, char **env, uint8_t flags, FILE **fpp, const char *command, ...);

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
