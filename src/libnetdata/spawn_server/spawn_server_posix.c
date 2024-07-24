// SPDX-License-Identifier: GPL-3.0-or-later

#include "spawn_server_internals.h"

#if defined(SPAWN_SERVER_VERSION_POSIX_SPAWN)

#ifdef __APPLE__
#include <crt_externs.h>
#define environ (*_NSGetEnviron())
#else
extern char **environ;
#endif

int spawn_server_instance_read_fd(SPAWN_INSTANCE *si) { return si->read_fd; }
int spawn_server_instance_write_fd(SPAWN_INSTANCE *si) { return si->write_fd; }
void spawn_server_instance_read_fd_unset(SPAWN_INSTANCE *si) { si->read_fd = -1; }
void spawn_server_instance_write_fd_unset(SPAWN_INSTANCE *si) { si->write_fd = -1; }
pid_t spawn_server_instance_pid(SPAWN_INSTANCE *si) { return si->child_pid; }

static struct {
    bool sigchld_initialized;
    SPINLOCK spinlock;
    SPAWN_INSTANCE *instances;
} spawn_globals = {
    .spinlock = NETDATA_SPINLOCK_INITIALIZER,
    .instances = NULL,
};

static void sigchld_handler(int signum) {
    (void) signum; // Unused parameter

    pid_t pid;
    int status;

    while ((pid = waitpid(-1, &status, WNOHANG)) > 0) {
        // Find the SPAWN_INSTANCE corresponding to this pid
        spinlock_lock(&spawn_globals.spinlock);
        for(SPAWN_INSTANCE *si = spawn_globals.instances; si ;si = si->next) {
            if (si->child_pid == pid) {
                si->exit_code = status;

                nd_log(NDLS_COLLECTORS, NDLP_ERR,
                       "SPAWN SERVER: process with pid %d exited with status %d",
                       si->child_pid, (int)status);

                uv_sem_post(&si->sem);
                break;
            }
        }
        spinlock_unlock(&spawn_globals.spinlock);
    }
}

SPAWN_SERVER* spawn_server_create(SPAWN_SERVER_OPTIONS options __maybe_unused, const char *name, spawn_request_callback_t cb  __maybe_unused, int argc __maybe_unused, const char **argv __maybe_unused) {
    SPAWN_SERVER* server = callocz(1, sizeof(SPAWN_SERVER));

    if (name)
        server->name = strdupz(name);
    else
        server->name = strdupz("unnamed");

    if(!spawn_globals.sigchld_initialized) {
        spawn_globals.sigchld_initialized = true;

        struct sigaction sa;
        sa.sa_handler = sigchld_handler;
        sigemptyset(&sa.sa_mask);
        sa.sa_flags = SA_RESTART | SA_NOCLDSTOP;
        if (sigaction(SIGCHLD, &sa, NULL) == -1) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: Failed to set SIGCHLD handler");
            freez((void *)server->name);
            freez(server);
            return NULL;
        }
    }

    return server;
}

void spawn_server_destroy(SPAWN_SERVER *server) {
    if (!server) return;
    freez((void *)server->name);
    freez(server);
}

