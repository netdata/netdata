// SPDX-License-Identifier: GPL-3.0-or-later

#include "spawn_server_internals.h"

#if defined(SPAWN_SERVER_VERSION_POSIX_SPAWN)

#define SPAWN_SERVER_MAX_DEFERRED_REAPERS 1

int spawn_server_instance_read_fd(SPAWN_INSTANCE *si) { return si->read_fd; }
int spawn_server_instance_write_fd(SPAWN_INSTANCE *si) { return si->write_fd; }
void spawn_server_instance_read_fd_unset(SPAWN_INSTANCE *si) { si->read_fd = -1; }
void spawn_server_instance_write_fd_unset(SPAWN_INSTANCE *si) { si->write_fd = -1; }
pid_t spawn_server_instance_pid(SPAWN_INSTANCE *si) { return si->child_pid; }

static struct {
    bool sigchld_initialized;
    SPINLOCK spinlock;
    SPAWN_INSTANCE *instances;
    uint32_t deferred_reapers;
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

SPAWN_SERVER *spawn_server_create(SPAWN_SERVER_OPTIONS options __maybe_unused, const char *name,
                                  spawn_request_callback_t cb __maybe_unused, int argc __maybe_unused,
                                  const char **argv __maybe_unused, ND_ENVIRONMENT *environment,
                                  const char *runtime_directory __maybe_unused) {
    if(!environment)
        return NULL;

    SPAWN_SERVER* server = callocz(1, sizeof(SPAWN_SERVER));
    server->environment = environment;

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
    posix_spawn_file_actions_t file_actions;
    posix_spawnattr_t attr;
    ND_ENV_SNAPSHOT *snapshot = NULL;
    bool file_actions_initialized = false;
    bool attr_initialized = false;
    bool listed = false;

    if (pipe(stdin_pipe) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: stdin pipe() failed: %s", cmdline);
        goto cleanup;
    }

    if (pipe(stdout_pipe) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: stdout pipe() failed: %s", cmdline);
        goto cleanup;
    }

    if (posix_spawn_file_actions_init(&file_actions) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: posix_spawn_file_actions_init() failed: %s", cmdline);
        goto cleanup;
    }
    file_actions_initialized = true;

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
        goto cleanup;
    }
    attr_initialized = true;

    // Set the flags to reset the signal mask and signal actions
    sigset_t empty_mask;
    sigemptyset(&empty_mask);
    if (posix_spawnattr_setsigmask(&attr, &empty_mask) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: posix_spawnattr_setsigmask() failed: %s", cmdline);
        goto cleanup;
    }

    short flags = POSIX_SPAWN_SETSIGMASK | POSIX_SPAWN_SETSIGDEF;
    if (posix_spawnattr_setflags(&attr, flags) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: posix_spawnattr_setflags() failed: %s", cmdline);
        goto cleanup;
    }

    snapshot = nd_environment_snapshot_acquire(server->environment);
    if(!snapshot)
        goto cleanup;

    spinlock_lock(&spawn_globals.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(spawn_globals.instances, si, prev, next);
    spinlock_unlock(&spawn_globals.spinlock);
    listed = true;

    // unfortunately, on CYGWIN/MSYS posix_spawn() is not thread safe
    // so, we run it one by one.
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    spinlock_lock(&spinlock);

    int fds[3] = { stdin_pipe[PIPE_READ], stdout_pipe[PIPE_WRITE], stderr_fd };
    os_close_all_non_std_open_fds_except(fds, 3, CLOSE_RANGE_CLOEXEC);

    errno_clear();
    int spawn_rc = posix_spawn(&si->child_pid, argv[0], &file_actions, &attr, (char *const *)argv,
                               (char *const *)nd_environment_snapshot_envp(snapshot));
    nd_environment_snapshot_release(snapshot);
    snapshot = NULL;
    if (spawn_rc != 0) {
        spinlock_unlock(&spinlock);
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: posix_spawn() failed: %s", cmdline);
        goto cleanup;
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

cleanup:
    nd_environment_snapshot_release(snapshot);

    if(listed) {
        spinlock_lock(&spawn_globals.spinlock);
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(spawn_globals.instances, si, prev, next);
        spinlock_unlock(&spawn_globals.spinlock);
    }

    if(attr_initialized)
        posix_spawnattr_destroy(&attr);
    if(file_actions_initialized)
        posix_spawn_file_actions_destroy(&file_actions);

    if(stdin_pipe[PIPE_READ] != -1) close(stdin_pipe[PIPE_READ]);
    if(stdin_pipe[PIPE_WRITE] != -1) close(stdin_pipe[PIPE_WRITE]);
    if(stdout_pipe[PIPE_READ] != -1) close(stdout_pipe[PIPE_READ]);
    if(stdout_pipe[PIPE_WRITE] != -1) close(stdout_pipe[PIPE_WRITE]);
    freez(si);
    return NULL;
}

static int spawn_server_waitpid(SPAWN_INSTANCE *si);

static void spawn_server_instance_cleanup(SPAWN_INSTANCE *si) {
    spinlock_lock(&spawn_globals.spinlock);
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(spawn_globals.instances, si, prev, next);
    spinlock_unlock(&spawn_globals.spinlock);

    freez((void *)si->cmdline);
    freez(si);
}

static void *spawn_server_detached_waitpid_cleanup(void *ptr) {
    SPAWN_INSTANCE *si = ptr;

    (void)spawn_server_waitpid(si);
    spawn_server_instance_cleanup(si);
    __atomic_sub_fetch(&spawn_globals.deferred_reapers, 1, __ATOMIC_RELAXED);

    return NULL;
}

static bool spawn_server_defer_waitpid_cleanup(SPAWN_INSTANCE *si) {
    pthread_t thread;
    pthread_attr_t attr;
    pid_t child_pid = si->child_pid;
    size_t request_id = si->request_id;

    if(__atomic_add_fetch(&spawn_globals.deferred_reapers, 1, __ATOMIC_RELAXED) >
        SPAWN_SERVER_MAX_DEFERRED_REAPERS) {
        __atomic_sub_fetch(&spawn_globals.deferred_reapers, 1, __ATOMIC_RELAXED);
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: deferred reaper limit reached for pid %d (request No %zu): %s",
               child_pid, request_id, si->cmdline);
        return false;
    }

    int rc = pthread_attr_init(&attr);
    bool attr_initialized = rc == 0;
    if(attr_initialized)
        rc = pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_DETACHED);

