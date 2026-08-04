// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef SPAWN_SERVER_H
#define SPAWN_SERVER_H

#include "../common.h"
#include "../environment/environment.h"

#define SPAWN_SERVER_TRANSFER_FDS 4

typedef enum __attribute__((packed)) {
    SPAWN_INSTANCE_TYPE_EXEC = 0,
    SPAWN_INSTANCE_TYPE_CALLBACK = 1
} SPAWN_INSTANCE_TYPE;

typedef enum __attribute__((packed)) {
    SPAWN_SERVER_OPTION_EXEC = (1 << 0),
    SPAWN_SERVER_OPTION_CALLBACK = (1 << 1),
} SPAWN_SERVER_OPTIONS;

// this is only used publicly for SPAWN_INSTANCE_TYPE_CALLBACK
// which is not available in Windows
typedef struct spawn_request {
    const char *cmdline;                // the cmd line of the command we should run
    size_t request_id;                  // the incremental request id
    pid_t pid;                          // the pid of the child
    int sock;                           // the socket for this request
    int fds[SPAWN_SERVER_TRANSFER_FDS]; // 0 = stdin, 1 = stdout, 2 = stderr, 3 = custom
    const char **envp;                  // the environment of the parent process
    const char **argv;                  // the command line and its parameters
    const void *data;                   // the data structure for the callback
    size_t data_size;                   // the data structure size
    SPAWN_INSTANCE_TYPE type;           // the type of the request

    struct spawn_request *prev, *next;  // linking of active requests at the spawn server
} SPAWN_REQUEST;

typedef int (*spawn_request_callback_t)(SPAWN_REQUEST *request);

typedef struct spawn_instance SPAWN_INSTANCE;
typedef struct spawn_server SPAWN_SERVER;

// The environment context remains caller-owned and must outlive the server.
SPAWN_SERVER *spawn_server_create(SPAWN_SERVER_OPTIONS options, const char *name,
                                  spawn_request_callback_t child_callback, int argc, const char **argv,
                                  ND_ENVIRONMENT *environment, const char *runtime_directory);
#if defined(OS_WINDOWS)
bool spawn_server_windows_publish_cygwin_path(void);
#endif
void spawn_server_destroy(SPAWN_SERVER *server);
pid_t spawn_server_pid(SPAWN_SERVER *server);

SPAWN_INSTANCE* spawn_server_exec(SPAWN_SERVER *server, int stderr_fd, int custom_fd, const char **argv, const void *data, size_t data_size, SPAWN_INSTANCE_TYPE type);
int spawn_server_exec_kill(SPAWN_SERVER *server, SPAWN_INSTANCE *si, int timeout_ms);
int spawn_server_exec_wait(SPAWN_SERVER *server, SPAWN_INSTANCE *si);

typedef enum __attribute__((packed)) {
    SPAWN_TIMEDWAIT_EXITED = 0,     // the child exited; *status holds its status and si has been freed
    SPAWN_TIMEDWAIT_RUNNING = 1,    // the timeout expired; the child is still running and si remains valid
    SPAWN_TIMEDWAIT_ERROR = 2,      // the wait could not be completed (broken status channel / unusable
                                    // handle); the child's state is unknown, si remains valid, and the
                                    // caller must reclaim it (e.g. via spawn_server_exec_kill())
} SPAWN_TIMEDWAIT_RESULT;

// Wait for the child for up to timeout_ms. A non-positive timeout_ms performs a single, minimal
// bounded poll (the nofork backend uses a ~1ms slice because its wait primitive treats 0 as
// "wait forever"; the others poll once); the wait is always bounded, never infinite.
// On SPAWN_TIMEDWAIT_EXITED the instance has been freed, exactly like spawn_server_exec_wait().
// On SPAWN_TIMEDWAIT_RUNNING or SPAWN_TIMEDWAIT_ERROR the caller keeps ownership and must eventually
// call spawn_server_exec_timedwait(), spawn_server_exec_wait() or spawn_server_exec_kill().
// ERROR is distinct from RUNNING on purpose: the wait could not progress (it is not merely "not yet"),
// so callers must NOT loop on it (that would spin forever when timeout_ms == 0) - they must reclaim
// the instance, typically by killing it.
// status is optional (may be NULL): on EXITED the wait/cleanup still happens, the status is just not stored.
SPAWN_TIMEDWAIT_RESULT spawn_server_exec_timedwait(SPAWN_SERVER *server, SPAWN_INSTANCE *si, int timeout_ms, int *status);

int spawn_server_instance_read_fd(SPAWN_INSTANCE *si);
int spawn_server_instance_write_fd(SPAWN_INSTANCE *si);
pid_t spawn_server_instance_pid(SPAWN_INSTANCE *si);
void spawn_server_instance_read_fd_unset(SPAWN_INSTANCE *si);
void spawn_server_instance_write_fd_unset(SPAWN_INSTANCE *si);

#endif //SPAWN_SERVER_H
