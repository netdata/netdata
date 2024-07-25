// SPDX-License-Identifier: GPL-3.0-or-later

#include "spawn_server_internals.h"

#if defined(SPAWN_SERVER_VERSION_UV)

int spawn_server_instance_read_fd(SPAWN_INSTANCE *si) { return si->read_fd; }
int spawn_server_instance_write_fd(SPAWN_INSTANCE *si) { return si->write_fd; }
void spawn_server_instance_read_fd_unset(SPAWN_INSTANCE *si) { si->read_fd = -1; }
void spawn_server_instance_write_fd_unset(SPAWN_INSTANCE *si) { si->write_fd = -1; }
pid_t spawn_server_instance_pid(SPAWN_INSTANCE *si) { return uv_process_get_pid(&si->process); }

typedef struct work_item {
    SPAWN_SERVER *server;
    SPAWN_INSTANCE *instance;
    uv_process_options_t options;
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

static void posix_unmask_sigchld_on_thread(void) {
    sigset_t sigset;
    sigemptyset(&sigset);  // Initialize the signal set to empty
    sigaddset(&sigset, SIGCHLD);  // Add SIGCHLD to the set

    if(pthread_sigmask(SIG_UNBLOCK, &sigset, NULL) != 0)
        netdata_log_error("SPAWN SERVER: cannot unmask SIGCHLD");
}

static void server_thread(void *arg) {
    SPAWN_SERVER *server = (SPAWN_SERVER *)arg;
    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "SPAWN SERVER: started");

    // this thread needs to process SIGCHLD (by libuv)
    // otherwise the on_exit() callback is never run
    posix_unmask_sigchld_on_thread();

    // run the event loop
    uv_run(server->loop, UV_RUN_DEFAULT);

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "SPAWN SERVER: ended");
}

static void async_callback(uv_async_t *handle) {
    nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN SERVER: dequeue commands started");
    SPAWN_SERVER *server = (SPAWN_SERVER *)handle->data;

    // Check if the server is stopping
    if (__atomic_load_n(&server->stopping, __ATOMIC_RELAXED)) {
        nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN SERVER: stopping...");
        uv_stop(server->loop);
        return;
    }

    work_item *item;
    spinlock_lock(&server->spinlock);
    while (server->work_queue) {
        item = server->work_queue;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(server->work_queue, item, prev, next);
        spinlock_unlock(&server->spinlock);

        SPAWN_INSTANCE *si = item->instance;

        nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN SERVER: stdin fd = %d, stdout fd = %d, stderr fd = %d, cmd: %s",
               item->options.stdio[0].data.fd, item->options.stdio[1].data.fd, item->options.stdio[2].data.fd,
               item->options.file);

        // uv_spawn() does not close all other open file descriptors
        // we have to close them manually
        int fds[3] = { item->options.stdio[0].data.fd, item->options.stdio[1].data.fd, item->options.stdio[2].data.fd };
        os_close_all_non_std_open_fds_except(fds, 3, CLOSE_RANGE_CLOEXEC);

        int rc = uv_spawn(server->loop, &si->process, &item->options);
        if (rc) {
            errno = uv_errno_to_errno(rc);
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: uv_spawn() failed with error %s, %s",
                   uv_err_name(rc), uv_strerror(rc));

            __atomic_store_n(&si->started, false, __ATOMIC_RELAXED);
        }
        else {
            // Successfully spawned

            // get the pid of the process spawned
            si->child_pid = uv_process_get_pid(&si->process);

            // on_process_exit() needs this to find the si
            item->instance->process.data = item->instance;

            nd_log(NDLS_COLLECTORS, NDLP_INFO,
                   "SPAWN SERVER: process created with pid %d", si->child_pid);

            __atomic_store_n(&si->started, true, __ATOMIC_RELAXED);
        }

        uv_sem_post(&si->sem);
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

static void close_handle(uv_handle_t* handle, void* arg __maybe_unused) {
    if (!uv_is_closing(handle)) {
        uv_close(handle, NULL);
    }
}

void spawn_server_destroy(SPAWN_SERVER *server) {
    if (!server) return;

    __atomic_store_n(&server->stopping, true, __ATOMIC_RELAXED);

    // Trigger the async callback to stop the event loop
    uv_async_send(&server->async);

    // Wait for the server thread to finish
    uv_thread_join(&server->thread);

    uv_stop(server->loop);
    uv_close((uv_handle_t*)&server->async, NULL);

    // Walk through and close any remaining handles
    uv_walk(server->loop, close_handle, NULL);

    uv_loop_close(server->loop);
    freez(server->loop);
    freez((void *)server->name);
    freez(server);
}

