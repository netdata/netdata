// SPDX-License-Identifier: GPL-3.0-or-later

#include "spawn_server_internals.h"

#if defined(SPAWN_SERVER_VERSION_UV)

int spawn_server_instance_read_fd(SPAWN_INSTANCE *si) { return si->read_fd; }
int spawn_server_instance_write_fd(SPAWN_INSTANCE *si) { return si->write_fd; }
void spawn_server_instance_read_fd_unset(SPAWN_INSTANCE *si) { si->read_fd = -1; }
void spawn_server_instance_write_fd_unset(SPAWN_INSTANCE *si) { si->write_fd = -1; }
pid_t spawn_server_instance_pid(SPAWN_INSTANCE *si) { return uv_process_get_pid(&si->process); }

typedef struct work_item {
    int stderr_fd;
    const char **argv;
    ND_ENV_SNAPSHOT *environment;
    uv_sem_t sem;
    SPAWN_INSTANCE *instance;
    struct work_item *prev;
    struct work_item *next;
} work_item;

int uv_errno_to_errno(int uv_err) {
    switch (uv_err) {
        case 0: return 0;
        case UV_E2BIG: return E2BIG;
        case UV_EACCES: return EACCES;
        case UV_EADDRINUSE: return EADDRINUSE;
        case UV_EADDRNOTAVAIL: return EADDRNOTAVAIL;
        case UV_EAFNOSUPPORT: return EAFNOSUPPORT;
        case UV_EAGAIN: return EAGAIN;
        case UV_EAI_ADDRFAMILY: return EAI_ADDRFAMILY;
        case UV_EAI_AGAIN: return EAI_AGAIN;
        case UV_EAI_BADFLAGS: return EAI_BADFLAGS;
#if defined(EAI_CANCELED)
        case UV_EAI_CANCELED: return EAI_CANCELED;
#endif
        case UV_EAI_FAIL: return EAI_FAIL;
        case UV_EAI_FAMILY: return EAI_FAMILY;
        case UV_EAI_MEMORY: return EAI_MEMORY;
        case UV_EAI_NODATA: return EAI_NODATA;
        case UV_EAI_NONAME: return EAI_NONAME;
        case UV_EAI_OVERFLOW: return EAI_OVERFLOW;
        case UV_EAI_SERVICE: return EAI_SERVICE;
        case UV_EAI_SOCKTYPE: return EAI_SOCKTYPE;
        case UV_EALREADY: return EALREADY;
        case UV_EBADF: return EBADF;
        case UV_EBUSY: return EBUSY;
        case UV_ECANCELED: return ECANCELED;
        case UV_ECHARSET: return EILSEQ;  // No direct mapping, using EILSEQ
        case UV_ECONNABORTED: return ECONNABORTED;
        case UV_ECONNREFUSED: return ECONNREFUSED;
        case UV_ECONNRESET: return ECONNRESET;
        case UV_EDESTADDRREQ: return EDESTADDRREQ;
        case UV_EEXIST: return EEXIST;
        case UV_EFAULT: return EFAULT;
        case UV_EFBIG: return EFBIG;
        case UV_EHOSTUNREACH: return EHOSTUNREACH;
        case UV_EINTR: return EINTR;
        case UV_EINVAL: return EINVAL;
        case UV_EIO: return EIO;
        case UV_EISCONN: return EISCONN;
        case UV_EISDIR: return EISDIR;
        case UV_ELOOP: return ELOOP;
        case UV_EMFILE: return EMFILE;
        case UV_EMSGSIZE: return EMSGSIZE;
        case UV_ENAMETOOLONG: return ENAMETOOLONG;
        case UV_ENETDOWN: return ENETDOWN;
        case UV_ENETUNREACH: return ENETUNREACH;
        case UV_ENFILE: return ENFILE;
        case UV_ENOBUFS: return ENOBUFS;
        case UV_ENODEV: return ENODEV;
        case UV_ENOENT: return ENOENT;
        case UV_ENOMEM: return ENOMEM;
        case UV_ENONET: return ENONET;
        case UV_ENOSPC: return ENOSPC;
        case UV_ENOSYS: return ENOSYS;
        case UV_ENOTCONN: return ENOTCONN;
        case UV_ENOTDIR: return ENOTDIR;
        case UV_ENOTEMPTY: return ENOTEMPTY;
        case UV_ENOTSOCK: return ENOTSOCK;
        case UV_ENOTSUP: return ENOTSUP;
        case UV_ENOTTY: return ENOTTY;
        case UV_ENXIO: return ENXIO;
        case UV_EPERM: return EPERM;
        case UV_EPIPE: return EPIPE;
        case UV_EPROTO: return EPROTO;
        case UV_EPROTONOSUPPORT: return EPROTONOSUPPORT;
        case UV_EPROTOTYPE: return EPROTOTYPE;
        case UV_ERANGE: return ERANGE;
        case UV_EROFS: return EROFS;
        case UV_ESHUTDOWN: return ESHUTDOWN;
        case UV_ESPIPE: return ESPIPE;
        case UV_ESRCH: return ESRCH;
        case UV_ETIMEDOUT: return ETIMEDOUT;
        case UV_ETXTBSY: return ETXTBSY;
        case UV_EXDEV: return EXDEV;
        default: return EINVAL; // Use EINVAL for unknown libuv errors
    }
}

