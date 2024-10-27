// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SPAWN_SERVER_INTERNALS_H
#define NETDATA_SPAWN_SERVER_INTERNALS_H

#include "../libnetdata.h"
#include "spawn_server.h"
#include "spawn_library.h"
#include "log-forwarder.h"

#if defined(OS_WINDOWS)
#define SPAWN_SERVER_VERSION_WINDOWS 1
// #define SPAWN_SERVER_VERSION_UV 1
// #define SPAWN_SERVER_VERSION_POSIX_SPAWN 1
#else
#define SPAWN_SERVER_VERSION_NOFORK 1
// #define SPAWN_SERVER_VERSION_UV 1
// #define SPAWN_SERVER_VERSION_POSIX_SPAWN 1
#endif

struct spawn_server {
    size_t id;
    size_t request_id;
    const char *name;

#if defined(SPAWN_SERVER_VERSION_UV)
    uv_loop_t *loop;
    uv_thread_t thread;
    uv_async_t async;
    bool stopping;

    SPINLOCK spinlock;
    struct work_item *work_queue;
#endif

#if defined(SPAWN_SERVER_VERSION_NOFORK)
    SPAWN_SERVER_OPTIONS options;

    ND_UUID magic;          // for authorizing requests, the client needs to know our random UUID
                            // it is ignored for PING requests

    int pipe[2];
    int sock;               // the listening socket of the server
    pid_t server_pid;
    char *path;
    spawn_request_callback_t cb;

    int argc;
    const char **argv;
#endif

#if defined(SPAWN_SERVER_VERSION_POSIX_SPAWN)
#endif

#if defined(SPAWN_SERVER_VERSION_WINDOWS)
    LOG_FORWARDER *log_forwarder;
#endif
};

struct spawn_instance {
    size_t request_id;
    int sock;
    int write_fd;       // the child's input pipe, writing side
    int read_fd;        // the child's output pipe, reading side
    int stderr_fd;
    pid_t child_pid;

#if defined(SPAWN_SERVER_VERSION_UV)
    uv_process_t process;
    int exit_code;
    uv_sem_t sem;
#endif

#if defined(SPAWN_SERVER_VERSION_NOFORK)
#endif

#if defined(SPAWN_SERVER_VERSION_POSIX_SPAWN)
    const char *cmdline;
    bool exited;
    int waitpid_status;
    struct spawn_instance *prev, *next;
#endif

#if defined(SPAWN_SERVER_VERSION_WINDOWS)
    HANDLE process_handle;
    DWORD dwProcessId;
#endif
};

#endif //NETDATA_SPAWN_SERVER_INTERNALS_H
