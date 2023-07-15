// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_POPEN_H
#define NETDATA_POPEN_H 1

#include "../libnetdata.h"

#define PIPE_READ 0
#define PIPE_WRITE 1

/* custom_popene_variadic_internal_dont_use_directly flag definitions */
#define POPEN_FLAG_NONE        0
#define POPEN_FLAG_CLOSE_FD    (1 << 0) // Close all file descriptors other than STDIN_FILENO, STDOUT_FILENO, STDERR_FILENO

// the flags to be used by default
#define POPEN_FLAGS_DEFAULT (POPEN_FLAG_CLOSE_FD)

// mypopen_raw is the interface to use instead of custom_popene_variadic_internal_dont_use_directly()
// mypopen_raw will add the terminating NULL at the arguments list
// we append the parameter 'command' twice - this is because the underlying call needs the command to execute and the argv[0] to pass to it
#define netdata_popen_raw_default_flags_and_environment(pidptr, fpp_child_input, fpp_child_output, command, args...) netdata_popene_variadic_internal_dont_use_directly(pidptr, environ, POPEN_FLAGS_DEFAULT, fpp_child_input, fpp_child_output, command, command, ##args, NULL)
#define netdata_popen_raw_default_flags(pidptr, env, fpp_child_input, fpp_child_output, command, args...) netdata_popene_variadic_internal_dont_use_directly(pidptr, env, POPEN_FLAGS_DEFAULT, fpp_child_input, fpp_child_output, command, command, ##args, NULL)
#define netdata_popen_raw(pidptr, env, flags, fpp_child_input, fpp_child_output, command, args...) netdata_popene_variadic_internal_dont_use_directly(pidptr, env, flags, fpp_child_input, fpp_child_output, command, command, ##args, NULL)

FILE *netdata_popen(const char *command, volatile pid_t *pidptr, FILE **fp_child_input);
FILE *netdata_popene(const char *command, volatile pid_t *pidptr, char **env, FILE **fp_child_input);
int netdata_popene_variadic_internal_dont_use_directly(volatile pid_t *pidptr, char **env, uint8_t flags, FILE **fpp_child_input, FILE **fpp_child_output, const char *command, ...);
int netdata_pclose(FILE *fp_child_input, FILE *fp_child_output, pid_t pid);

int netdata_spawn(const char *command, volatile pid_t *pidptr);
int netdata_waitid(idtype_t idtype, id_t id, siginfo_t *infop, int options);

#endif /* NETDATA_POPEN_H */
