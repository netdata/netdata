// SPDX-License-Identifier: GPL-3.0-or-later

#include "spawn_server_internals.h"

#if defined(SPAWN_SERVER_VERSION_UV)

int spawn_server_instance_read_fd(SPAWN_INSTANCE *si) { return si->read_fd; }
int spawn_server_instance_write_fd(SPAWN_INSTANCE *si) { return si->write_fd; }
void spawn_server_instance_read_fd_unset(SPAWN_INSTANCE *si) { si->read_fd = -1; }
void spawn_server_instance_write_fd_unset(SPAWN_INSTANCE *si) { si->write_fd = -1; }
pid_t spawn_server_instance_pid(SPAWN_INSTANCE *si) { return si->child_pid; }

typedef struct work_item {
    SPAWN_SERVER *server;
    SPAWN_INSTANCE *instance;
    uv_process_options_t options;
    struct work_item *prev;
    struct work_item *next;
} work_item;

static void server_thread(void *arg) {
    SPAWN_SERVER *server = (SPAWN_SERVER *)arg;
    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "SPAWN SERVER: started");

    uv_run(server->loop, UV_RUN_DEFAULT);

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "SPAWN SERVER: ended");
}

static void async_callback(uv_async_t *handle) {
    nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN SERVER: dequeue commands started");

    SPAWN_SERVER *server = (SPAWN_SERVER *)handle->data;
    work_item *item;
    spinlock_lock(&server->spinlock);
    while (server->work_queue) {
        item = server->work_queue;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(server->work_queue, item, prev, next);
        spinlock_unlock(&server->spinlock);

        SPAWN_INSTANCE *instance = item->instance;
        nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN SERVER: got an instance, running uv_spawn()");
        int rc = uv_spawn(server->loop, &instance->process, &item->options);
        nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN SERVER: uv_spawn() returned %d", rc);

        if (rc) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: uv_spawn() failed");
            uv_sem_post(&instance->sem); // Signal failure
        }
        else {
            // Successfully spawned

            // get the pid of the process spawned
            instance->child_pid = item->instance->process.pid;

            // close the child sides of the pipes
            if(instance->stdin_pipe[PIPE_READ] != -1) { close(instance->stdin_pipe[PIPE_READ]); instance->stdin_pipe[PIPE_READ] = -1; }
            if(instance->stdout_pipe[PIPE_WRITE] != -1) { close(instance->stdout_pipe[PIPE_WRITE]); instance->stdout_pipe[PIPE_WRITE] = -1; }

            // on_process_exit() needs this to find the instance
            item->instance->process.data = item->instance;

            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN SERVER: process created with pid %d",
                   instance->child_pid);
        }

        freez(item);
        spinlock_lock(&server->spinlock);
    }
    spinlock_unlock(&server->spinlock);

    nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN SERVER: dequeue commands done");
}

SPAWN_SERVER* spawn_server_create(SPAWN_SERVER_OPTIONS options __maybe_unused, const char *name, spawn_request_callback_t cb  __maybe_unused, int argc __maybe_unused, const char **argv __maybe_unused) {
    SPAWN_SERVER* server = callocz(1, sizeof(SPAWN_SERVER));
    spinlock_init(&server->spinlock);

    if (name)
        server->name = strdupz(name);
    else
        server->name = strdupz("unnamed");

    server->loop = mallocz(sizeof(uv_loop_t));
    if (uv_loop_init(server->loop)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: uv_loop_init() failed");
        freez(server->loop);
        freez((void *)server->name);
        freez(server);
        return NULL;
    }

    if (uv_async_init(server->loop, &server->async, async_callback)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: uv_async_init() failed");
        uv_loop_close(server->loop);
        freez(server->loop);
        freez((void *)server->name);
        freez(server);
        return NULL;
    }
    server->async.data = server;

    if (uv_thread_create(&server->thread, server_thread, server)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: uv_thread_create() failed");
        uv_close((uv_handle_t*)&server->async, NULL);
        uv_loop_close(server->loop);
        freez(server->loop);
        freez((void *)server->name);
        freez(server);
        return NULL;
    }

    return server;
}

void spawn_server_destroy(SPAWN_SERVER *server) {
    if (server) {
        uv_stop(server->loop);
        uv_thread_join(&server->thread);
        uv_close((uv_handle_t*)&server->async, NULL);
        uv_loop_close(server->loop);
        freez(server->loop);
        freez((void *)server->name);
        freez(server);
    }
}

static void on_process_exit(uv_process_t *req, int64_t exit_status, int term_signal __maybe_unused) {
    SPAWN_INSTANCE *instance = (SPAWN_INSTANCE *)req->data;
    instance->exit_code = (int)exit_status;
    uv_sem_post(&instance->sem); // Signal that the process has exited
    uv_close((uv_handle_t *)req, NULL);

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "SPAWN SERVER: process with pid %d exited with code %d and term_signal %d",
           instance->child_pid, (int)exit_status, term_signal);
}

