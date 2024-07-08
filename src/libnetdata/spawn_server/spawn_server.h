// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef SPAWN_SERVER_H
#define SPAWN_SERVER_H

#define SPAWN_SERVER_TRANSFER_FDS 4

typedef enum {
    SPAWN_INSTANCE_TYPE_EXEC = 0,
    SPAWN_INSTANCE_TYPE_CALLBACK = 1
} SPAWN_INSTANCE_TYPE;

// the request at the worker process
typedef struct spawn_request {
    size_t request_id;
    pid_t pid;
    int socket;
    int fds[SPAWN_SERVER_TRANSFER_FDS]; // 0 = stdin, 1 = stdout, 2 = stderr, 3 = custom
    const char **environment;
    const char **argv;
    const void *data;
    size_t data_size;
    SPAWN_INSTANCE_TYPE type;
    struct spawn_request *prev, *next;
} SPAWN_REQUEST;

typedef void (*spawn_request_callback_t)(SPAWN_REQUEST *request);

// the request at the parent process
typedef struct {
    size_t request_id;
    int client_sock;
    int write_fd;
    int read_fd;
    pid_t child_pid;
} SPAWN_INSTANCE;

// the spawn server at the parent process
typedef struct {
    size_t id;
    const char *name;
    int pipe[2];
    int server_sock;
    pid_t server_pid;
    char *path;
    size_t request_id;
    spawn_request_callback_t cb;

    char **argv;
    size_t argv0_size;
} SPAWN_SERVER;

SPAWN_SERVER* spawn_server_create(const char *name, spawn_request_callback_t child_callback, int argc, char **argv);
void spawn_server_destroy(SPAWN_SERVER *server);

SPAWN_INSTANCE* spawn_server_exec(SPAWN_SERVER *server, int stderr_fd, int custom_fd, const char **argv, const void *data, size_t data_size, SPAWN_INSTANCE_TYPE type);
int spawn_server_exec_kill(SPAWN_SERVER *server, SPAWN_INSTANCE *instance);
int spawn_server_exec_wait(SPAWN_SERVER *server, SPAWN_INSTANCE *instance);

#endif //SPAWN_SERVER_H