static void server_thread(void *arg) {
    SPAWN_SERVER *server = (SPAWN_SERVER *)arg;
    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "SPAWN SERVER: started");

    // this thread needs to process SIGCHLD (by libuv)
    // otherwise the on_exit() callback is never run
    signals_unblock_one(SIGCHLD);

    // run the event loop
    uv_run(server->loop, UV_RUN_DEFAULT);

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "SPAWN SERVER: ended");
}

static void on_process_closed(uv_handle_t *handle) {
    SPAWN_INSTANCE *si = (SPAWN_INSTANCE *)handle->data;
    if(si) {
        uv_sem_post(&si->sem);
        __atomic_store_n(&si->process_close_completed, true, __ATOMIC_RELEASE);
    }
}

static void wait_for_process_close_callback(SPAWN_INSTANCE *si) {
    while(!__atomic_load_n(&si->process_close_completed, __ATOMIC_ACQUIRE))
        uv_sleep(0);
}

typedef struct inherited_fd_state {
    int fd;
    int original_flags;
    bool changed;
} inherited_fd_state;

static bool inherited_fd_prepare_for_dup(inherited_fd_state *state, int inherited_fd, int std_fd) {
    state->fd = inherited_fd;
    if(inherited_fd == std_fd)
        return true;

    int flags = fcntl(inherited_fd, F_GETFD);
    if(flags == -1)
        return false;

    state->original_flags = flags;
    if(flags & FD_CLOEXEC)
        return true;

    if(fcntl(inherited_fd, F_SETFD, flags | FD_CLOEXEC) == -1)
        return false;

    state->changed = true;
    return true;
}

static void inherited_fd_restore(const inherited_fd_state *state) {
    if(state->changed && fcntl(state->fd, F_SETFD, state->original_flags) == -1)
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: cannot restore inherited descriptor flags");
}

static void on_process_exit(uv_process_t *req, int64_t exit_status, int term_signal) {
    SPAWN_INSTANCE *si = (SPAWN_INSTANCE *)req->data;
    SPAWN_SERVER *server = si->server;
    si->exit_code = (int)(term_signal ? term_signal : exit_status << 8);
    uv_close((uv_handle_t *)req, on_process_closed);

    if(server->live_processes > 0)
        server->live_processes--;
    else
        internal_fatal(true, "SPAWN SERVER: live process accounting underflow");
    if(__atomic_load_n(&server->stopping, __ATOMIC_RELAXED) &&
       server->live_processes == 0 && server->shutdown_timer_initialized &&
       !uv_is_closing((uv_handle_t *)&server->shutdown_timer)) {
        uv_timer_stop(&server->shutdown_timer);
        uv_close((uv_handle_t *)&server->shutdown_timer, NULL);
    }

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "SPAWN SERVER: process with pid %d exited with code %d and term_signal %d",
           si->child_pid, (int)exit_status, term_signal);

}

