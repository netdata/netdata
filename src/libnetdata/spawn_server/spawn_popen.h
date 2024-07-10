// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef SPAWN_POPEN_H
#define SPAWN_POPEN_H

#include "../libnetdata.h"

extern SPAWN_SERVER *netdata_main_spawn_server;
bool netdata_main_spawn_server_init(const char *name, int argc, const char **argv);
void netdata_main_spawn_server_cleanup(void);

typedef struct {
    SPAWN_INSTANCE *si;
    FILE *child_stdin_fp;
    FILE *child_stdout_fp;
} POPEN_INSTANCE;

POPEN_INSTANCE *spawn_popen_run(const char *cmd);
POPEN_INSTANCE *spawn_popen_run_argv(const char **argv);
POPEN_INSTANCE *spawn_popen_run_variadic(const char *cmd, ...);
int spawn_popen_wait(POPEN_INSTANCE *pi);
int spawn_popen_kill(POPEN_INSTANCE *pi);

#endif //SPAWN_POPEN_H
