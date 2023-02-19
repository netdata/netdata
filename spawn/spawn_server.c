// SPDX-License-Identifier: GPL-3.0-or-later

#include "spawn.h"

static uv_loop_t *loop;
static uv_pipe_t server_pipe;

static int server_shutdown = 0;

static uv_thread_t thread;

/* spawn outstanding execution structure */
static avl_tree_lock spawn_outstanding_exec_tree;

static char prot_buffer[MAX_COMMAND_LENGTH];
static unsigned prot_buffer_len = 0;

struct spawn_execution_info {
    avl_t avl;

    void *handle;
    int exit_status;
    pid_t pid;
    struct spawn_execution_info *next;
};

int spawn_exec_compare(void *a, void *b)
{
    struct spawn_execution_info *spwna = a, *spwnb = b;

    if (spwna->pid < spwnb->pid) return -1;
    if (spwna->pid > spwnb->pid) return 1;

    return 0;
}

/* wake up waiter thread to reap the spawned processes */
static uv_mutex_t wait_children_mutex;
static uv_cond_t wait_children_cond;
static uint8_t spawned_processes;
static struct spawn_execution_info *child_waited_list;
static uv_async_t child_waited_async;

static inline struct spawn_execution_info *dequeue_child_waited_list(void)
{
    struct spawn_execution_info *exec_info;

    uv_mutex_lock(&wait_children_mutex);
    if (NULL == child_waited_list) {
        exec_info = NULL;
    } else {
        exec_info = child_waited_list;
        child_waited_list = exec_info->next;
    }
    uv_mutex_unlock(&wait_children_mutex);

    return exec_info;
}

static inline void enqueue_child_waited_list(struct spawn_execution_info *exec_info)
{
    uv_mutex_lock(&wait_children_mutex);
    exec_info->next = child_waited_list;
    child_waited_list = exec_info;
    uv_mutex_unlock(&wait_children_mutex);
}

static void after_pipe_write(uv_write_t *req, int status)
{
    (void)status;
#ifdef SPAWN_DEBUG
    fprintf(stderr, "SERVER %s called status=%d\n", __func__, status);
#endif
    void **data = req->data;
    freez(data[0]);
    freez(data[1]);
    freez(data);
}

static void child_waited_async_cb(uv_async_t *async_handle)
{
    uv_buf_t *writebuf;
    int ret;
    struct spawn_execution_info *exec_info;
    struct write_context *write_ctx;

    (void)async_handle;
    while (NULL != (exec_info = dequeue_child_waited_list())) {
        write_ctx = mallocz(sizeof(*write_ctx));

        void **data = callocz(2, sizeof(void *));
        writebuf = callocz(2, sizeof(uv_buf_t));

        data[0] = write_ctx;
        data[1] = writebuf;
        write_ctx->write_req.data = data;

        write_ctx->header.opcode = SPAWN_PROT_CMD_EXIT_STATUS;
        write_ctx->header.handle = exec_info->handle;
        write_ctx->exit_status.exec_exit_status = exec_info->exit_status;
        writebuf[0] = uv_buf_init((char *) &write_ctx->header, sizeof(write_ctx->header));
        writebuf[1] = uv_buf_init((char *) &write_ctx->exit_status, sizeof(write_ctx->exit_status));
#ifdef SPAWN_DEBUG
        fprintf(stderr, "SERVER %s SPAWN_PROT_CMD_EXIT_STATUS\n", __func__);
#endif
        ret = uv_write(&write_ctx->write_req, (uv_stream_t *) &server_pipe, writebuf, 2, after_pipe_write);
        fatal_assert(ret == 0);

        freez(exec_info);
    }
}

static void wait_children(void *arg)
{
    siginfo_t i;
    struct spawn_execution_info tmp, *exec_info;
    avl_t *ret_avl;

    (void)arg;
    while (!server_shutdown) {
        uv_mutex_lock(&wait_children_mutex);
        while (!spawned_processes) {
            uv_cond_wait(&wait_children_cond, &wait_children_mutex);
        }
        spawned_processes = 0;
        uv_mutex_unlock(&wait_children_mutex);

        while (!server_shutdown) {
            i.si_pid = 0;
            if (waitid(P_ALL, (id_t) 0, &i, WEXITED) == -1) {
                if (errno != ECHILD)
                    fprintf(stderr, "SPAWN: Failed to wait: %s\n", strerror(errno));
                break;
            }
            if (i.si_pid == 0) {
                fprintf(stderr, "SPAWN: No child exited.\n");
                break;
            }
#ifdef SPAWN_DEBUG
            fprintf(stderr, "SPAWN: Successfully waited for pid:%d.\n", (int) i.si_pid);
#endif
            fatal_assert(CLD_EXITED == i.si_code);
            tmp.pid = (pid_t)i.si_pid;
            while (NULL == (ret_avl = avl_remove_lock(&spawn_outstanding_exec_tree, (avl_t *)&tmp))) {
                fprintf(stderr,
                        "SPAWN: race condition detected, waiting for child process %d to be indexed.\n",
                        (int)tmp.pid);
                (void)sleep_usec(10000); /* 10 msec */
            }
            exec_info = (struct spawn_execution_info *)ret_avl;
            exec_info->exit_status = i.si_status;
            enqueue_child_waited_list(exec_info);

            /* wake up event loop */
            fatal_assert(0 == uv_async_send(&child_waited_async));
        }
    }
}