static void on_process_exit(uv_process_t *req, int64_t exit_status, int term_signal) {
    SPAWN_INSTANCE *si = (SPAWN_INSTANCE *)req->data;
    si->exit_code = (int)(term_signal ? term_signal : exit_status << 8);
    uv_close((uv_handle_t *)req, NULL); // Properly close the process handle

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "SPAWN SERVER: process with pid %d exited with code %d and term_signal %d",
           si->child_pid, (int)exit_status, term_signal);

    uv_sem_post(&si->sem); // Signal that the process has exited
}

SPAWN_INSTANCE* spawn_server_exec(SPAWN_SERVER *server, int stderr_fd __maybe_unused, int custom_fd __maybe_unused, const char **argv, const void *data __maybe_unused, size_t data_size __maybe_unused, SPAWN_INSTANCE_TYPE type) {
    if (type != SPAWN_INSTANCE_TYPE_EXEC)
        return NULL;

    int stdin_pipe[2] = { -1, -1 };
    int stdout_pipe[2] = { -1, -1 };

    SPAWN_INSTANCE *si = callocz(1, sizeof(SPAWN_INSTANCE));
    si->child_pid = -1;
    si->exit_code = -1;
    si->request_id = __atomic_add_fetch(&server->request_id, 1, __ATOMIC_RELAXED);

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

    work_item item = { 0 };
    item.server = server;
    item.instance = si;
    item.options.exit_cb = on_process_exit;
    item.options.file = argv[0];
    item.options.args = (char **)argv;
    item.options.env = (char **)environ;

    item.options.stdio_count = 3;
    uv_stdio_container_t stdio[3];
    stdio[0].flags = UV_INHERIT_FD;
    stdio[0].data.fd = stdin_pipe[PIPE_READ];
    stdio[1].flags = UV_INHERIT_FD;
    stdio[1].data.fd = stdout_pipe[PIPE_WRITE];
    stdio[2].flags = UV_INHERIT_FD;
    stdio[2].data.fd = STDERR_FILENO;
    item.options.stdio = stdio;

    spinlock_lock(&server->spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(server->work_queue, &item, prev, next);
    spinlock_unlock(&server->spinlock);

    uv_async_send(&server->async);

    nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN PARENT: queued command");

    // Wait for the command to be executed
    uv_sem_wait(&si->sem);
    bool started = __atomic_load_n(&si->started, __ATOMIC_RELAXED);

    if(!started) {
        nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN PARENT: process failed to be started");

        close(stdin_pipe[PIPE_READ]);
        close(stdin_pipe[PIPE_WRITE]);
        close(stdout_pipe[PIPE_READ]);
        close(stdout_pipe[PIPE_WRITE]);

        uv_sem_destroy(&si->sem);
        freez(si);
        return NULL;
    }

    // close the child sides of the pipes
    close(stdin_pipe[PIPE_READ]);
    si->write_fd = stdin_pipe[PIPE_WRITE];
    si->read_fd = stdout_pipe[PIPE_READ];
    close(stdout_pipe[PIPE_WRITE]);

    nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN PARENT: process started");

    return si;
}

int spawn_server_exec_kill(SPAWN_SERVER *server __maybe_unused, SPAWN_INSTANCE *si) {
    if(!si) return -1;

    // close all pipe descriptors to force the child to exit
    if(si->read_fd != -1) { close(si->read_fd); si->read_fd = -1; }
    if(si->write_fd != -1) { close(si->write_fd); si->write_fd = -1; }

    if (uv_process_kill(&si->process, SIGTERM)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: uv_process_kill() failed");
        return -1;
    }

    return spawn_server_exec_wait(server, si);
}

int spawn_server_exec_wait(SPAWN_SERVER *server __maybe_unused, SPAWN_INSTANCE *si) {
    if (!si) return -1;

    // close all pipe descriptors to force the child to exit
    if(si->read_fd != -1) { close(si->read_fd); si->read_fd = -1; }
    if(si->write_fd != -1) { close(si->write_fd); si->write_fd = -1; }

    // Wait for the process to exit
    uv_sem_wait(&si->sem);
    int exit_code = si->exit_code;

    uv_sem_destroy(&si->sem);
    freez(si);
    return exit_code;
}

#endif