SPAWN_INSTANCE* spawn_server_exec(SPAWN_SERVER *server, int stderr_fd __maybe_unused, int custom_fd __maybe_unused, const char **argv, const void *data __maybe_unused, size_t data_size __maybe_unused, SPAWN_INSTANCE_TYPE type) {
    if (type != SPAWN_INSTANCE_TYPE_EXEC)
        return NULL;

    SPAWN_INSTANCE *instance = callocz(1, sizeof(SPAWN_INSTANCE));
    instance->child_pid = -1;
    instance->exit_code = -1;
    instance->stdin_pipe[PIPE_READ] = -1;
    instance->stdin_pipe[PIPE_WRITE] = -1;
    instance->stdout_pipe[PIPE_READ] = -1;
    instance->stdout_pipe[PIPE_WRITE] = -1;
    instance->request_id = __atomic_add_fetch(&server->request_id, 1, __ATOMIC_RELAXED);

    if (uv_sem_init(&instance->sem, 0)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: uv_sem_init() failed");
        freez(instance);
        return NULL;
    }

    work_item *item = callocz(1, sizeof(work_item));
    item->server = server;
    item->instance = instance;
    item->options.exit_cb = on_process_exit;
    item->options.file = argv[0];
    item->options.args = (char **)argv;
    item->options.env = (char **)environ;

    if (pipe(instance->stdin_pipe) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: stdin pipe() failed");
        freez(item);
        uv_sem_destroy(&instance->sem);
        freez(instance);
        return NULL;
    }

    if (pipe(instance->stdout_pipe) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: stdout pipe() failed");
        close(instance->stdin_pipe[PIPE_READ]);
        close(instance->stdin_pipe[PIPE_WRITE]);
        freez(item);
        uv_sem_destroy(&instance->sem);
        freez(instance);
        return NULL;
    }

    item->options.stdio_count = 3;
    uv_stdio_container_t stdio[3];
    stdio[0].flags = UV_INHERIT_FD;
    stdio[0].data.fd = instance->stdin_pipe[PIPE_READ];
    stdio[1].flags = UV_INHERIT_FD;
    stdio[1].data.fd = instance->stdout_pipe[PIPE_WRITE];
    stdio[2].flags = UV_INHERIT_FD;
    stdio[2].data.fd = STDERR_FILENO;
    item->options.stdio = stdio;

    instance->write_fd = instance->stdin_pipe[PIPE_WRITE]; instance->stdin_pipe[PIPE_WRITE] = -1;
    instance->read_fd = instance->stdout_pipe[PIPE_READ]; instance->stdout_pipe[PIPE_READ] = -1;

    spinlock_lock(&server->spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(server->work_queue, item, prev, next);
    spinlock_unlock(&server->spinlock);

    uv_async_send(&server->async);

    nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN PARENT: queued command");

    return instance;
}

int spawn_server_exec_kill(SPAWN_SERVER *server __maybe_unused, SPAWN_INSTANCE *instance) {
    if(!instance) return -1;

    if (uv_process_kill(&instance->process, SIGTERM)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: uv_process_kill() failed");
        return -1;
    }

    return spawn_server_exec_wait(server, instance);
}

int spawn_server_exec_wait(SPAWN_SERVER *server __maybe_unused, SPAWN_INSTANCE *instance) {
    if (!instance) return -1;

    // close all pipe descriptors to force the child to exit
    if(instance->read_fd != -1) close(instance->read_fd);
    if(instance->write_fd != -1) close(instance->write_fd);
    if(instance->stdin_pipe[PIPE_READ] != -1) { close(instance->stdin_pipe[PIPE_READ]); instance->stdin_pipe[PIPE_READ] = -1; }
    if(instance->stdin_pipe[PIPE_WRITE] != -1) { close(instance->stdin_pipe[PIPE_WRITE]); instance->stdin_pipe[PIPE_WRITE] = -1; }
    if(instance->stdout_pipe[PIPE_READ] != -1) { close(instance->stdout_pipe[PIPE_READ]); instance->stdout_pipe[PIPE_READ] = -1; }
    if(instance->stdout_pipe[PIPE_WRITE] != -1) { close(instance->stdout_pipe[PIPE_WRITE]); instance->stdout_pipe[PIPE_WRITE] = -1; }

    // Wait for the process to exit
    uv_sem_wait(&instance->sem);
    int exit_code = instance->exit_code;
    uv_sem_destroy(&instance->sem);

    freez(instance);
    return exit_code;
}

#endif