static SPAWN_INSTANCE *spawn_process_with_libuv(SPAWN_SERVER *server, int stderr_fd, const char **argv,
                                                const ND_ENV_SNAPSHOT *environment) {
    SPAWN_INSTANCE *si = NULL;
    bool si_sem_init = false;

    int stdin_pipe[2] = { -1, -1 };
    int stdout_pipe[2] = { -1, -1 };

    if (pipe(stdin_pipe) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: stdin pipe() failed");
        goto cleanup;
    }

    if (pipe(stdout_pipe) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: stdout pipe() failed");
        goto cleanup;
    }

    si = callocz(1, sizeof(SPAWN_INSTANCE));
    si->exit_code = -1;

    if (uv_sem_init(&si->sem, 0)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: uv_sem_init() failed");
        goto cleanup;
    }
    si_sem_init = true;

    uv_stdio_container_t stdio[3] = { 0 };
    stdio[0].flags = UV_INHERIT_FD;
    stdio[0].data.fd = stdin_pipe[PIPE_READ];
    stdio[1].flags = UV_INHERIT_FD;
    stdio[1].data.fd = stdout_pipe[PIPE_WRITE];
    stdio[2].flags = UV_INHERIT_FD;
    stdio[2].data.fd = stderr_fd;

    inherited_fd_state inherited_fds[3] = { 0 };
    bool inherited_fds_prepared =
        inherited_fd_prepare_for_dup(&inherited_fds[0], stdio[0].data.fd, STDIN_FILENO) &&
        inherited_fd_prepare_for_dup(&inherited_fds[1], stdio[1].data.fd, STDOUT_FILENO) &&
        inherited_fd_prepare_for_dup(&inherited_fds[2], stdio[2].data.fd, STDERR_FILENO);
    if(!inherited_fds_prepared) {
        for(size_t i = 0; i < _countof(inherited_fds); i++)
            inherited_fd_restore(&inherited_fds[i]);

        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: cannot mark inherited descriptor close-on-exec");
        goto cleanup;
    }

    uv_process_options_t options = { 0 };
    options.stdio_count = 3;
    options.stdio = stdio;
    options.exit_cb = on_process_exit;
    options.file = argv[0];
    options.args = (char **)argv;
    options.env = (char **)nd_environment_snapshot_envp(environment);

    // uv_spawn() does not close all other open file descriptors
    // we have to close them manually
    int fds[3] = { stdio[0].data.fd, stdio[1].data.fd, stdio[2].data.fd };
    os_close_all_non_std_open_fds_except(fds, 3, CLOSE_RANGE_CLOEXEC);

    int rc = uv_spawn(server->loop, &si->process, &options);
    for(size_t i = 0; i < _countof(inherited_fds); i++)
        inherited_fd_restore(&inherited_fds[i]);

    if (rc) {
        errno = uv_errno_to_errno(rc);
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: uv_spawn() failed with error %s, %s",
               uv_err_name(rc), uv_strerror(rc));
        goto cleanup;
    }

    // Successfully spawned
    si->server = server;
    server->live_processes++;

    // get the pid of the process spawned
    si->child_pid = uv_process_get_pid(&si->process);

    // on_process_exit() needs this to find the si
    si->process.data = si;

    nd_log(NDLS_COLLECTORS, NDLP_INFO,
           "SPAWN SERVER: process created with pid %d", si->child_pid);

    // close the child sides of the pipes
    close(stdin_pipe[PIPE_READ]);
    si->write_fd = stdin_pipe[PIPE_WRITE];
    si->read_fd = stdout_pipe[PIPE_READ];
    close(stdout_pipe[PIPE_WRITE]);

    return si;

cleanup:
    if(stdin_pipe[PIPE_READ] != -1) close(stdin_pipe[PIPE_READ]);
    if(stdin_pipe[PIPE_WRITE] != -1) close(stdin_pipe[PIPE_WRITE]);
    if(stdout_pipe[PIPE_READ] != -1) close(stdout_pipe[PIPE_READ]);
    if(stdout_pipe[PIPE_WRITE] != -1) close(stdout_pipe[PIPE_WRITE]);
    if(si) {
        if(si_sem_init)
            uv_sem_destroy(&si->sem);

        freez(si);
    }
    return NULL;
}

static void signal_live_process(uv_handle_t *handle, void *arg) {
    if(handle->type != UV_PROCESS || uv_is_closing(handle))
        return;

    int signal = *(int *)arg;
    int rc = uv_process_kill((uv_process_t *)handle, signal);
    if(rc != 0 && rc != UV_ESRCH)
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: cannot send signal %d to process %d: %s",
               signal, uv_process_get_pid((uv_process_t *)handle), uv_strerror(rc));
}

static void shutdown_timer_callback(uv_timer_t *timer) {
    SPAWN_SERVER *server = timer->data;
    int signal = SIGKILL;
    uv_walk(server->loop, signal_live_process, &signal);
    uv_close((uv_handle_t *)timer, NULL);
}

static void begin_shutdown(SPAWN_SERVER *server) {
    if(!uv_is_closing((uv_handle_t *)&server->async))
        uv_close((uv_handle_t *)&server->async, NULL);

    if(server->live_processes == 0)
        return;

    int signal = SIGTERM;
    uv_walk(server->loop, signal_live_process, &signal);

    int rc = uv_timer_init(server->loop, &server->shutdown_timer);
    if(rc == 0) {
        server->shutdown_timer_initialized = true;
        server->shutdown_timer.data = server;
        rc = uv_timer_start(
            &server->shutdown_timer, shutdown_timer_callback, SPAWN_KILL_DEFAULT_GRACE_MS, 0);
    }

    if(rc != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: cannot start the shutdown grace timer: %s; sending SIGKILL now",
               uv_strerror(rc));
        signal = SIGKILL;
        uv_walk(server->loop, signal_live_process, &signal);
        if(server->shutdown_timer_initialized &&
           !uv_is_closing((uv_handle_t *)&server->shutdown_timer))
            uv_close((uv_handle_t *)&server->shutdown_timer, NULL);
    }
}

