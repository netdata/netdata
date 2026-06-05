// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef SPAWN_POPEN_H
#define SPAWN_POPEN_H

#include "../libnetdata.h"

extern SPAWN_SERVER *netdata_main_spawn_server;
bool netdata_main_spawn_server_init(const char *name, int argc, const char **argv);
void netdata_main_spawn_server_cleanup(void);

typedef struct popen_instance POPEN_INSTANCE;

POPEN_INSTANCE *spawn_popen_run(const char *cmd);
POPEN_INSTANCE *spawn_popen_run_argv(const char **argv);
POPEN_INSTANCE *spawn_popen_run_variadic(const char *cmd, ...);
int spawn_popen_wait(POPEN_INSTANCE *pi);

// Wait for the child for up to timeout_ms. A non-positive timeout_ms performs a single, minimal
// bounded poll; the wait is always bounded, never infinite.
// SPAWN_TIMEDWAIT_EXITED: the child exited; *code holds its spawn_popen_wait()-style return code
//   and pi has been freed.
// SPAWN_TIMEDWAIT_RUNNING: still running after timeout_ms; pi remains valid - call again, or
//   spawn_popen_wait()/spawn_popen_kill().
// SPAWN_TIMEDWAIT_ERROR: the wait could not be completed (the child's state is unknown); pi remains
//   valid and the caller must reclaim it with spawn_popen_kill(). Do NOT loop on ERROR.
SPAWN_TIMEDWAIT_RESULT spawn_popen_timedwait(POPEN_INSTANCE *pi, int timeout_ms, int *code);

int spawn_popen_kill(POPEN_INSTANCE *pi, int timeout_ms);

pid_t spawn_popen_pid(POPEN_INSTANCE *pi);
int spawn_popen_read_fd(POPEN_INSTANCE *pi);
int spawn_popen_write_fd(POPEN_INSTANCE *pi);
FILE *spawn_popen_stdin(POPEN_INSTANCE *pi);
FILE *spawn_popen_stdout(POPEN_INSTANCE *pi);

#endif //SPAWN_POPEN_H
