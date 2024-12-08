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
    .spinlock = SPINLOCK_INITIALIZER,
    .instances = NULL,
};

//static void sigchld_handler(int signum __maybe_unused) {
//    pid_t pid;
//    int status;
//
//    while ((pid = waitpid(-1, &status, WNOHANG)) > 0) {
//        // Find the SPAWN_INSTANCE corresponding to this pid
//        spinlock_lock(&spawn_globals.spinlock);
//        for(SPAWN_INSTANCE *si = spawn_globals.instances; si ;si = si->next) {
//            if (si->child_pid == pid) {
//                __atomic_store_n(&si->waitpid_status, status, __ATOMIC_RELAXED);
//                __atomic_store_n(&si->exited, true, __ATOMIC_RELAXED);
//                break;
//            }
//        }
//        spinlock_unlock(&spawn_globals.spinlock);
//    }
//}

SPAWN_SERVER* spawn_server_create(SPAWN_SERVER_OPTIONS options __maybe_unused, const char *name, spawn_request_callback_t cb  __maybe_unused, int argc __maybe_unused, const char **argv __maybe_unused) {
    SPAWN_SERVER* server = callocz(1, sizeof(SPAWN_SERVER));

    if (name)
        server->name = strdupz(name);
    else
        server->name = strdupz("unnamed");

    if(!spawn_globals.sigchld_initialized) {
        spawn_globals.sigchld_initialized = true;

//        struct sigaction sa;
//        sa.sa_handler = sigchld_handler;
//        sigemptyset(&sa.sa_mask);
//        sa.sa_flags = SA_RESTART | SA_NOCLDSTOP;
//        if (sigaction(SIGCHLD, &sa, NULL) == -1) {
//            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: Failed to set SIGCHLD handler");
//            freez((void *)server->name);
//            freez(server);
//            return NULL;
//        }
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

    CLEAN_BUFFER *cmdline_wb = argv_to_cmdline_buffer(argv);
    const char *cmdline = buffer_tostring(cmdline_wb);

    SPAWN_INSTANCE *si = callocz(1, sizeof(SPAWN_INSTANCE));
    si->child_pid = -1;
    si->request_id = __atomic_add_fetch(&server->request_id, 1, __ATOMIC_RELAXED);

    int stdin_pipe[2] = { -1, -1 };
    int stdout_pipe[2] = { -1, -1 };

    if (pipe(stdin_pipe) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: stdin pipe() failed: %s", cmdline);
        freez(si);
        return NULL;
    }

    if (pipe(stdout_pipe) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: stdout pipe() failed: %s", cmdline);
        close(stdin_pipe[PIPE_READ]);
        close(stdin_pipe[PIPE_WRITE]);
        freez(si);
        return NULL;
    }

    posix_spawn_file_actions_t file_actions;
    posix_spawnattr_t attr;

    if (posix_spawn_file_actions_init(&file_actions) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: posix_spawn_file_actions_init() failed: %s", cmdline);
        close(stdin_pipe[PIPE_READ]);
        close(stdin_pipe[PIPE_WRITE]);
        close(stdout_pipe[PIPE_READ]);
        close(stdout_pipe[PIPE_WRITE]);
        freez(si);
        return NULL;
    }

    posix_spawn_file_actions_adddup2(&file_actions, stdin_pipe[PIPE_READ], STDIN_FILENO);
    posix_spawn_file_actions_adddup2(&file_actions, stdout_pipe[PIPE_WRITE], STDOUT_FILENO);
    posix_spawn_file_actions_addclose(&file_actions, stdin_pipe[PIPE_READ]);
    posix_spawn_file_actions_addclose(&file_actions, stdin_pipe[PIPE_WRITE]);
    posix_spawn_file_actions_addclose(&file_actions, stdout_pipe[PIPE_READ]);
    posix_spawn_file_actions_addclose(&file_actions, stdout_pipe[PIPE_WRITE]);
    if(stderr_fd != STDERR_FILENO) {
        posix_spawn_file_actions_adddup2(&file_actions, stderr_fd, STDERR_FILENO);
        posix_spawn_file_actions_addclose(&file_actions, stderr_fd);
    }

    if (posix_spawnattr_init(&attr) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: posix_spawnattr_init() failed: %s", cmdline);
        posix_spawn_file_actions_destroy(&file_actions);
        close(stdin_pipe[PIPE_READ]);
        close(stdin_pipe[PIPE_WRITE]);
        close(stdout_pipe[PIPE_READ]);
        close(stdout_pipe[PIPE_WRITE]);
        freez(si);
        return NULL;
    }

    // Set the flags to reset the signal mask and signal actions
    sigset_t empty_mask;
    sigemptyset(&empty_mask);
    if (posix_spawnattr_setsigmask(&attr, &empty_mask) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: posix_spawnattr_setsigmask() failed: %s", cmdline);
        posix_spawn_file_actions_destroy(&file_actions);
        posix_spawnattr_destroy(&attr);
        return false;
    }

    short flags = POSIX_SPAWN_SETSIGMASK | POSIX_SPAWN_SETSIGDEF;
    if (posix_spawnattr_setflags(&attr, flags) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: posix_spawnattr_setflags() failed: %s", cmdline);
        posix_spawn_file_actions_destroy(&file_actions);
        posix_spawnattr_destroy(&attr);
        return false;
    }

    spinlock_lock(&spawn_globals.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(spawn_globals.instances, si, prev, next);
    spinlock_unlock(&spawn_globals.spinlock);

    // unfortunately, on CYGWIN/MSYS posix_spawn() is not thread safe
    // so, we run it one by one.
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    spinlock_lock(&spinlock);

    int fds[3] = { stdin_pipe[PIPE_READ], stdout_pipe[PIPE_WRITE], stderr_fd };
    os_close_all_non_std_open_fds_except(fds, 3, CLOSE_RANGE_CLOEXEC);

    errno_clear();
    if (posix_spawn(&si->child_pid, argv[0], &file_actions, &attr, (char * const *)argv, environ) != 0) {
        spinlock_unlock(&spinlock);
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: posix_spawn() failed: %s", cmdline);

        spinlock_lock(&spawn_globals.spinlock);
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(spawn_globals.instances, si, prev, next);
        spinlock_unlock(&spawn_globals.spinlock);

        posix_spawnattr_destroy(&attr);
        posix_spawn_file_actions_destroy(&file_actions);

        close(stdin_pipe[PIPE_READ]);
        close(stdin_pipe[PIPE_WRITE]);
        close(stdout_pipe[PIPE_READ]);
        close(stdout_pipe[PIPE_WRITE]);
        freez(si);
        return NULL;
    }
    spinlock_unlock(&spinlock);

    // Destroy the posix_spawnattr_t and posix_spawn_file_actions_t structures
    posix_spawnattr_destroy(&attr);
    posix_spawn_file_actions_destroy(&file_actions);

    // Close the read end of the stdin pipe and the write end of the stdout pipe in the parent process
    close(stdin_pipe[PIPE_READ]);
    close(stdout_pipe[PIPE_WRITE]);

    si->write_fd = stdin_pipe[PIPE_WRITE];
    si->read_fd = stdout_pipe[PIPE_READ];
    si->cmdline = strdupz(cmdline);

    nd_log(NDLS_COLLECTORS, NDLP_INFO,
           "SPAWN SERVER: process created with pid %d: %s",
           si->child_pid, cmdline);
    return si;
}

int spawn_server_exec_kill(SPAWN_SERVER *server, SPAWN_INSTANCE *si, int timeout_ms __maybe_unused) {
    if (!si) return -1;

    if (kill(si->child_pid, SIGTERM))
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: kill() of pid %d failed: %s",
               si->child_pid, si->cmdline);

    return spawn_server_exec_wait(server, si);
}

static int spawn_server_waitpid(SPAWN_INSTANCE *si) {
    int status;
    pid_t pid;

    pid = waitpid(si->child_pid, &status, 0);

    if(pid != si->child_pid) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: failed to wait for pid %d: %s",
               si->child_pid, si->cmdline);

        return -1;
    }

    errno_clear();

    if(WIFEXITED(status)) {
        if(WEXITSTATUS(status))
            nd_log(NDLS_COLLECTORS, NDLP_INFO,
                   "SPAWN SERVER: child with pid %d (request %zu) exited with exit code %d: %s",
                   pid, si->request_id, WEXITSTATUS(status), si->cmdline);
    }
    else if(WIFSIGNALED(status)) {
        if(WCOREDUMP(status))
            nd_log(NDLS_COLLECTORS, NDLP_INFO,
                   "SPAWN SERVER: child with pid %d (request %zu) coredump'd due to signal %d: %s",
                   pid, si->request_id, WTERMSIG(status), si->cmdline);
        else
            nd_log(NDLS_COLLECTORS, NDLP_INFO,
                   "SPAWN SERVER: child with pid %d (request %zu) killed by signal %d: %s",
                   pid, si->request_id, WTERMSIG(status), si->cmdline);
    }
    else if(WIFSTOPPED(status)) {
        nd_log(NDLS_COLLECTORS, NDLP_INFO,
               "SPAWN SERVER: child with pid %d (request %zu) stopped due to signal %d: %s",
               pid, si->request_id, WSTOPSIG(status), si->cmdline);
    }
    else if(WIFCONTINUED(status)) {
        nd_log(NDLS_COLLECTORS, NDLP_INFO,
               "SPAWN SERVER: child with pid %d (request %zu) continued due to signal %d: %s",
               pid, si->request_id, SIGCONT, si->cmdline);
    }
    else {
        nd_log(NDLS_COLLECTORS, NDLP_INFO,
               "SPAWN SERVER: child with pid %d (request %zu) reports unhandled status: %s",
               pid, si->request_id, si->cmdline);
    }

    return status;
}

int spawn_server_exec_wait(SPAWN_SERVER *server __maybe_unused, SPAWN_INSTANCE *si) {
    if (!si) return -1;

    // Close all pipe descriptors to force the child to exit
    if (si->read_fd != -1) close(si->read_fd);
    if (si->write_fd != -1) close(si->write_fd);

    // Wait for the process to exit
    int status = __atomic_load_n(&si->waitpid_status, __ATOMIC_RELAXED);
    bool exited = __atomic_load_n(&si->exited, __ATOMIC_RELAXED);
    if(!exited)
        status = spawn_server_waitpid(si);
    else
        nd_log(NDLS_COLLECTORS, NDLP_INFO,
               "SPAWN PARENT: child with pid %d exited with status %d (sighandler): %s",
               si->child_pid, status, si->cmdline);

    spinlock_lock(&spawn_globals.spinlock);
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(spawn_globals.instances, si, prev, next);
    spinlock_unlock(&spawn_globals.spinlock);

    freez((void *)si->cmdline);
    freez(si);
    return status;
}

#endif