    if(rc == 0)
        rc = pthread_create(&thread, &attr, spawn_server_detached_waitpid_cleanup, si);

    if(attr_initialized && pthread_attr_destroy(&attr) != 0 && rc == 0)
        nd_log(NDLS_COLLECTORS, NDLP_WARNING,
               "SPAWN PARENT: failed to destroy detached reaper attributes for pid %d (request No %zu)",
               child_pid, request_id);

    if(rc != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: failed to start detached reaper for pid %d: %s",
               si->child_pid, si->cmdline);
        __atomic_sub_fetch(&spawn_globals.deferred_reapers, 1, __ATOMIC_RELAXED);
        return false;
    }

    return true;
}

int spawn_server_exec_kill(SPAWN_SERVER *server, SPAWN_INSTANCE *si, int timeout_ms) {
    if (!si) return -1;

    if (kill(si->child_pid, SIGTERM))
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: kill() of pid %d failed: %s",
               si->child_pid, si->cmdline);

    // escalate to SIGKILL if the child does not exit promptly after SIGTERM (or if the wait could
    // not be completed), so a SIGTERM-ignoring child cannot make the final wait block forever.
    // the caller's timeout_ms is the SIGTERM grace; fall back to a default when not specified.
    int grace_ms = timeout_ms > 0 ? timeout_ms : SPAWN_KILL_DEFAULT_GRACE_MS;
    int status;
    if(spawn_server_exec_timedwait(server, si, grace_ms, &status) != SPAWN_TIMEDWAIT_EXITED) {
        if(kill(si->child_pid, SIGKILL) != 0)
            // a failed SIGKILL almost always means the child is already gone (ESRCH); the wait
            // below then returns immediately. SIGKILL is uncatchable, so it cannot be ignored by
            // a live child - the only unbounded case left is uninterruptible (D-state) sleep,
            // which no signal or timeout can resolve.
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN PARENT: SIGKILL of pid %d failed: %s", si->child_pid, si->cmdline);
    }
    else
        return status;

    if(spawn_server_exec_timedwait(server, si, SPAWN_KILL_DEFAULT_GRACE_MS, &status) == SPAWN_TIMEDWAIT_EXITED)
        return status;

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "SPAWN PARENT: giving up waiting for pid %d after SIGKILL (request No %zu) - cleanup deferred",
           si->child_pid, si->request_id);

    if(spawn_server_defer_waitpid_cleanup(si))
        return -1;

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

SPAWN_TIMEDWAIT_RESULT spawn_server_exec_timedwait(SPAWN_SERVER *server, SPAWN_INSTANCE *si, int timeout_ms, int *status) {
    if (!si) { if(status) *status = -1; return SPAWN_TIMEDWAIT_EXITED; }

    // close the child pipes to force it to exit, matching spawn_server_exec_wait and the
    // other backends; otherwise a child blocked on stdin/stdout would never see EOF and
    // would stay alive until the deadline forces a SIGKILL
    if (si->read_fd != -1) { close(si->read_fd); si->read_fd = -1; }
    if (si->write_fd != -1) { close(si->write_fd); si->write_fd = -1; }

    // a negative timeout would become a huge usec_t deadline (= unbounded wait); clamp to poll-once
    if(timeout_ms < 0) timeout_ms = 0;
    usec_t deadline_ut = now_monotonic_usec() + (usec_t)timeout_ms * USEC_PER_MS;

    while(!__atomic_load_n(&si->exited, __ATOMIC_RELAXED)) {
        int wstatus = 0;
        pid_t pid = waitpid(si->child_pid, &wstatus, WNOHANG);
        if(pid == si->child_pid) {
            __atomic_store_n(&si->waitpid_status, wstatus, __ATOMIC_RELAXED);
            __atomic_store_n(&si->exited, true, __ATOMIC_RELAXED);
            break;
        }

        if(pid < 0 && errno != EINTR)
            // child reaped elsewhere (e.g. ECHILD) - let the blocking wait resolve it immediately
            break;

        // pid == 0 (still running) or EINTR (interrupted before any state change):
        // keep waiting, but never past the deadline
        if(now_monotonic_usec() >= deadline_ut)
            return SPAWN_TIMEDWAIT_RUNNING;

        sleep_usec(10 * USEC_PER_MS);
    }

    int st = spawn_server_exec_wait(server, si);
    if(status) *status = st;
    return SPAWN_TIMEDWAIT_EXITED;
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

    spawn_server_instance_cleanup(si);
    return status;
}

#endif