void spawn_protocol_execute_command(void *handle, char *command_to_run, uint16_t command_length)
{
    uv_buf_t *writebuf;
    int ret;
    avl_t *avl_ret;
    struct spawn_execution_info *exec_info;
    struct write_context *write_ctx;

    write_ctx = mallocz(sizeof(*write_ctx));
    void **data = callocz(2, sizeof(void *));
    writebuf = callocz(2, sizeof(uv_buf_t));
    data[0] = write_ctx;
    data[1] = writebuf;
    write_ctx->write_req.data = data;

    command_to_run[command_length] = '\0';
#ifdef SPAWN_DEBUG
    fprintf(stderr, "SPAWN: executing command '%s'\n", command_to_run);
#endif
    if (netdata_spawn(command_to_run, &write_ctx->spawn_result.exec_pid)) {
        fprintf(stderr, "SPAWN: Cannot spawn(\"%s\", \"r\").\n", command_to_run);
        write_ctx->spawn_result.exec_pid = 0;
    } else { /* successfully spawned command */
        write_ctx->spawn_result.exec_run_timestamp = now_realtime_sec();

        /* record it for when the process finishes execution */
        exec_info = mallocz(sizeof(*exec_info));
        exec_info->handle = handle;
        exec_info->pid = write_ctx->spawn_result.exec_pid;
        avl_ret = avl_insert_lock(&spawn_outstanding_exec_tree, (avl_t *)exec_info);
        fatal_assert(avl_ret == (avl_t *)exec_info);

        /* wake up the thread that blocks waiting for processes to exit */
        uv_mutex_lock(&wait_children_mutex);
        spawned_processes = 1;
        uv_cond_signal(&wait_children_cond);
        uv_mutex_unlock(&wait_children_mutex);
    }

    write_ctx->header.opcode = SPAWN_PROT_SPAWN_RESULT;
    write_ctx->header.handle = handle;
    writebuf[0] = uv_buf_init((char *)&write_ctx->header, sizeof(write_ctx->header));
    writebuf[1] = uv_buf_init((char *)&write_ctx->spawn_result, sizeof(write_ctx->spawn_result));
#ifdef SPAWN_DEBUG
    fprintf(stderr, "SERVER %s SPAWN_PROT_SPAWN_RESULT\n", __func__);
#endif
    ret = uv_write(&write_ctx->write_req, (uv_stream_t *)&server_pipe, writebuf, 2, after_pipe_write);
    fatal_assert(ret == 0);
}

static void server_parse_spawn_protocol(unsigned source_len, char *source)
{
    unsigned required_len;
    struct spawn_prot_header *header;
    struct spawn_prot_exec_cmd *payload;
    uint16_t command_length;

    while (source_len) {
        required_len = sizeof(*header);
        if (prot_buffer_len < required_len)
            copy_to_prot_buffer(prot_buffer, &prot_buffer_len, required_len - prot_buffer_len, &source, &source_len);
        if (prot_buffer_len < required_len)
            return; /* Source buffer ran out */

        header = (struct spawn_prot_header *)prot_buffer;
        fatal_assert(SPAWN_PROT_EXEC_CMD == header->opcode);
        fatal_assert(NULL != header->handle);

        required_len += sizeof(*payload);
        if (prot_buffer_len < required_len)
            copy_to_prot_buffer(prot_buffer, &prot_buffer_len, required_len - prot_buffer_len, &source, &source_len);
        if (prot_buffer_len < required_len)
            return; /* Source buffer ran out */

        payload = (struct spawn_prot_exec_cmd *)(header + 1);
        command_length = payload->command_length;

        required_len += command_length;
        if (unlikely(required_len > MAX_COMMAND_LENGTH - 1)) {
            fprintf(stderr, "SPAWN: Ran out of protocol buffer space.\n");
            command_length = (MAX_COMMAND_LENGTH - 1) - (sizeof(*header) + sizeof(*payload));
            required_len = MAX_COMMAND_LENGTH - 1;
        }
        if (prot_buffer_len < required_len)
            copy_to_prot_buffer(prot_buffer, &prot_buffer_len, required_len - prot_buffer_len, &source, &source_len);
        if (prot_buffer_len < required_len)
            return; /* Source buffer ran out */

        spawn_protocol_execute_command(header->handle, payload->command_to_run, command_length);
        prot_buffer_len = 0;
    }
}