SPAWN_INSTANCE* spawn_server_exec(SPAWN_SERVER *server, int stderr_fd, int custom_fd __maybe_unused, const char **argv, const void *data __maybe_unused, size_t data_size __maybe_unused, SPAWN_INSTANCE_TYPE type) {
    if (type != SPAWN_INSTANCE_TYPE_EXEC)
        return NULL;

    SPAWN_INSTANCE *si = callocz(1, sizeof(SPAWN_INSTANCE));
    si->child_pid = -1;
    si->exit_code = -1;
    si->request_id = __atomic_add_fetch(&server->request_id, 1, __ATOMIC_RELAXED);

    int stdin_pipe[2] = { -1, -1 };
    int stdout_pipe[2] = { -1, -1 };

    if (uv_sem_init(&si->sem, 0)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: uv_sem_init() failed");
        freez(si);
        return NULL;
    }

    if (pipe(stdin_pipe) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: stdin pipe() failed");
        uv_sem_destroy(&si->sem);
        freez(si);
        return NULL;
    }

    if (pipe(stdout_pipe) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: stdout pipe() failed");
        close(stdin_pipe[PIPE_READ]);
        close(stdin_pipe[PIPE_WRITE]);
        uv_sem_destroy(&si->sem);
        freez(si);
        return NULL;
    }

    posix_spawn_file_actions_t file_actions;
    posix_spawnattr_t attr;

    if (posix_spawn_file_actions_init(&file_actions) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: posix_spawn_file_actions_init() failed");
        close(stdin_pipe[PIPE_READ]);
        close(stdin_pipe[PIPE_WRITE]);
        close(stdout_pipe[PIPE_READ]);
        close(stdout_pipe[PIPE_WRITE]);
        uv_sem_destroy(&si->sem);
        freez(si);
        return NULL;
    }

    posix_spawn_file_actions_adddup2(&file_actions, stdin_pipe[PIPE_READ], STDIN_FILENO);
    posix_spawn_file_actions_adddup2(&file_actions, stdout_pipe[PIPE_WRITE], STDOUT_FILENO);
    posix_spawn_file_actions_adddup2(&file_actions, stderr_fd, STDERR_FILENO);
    posix_spawn_file_actions_addclose(&file_actions, stdin_pipe[PIPE_READ]);
    posix_spawn_file_actions_addclose(&file_actions, stdout_pipe[PIPE_WRITE]);

    if (posix_spawnattr_init(&attr) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: posix_spawnattr_init() failed");
        posix_spawn_file_actions_destroy(&file_actions);
        close(stdin_pipe[PIPE_READ]);
        close(stdin_pipe[PIPE_WRITE]);
        close(stdout_pipe[PIPE_READ]);
        close(stdout_pipe[PIPE_WRITE]);
        uv_sem_destroy(&si->sem);
        freez(si);
        return NULL;
    }

    spinlock_lock(&spawn_globals.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(spawn_globals.instances, si, prev, next);
    spinlock_unlock(&spawn_globals.spinlock);

    errno_clear();
    if (posix_spawn(&si->child_pid, argv[0], &file_actions, &attr, (char * const *)argv, environ) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: posix_spawn() failed");
        posix_spawnattr_destroy(&attr);
        posix_spawn_file_actions_destroy(&file_actions);
        close(stdin_pipe[PIPE_READ]);
        close(stdin_pipe[PIPE_WRITE]);
        close(stdout_pipe[PIPE_READ]);
        close(stdout_pipe[PIPE_WRITE]);
        uv_sem_destroy(&si->sem);
        freez(si);
        return NULL;
    }

    // Destroy the posix_spawnattr_t and posix_spawn_file_actions_t structures
    posix_spawnattr_destroy(&attr);
    posix_spawn_file_actions_destroy(&file_actions);

    si->write_fd = stdin_pipe[PIPE_WRITE];
    si->read_fd = stdout_pipe[PIPE_READ];

    nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: process created with pid %d", si->child_pid);
    return si;
}

int spawn_server_exec_kill(SPAWN_SERVER *server, SPAWN_INSTANCE *si) {
    if (!si) return -1;

    if (kill(si->child_pid, SIGTERM))
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: kill() failed");

    return spawn_server_exec_wait(server, si);
}

int spawn_server_exec_wait(SPAWN_SERVER *server __maybe_unused, SPAWN_INSTANCE *si) {
    if (!si) return -1;

    // Close all pipe descriptors to force the child to exit
    if (si->read_fd != -1) close(si->read_fd);
    if (si->write_fd != -1) close(si->write_fd);

    // Wait for the process to exit
    uv_sem_wait(&si->sem);
    int exit_code = si->exit_code;

    spinlock_lock(&spawn_globals.spinlock);
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(spawn_globals.instances, si, prev, next);
    spinlock_unlock(&spawn_globals.spinlock);

    uv_sem_destroy(&si->sem);
    freez(si);
    return exit_code;
}

#endif