static void async_callback(uv_async_t *handle) {
    nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN SERVER: dequeue commands started");
    SPAWN_SERVER *server = (SPAWN_SERVER *)handle->data;

    bool stopping = __atomic_load_n(&server->stopping, __ATOMIC_RELAXED);

    work_item *item;
    spinlock_lock(&server->spinlock);
    while (server->work_queue) {
        item = server->work_queue;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(server->work_queue, item, prev, next);
        spinlock_unlock(&server->spinlock);

        if(stopping)
            item->instance = NULL;
        else
            item->instance = spawn_process_with_libuv(server, item->stderr_fd, item->argv, item->environment);

        ND_ENV_SNAPSHOT *environment = item->environment;
        item->environment = NULL;
        nd_environment_snapshot_release(environment);
        uv_sem_post(&item->sem);

        spinlock_lock(&server->spinlock);
    }
    spinlock_unlock(&server->spinlock);

    if(stopping) {
        nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN SERVER: stopping...");
        begin_shutdown(server);
    }

    nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN SERVER: dequeue commands done");
}


SPAWN_SERVER *spawn_server_create(SPAWN_SERVER_OPTIONS options __maybe_unused, const char *name,
                                  spawn_request_callback_t cb __maybe_unused, int argc __maybe_unused,
                                  const char **argv __maybe_unused, ND_ENVIRONMENT *environment,
                                  const char *runtime_directory __maybe_unused) {
    if(!nd_environment_is_process_frozen()) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: refusing to start the libuv server before the environment freeze");
        return NULL;
    }

    if(!environment)
        return NULL;

    SPAWN_SERVER* server = callocz(1, sizeof(SPAWN_SERVER));
    server->environment = environment;
    spinlock_init(&server->spinlock);

    if (name)
        server->name = strdupz(name);
    else
        server->name = strdupz("unnamed");

    server->loop = callocz(1, sizeof(uv_loop_t));
    if (uv_loop_init(server->loop)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: uv_loop_init() failed");
        freez(server->loop);
        freez((void *)server->name);
        freez(server);
        return NULL;
    }

    if (uv_async_init(server->loop, &server->async, async_callback)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: uv_async_init() failed");
        int rc = uv_loop_close(server->loop);
        if(rc != 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN PARENT: uv_loop_close() failed after async initialization failure: %s",
                   uv_strerror(rc));
            return NULL;
        }
        freez(server->loop);
        freez((void *)server->name);
        freez(server);
        return NULL;
    }
    server->async.data = server;

    if (uv_thread_create(&server->thread, server_thread, server)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: uv_thread_create() failed");
        uv_close((uv_handle_t*)&server->async, NULL);
        uv_run(server->loop, UV_RUN_DEFAULT);
        int rc = uv_loop_close(server->loop);
        if(rc != 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN PARENT: uv_loop_close() failed after thread creation failure: %s",
                   uv_strerror(rc));
            return NULL;
        }
        freez(server->loop);
        freez((void *)server->name);
        freez(server);
        return NULL;
    }

    return server;
}

static void close_handle(uv_handle_t* handle, void* arg __maybe_unused) {
    if(!uv_is_closing(handle) && handle->type != UV_PROCESS)
        uv_close(handle, NULL);
}

void spawn_server_destroy(SPAWN_SERVER *server) {
    if (!server) return;

    __atomic_store_n(&server->stopping, true, __ATOMIC_RELAXED);

    // Trigger the async callback to stop the event loop
    uv_async_send(&server->async);

    // Wait for the server thread to finish
    uv_thread_join(&server->thread);

    // Close only non-process stragglers. Live process handles must reach on_process_exit()
    // so libuv can reap the child before the handle is closed.
    uv_walk(server->loop, close_handle, NULL);
    uv_run(server->loop, UV_RUN_DEFAULT);

    int rc = uv_loop_close(server->loop);
    if(rc != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: uv_loop_close() failed during shutdown: %s",
               uv_strerror(rc));
        return;
    }

    freez(server->loop);
    freez((void *)server->name);
    freez(server);
}

