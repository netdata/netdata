// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef SPAWN_SERVER_H
#define SPAWN_SERVER_H

#define SPAWN_SERVER_TRANSFER_FDS 3

typedef enum {
    SPAWN_INSTANCE_TYPE_EXEC = 0,
    SPAWN_INSTANCE_TYPE_CALLBACK = 1
} SPAWN_INSTANCE_TYPE;

// the request at the worker process
typedef struct {
    size_t request_id;
    int socket;
    int fds[SPAWN_SERVER_TRANSFER_FDS]; // 0 = stdin, 1 = stdout, 2 = stderr
    const char **environment;
    const char **argv;
    const void *data;
    size_t data_size;
    SPAWN_INSTANCE_TYPE type;
} SPAWN_REQUEST;

typedef void (*spawn_request_callback_t)(SPAWN_REQUEST *request);

// the request at the parent process
typedef struct {
    size_t request_id;
    int write_fd;
    int read_fd;
    pid_t child_pid;
} SPAWN_INSTANCE;

// the spawn server at the parent process
typedef struct {
    int pipe[2];
    int server_sock;
    int client_sock;
    pid_t server_pid;
    char *path;
    size_t request_id;
    SPINLOCK spinlock;
    spawn_request_callback_t cb;
} SPAWN_SERVER;

SPAWN_SERVER* spawn_server_create(spawn_request_callback_t child_callback);
void spawn_server_destroy(SPAWN_SERVER *server);

#endif //SPAWN_SERVER_H