static void on_pipe_read(uv_stream_t *pipe, ssize_t nread, const uv_buf_t *buf)
{
    if (0 == nread) {
        fprintf(stderr, "SERVER %s: Zero bytes read from spawn pipe.\n", __func__);
    } else if (UV_EOF == nread) {
        fprintf(stderr, "EOF found in spawn pipe.\n");
    } else if (nread < 0) {
        fprintf(stderr, "%s: %s\n", __func__, uv_strerror(nread));
    }

    if (nread < 0) { /* stop spawn server due to EOF or error */
        int error;

        uv_mutex_lock(&wait_children_mutex);
        server_shutdown = 1;
        spawned_processes = 1;
        uv_cond_signal(&wait_children_cond);
        uv_mutex_unlock(&wait_children_mutex);

        fprintf(stderr, "Shutting down spawn server event loop.\n");
        /* cleanup operations of the event loop */
        (void)uv_read_stop((uv_stream_t *) pipe);
        uv_close((uv_handle_t *)&server_pipe, NULL);

        error = uv_thread_join(&thread);
        if (error) {
            fprintf(stderr, "uv_thread_create(): %s", uv_strerror(error));
        }
        /* After joining it is safe to destroy child_waited_async */
        uv_close((uv_handle_t *)&child_waited_async, NULL);
    } else if (nread) {
#ifdef SPAWN_DEBUG
        fprintf(stderr, "SERVER %s nread %u\n", __func__, (unsigned)nread);
#endif
        server_parse_spawn_protocol(nread, buf->base);
    }
    if (buf && buf->len) {
        freez(buf->base);
    }
}

static void on_read_alloc(uv_handle_t *handle,
                          size_t suggested_size,
                          uv_buf_t* buf)
{
    (void)handle;
    buf->base = mallocz(suggested_size);
    buf->len = suggested_size;
}

static void ignore_signal_handler(int signo) {
    /*
     * By having a signal handler we allow spawned processes to reset default signal dispositions. Setting SIG_IGN
     * would be inherited by the spawned children which is not desirable.
     */
    (void)signo;
}

void spawn_server(void)
{
    int error;

    // initialize the system clocks
    clocks_init();

    // close all open file descriptors, except the standard ones
    // the caller may have left open files (lxc-attach has this issue)
    for_each_open_fd(OPEN_FD_ACTION_CLOSE, OPEN_FD_EXCLUDE_STDIN | OPEN_FD_EXCLUDE_STDOUT | OPEN_FD_EXCLUDE_STDERR);

    // Have the libuv IPC pipe be closed when forking child processes
    (void) fcntl(0, F_SETFD, FD_CLOEXEC);
    fprintf(stderr, "Spawn server is up.\n");

    // Define signals we want to ignore
    struct sigaction sa;
    int signals_to_ignore[] = {SIGPIPE, SIGINT, SIGQUIT, SIGTERM, SIGHUP, SIGUSR1, SIGUSR2, SIGBUS, SIGCHLD};
    unsigned ignore_length = sizeof(signals_to_ignore) / sizeof(signals_to_ignore[0]);

    unsigned i;
    for (i = 0; i < ignore_length ; ++i) {
        sa.sa_flags = 0;
        sigemptyset(&sa.sa_mask);
        sa.sa_handler = ignore_signal_handler;
        if(sigaction(signals_to_ignore[i], &sa, NULL) == -1)
            fprintf(stderr, "SPAWN: Failed to change signal handler for signal: %d.\n", signals_to_ignore[i]);
    }

    signals_unblock();

    loop = uv_default_loop();
    loop->data = NULL;

    error = uv_pipe_init(loop, &server_pipe, 1);
    if (error) {
        fprintf(stderr, "uv_pipe_init(): %s\n", uv_strerror(error));
        exit(error);
    }
    fatal_assert(server_pipe.ipc);

    error = uv_pipe_open(&server_pipe, 0 /* UV_STDIN_FD */);
    if (error) {
        fprintf(stderr, "uv_pipe_open(): %s\n", uv_strerror(error));
        exit(error);
    }
    avl_init_lock(&spawn_outstanding_exec_tree, spawn_exec_compare);

    spawned_processes = 0;
    fatal_assert(0 == uv_cond_init(&wait_children_cond));
    fatal_assert(0 == uv_mutex_init(&wait_children_mutex));
    child_waited_list = NULL;
    error = uv_async_init(loop, &child_waited_async, child_waited_async_cb);
    if (error) {
        fprintf(stderr, "uv_async_init(): %s\n", uv_strerror(error));
        exit(error);
    }

    error = uv_thread_create(&thread, wait_children, NULL);
    if (error) {
        fprintf(stderr, "uv_thread_create(): %s\n", uv_strerror(error));
        exit(error);
    }

    prot_buffer_len = 0;
    error = uv_read_start((uv_stream_t *)&server_pipe, on_read_alloc, on_pipe_read);
    fatal_assert(error == 0);

    while (!server_shutdown) {
        uv_run(loop, UV_RUN_DEFAULT);
    }
    fprintf(stderr, "Shutting down spawn server loop complete.\n");
    fatal_assert(0 == uv_loop_close(loop));

    exit(0);
}