SPAWN_INSTANCE* spawn_server_exec(SPAWN_SERVER *server, int stderr_fd __maybe_unused, int custom_fd __maybe_unused, const char **argv, const void *data __maybe_unused, size_t data_size __maybe_unused, SPAWN_INSTANCE_TYPE type) {
    if (type != SPAWN_INSTANCE_TYPE_EXEC)
        return NULL;

    work_item item = { 0 };
    item.stderr_fd = stderr_fd;
    item.argv = argv;

    if (uv_sem_init(&item.sem, 0)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: uv_sem_init() failed");
        return NULL;
    }

    item.environment = nd_environment_snapshot_acquire(server->environment);
    if(!item.environment) {
        uv_sem_destroy(&item.sem);
        return NULL;
    }

    spinlock_lock(&server->spinlock);
    // item is in the stack, but the server will remove it before sending to us
    // the semaphore, so it is safe to have the item in the stack.
    work_item *item_ptr = &item;
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(server->work_queue, item_ptr, prev, next);
    spinlock_unlock(&server->spinlock);

    uv_async_send(&server->async);

    nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN PARENT: queued command");

    // Wait for the command to be executed
    uv_sem_wait(&item.sem);
    uv_sem_destroy(&item.sem);

    if (!item.instance) {
        nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN PARENT: process failed to be started");
        return NULL;
    }

    nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN PARENT: process started");

    return item.instance;
}

int spawn_server_exec_kill(SPAWN_SERVER *server __maybe_unused, SPAWN_INSTANCE *si, int timeout_ms) {
    if(!si) return -1;

    // close all pipe descriptors to force the child to exit
    if(si->read_fd != -1) { close(si->read_fd); si->read_fd = -1; }
    if(si->write_fd != -1) { close(si->write_fd); si->write_fd = -1; }

    if (uv_process_kill(&si->process, SIGTERM)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: uv_process_kill() failed");
        return -1;
    }

    // escalate to SIGKILL if the child does not exit promptly after SIGTERM (or if the wait could
    // not be completed), so a SIGTERM-ignoring child cannot make the final wait block forever.
    // the caller's timeout_ms is the SIGTERM grace; fall back to a default when not specified.
    int grace_ms = timeout_ms > 0 ? timeout_ms : SPAWN_KILL_DEFAULT_GRACE_MS;
    int status;
    if(spawn_server_exec_timedwait(server, si, grace_ms, &status) != SPAWN_TIMEDWAIT_EXITED) {
        if(uv_process_kill(&si->process, SIGKILL))
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: uv_process_kill(SIGKILL) failed");
    }
    else
        return status;

    return spawn_server_exec_wait(server, si);
}

SPAWN_TIMEDWAIT_RESULT spawn_server_exec_timedwait(SPAWN_SERVER *server __maybe_unused, SPAWN_INSTANCE *si, int timeout_ms, int *status) {
    if (!si) { if(status) *status = -1; return SPAWN_TIMEDWAIT_EXITED; }

    // close all pipe descriptors to force the child to exit
    if(si->read_fd != -1) { close(si->read_fd); si->read_fd = -1; }
    if(si->write_fd != -1) { close(si->write_fd); si->write_fd = -1; }

    // a negative timeout would become a huge usec_t deadline (= unbounded wait); clamp to poll-once
    if(timeout_ms < 0) timeout_ms = 0;
    usec_t deadline_ut = now_monotonic_usec() + (usec_t)timeout_ms * USEC_PER_MS;

    while(uv_sem_trywait(&si->sem) != 0) {
        if(now_monotonic_usec() >= deadline_ut)
            return SPAWN_TIMEDWAIT_RUNNING;

        sleep_usec(10 * USEC_PER_MS);
    }

    // the semaphore is consumed - finish exactly like spawn_server_exec_wait()
    wait_for_process_close_callback(si);
    int st = si->exit_code;
    uv_sem_destroy(&si->sem);
    freez(si);
    if(status) *status = st;
    return SPAWN_TIMEDWAIT_EXITED;
}

int spawn_server_exec_wait(SPAWN_SERVER *server __maybe_unused, SPAWN_INSTANCE *si) {
    if (!si) return -1;

    // close all pipe descriptors to force the child to exit
    if(si->read_fd != -1) { close(si->read_fd); si->read_fd = -1; }
    if(si->write_fd != -1) { close(si->write_fd); si->write_fd = -1; }

    // Wait for the process to exit
    uv_sem_wait(&si->sem);
    wait_for_process_close_callback(si);
    int exit_code = si->exit_code;

    uv_sem_destroy(&si->sem);
    freez(si);
    return exit_code;
}

#endif
