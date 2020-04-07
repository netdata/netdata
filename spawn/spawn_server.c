// SPDX-License-Identifier: GPL-3.0-or-later

#include "spawn.h"

static uv_loop_t *loop;
static struct completion completion;
static uv_pipe_t server_pipe;

static int server_shutdown = 0;

static uv_thread_t thread;

/* spawn outstanding execution structure */
static avl_tree_lock spawn_outstanding_exec_tree;

struct spawn_execution_info {
    avl avl;

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

static void pipe_close_cb(uv_handle_t *handle)
{
    ;
}

static void after_pipe_write(uv_write_t *req, int status)
{
    fprintf(stderr, "SERVER %s called status=%d\n", __func__, status);
    freez(req->data);
}

void child_waited_async_close_cb(uv_handle_t *handle)
{
    freez(handle);
}

static void child_waited_async_cb(uv_async_t *async_handle)
{
    uv_buf_t writebuf[2];
    int ret;
    struct spawn_execution_info *exec_info;
    struct write_context *write_ctx;

    while (NULL != (exec_info = dequeue_child_waited_list())) {
        write_ctx = mallocz(sizeof(*write_ctx));
        write_ctx->write_req.data = write_ctx;


        write_ctx->header.opcode = SPAWN_PROT_CMD_EXIT_STATUS;
        write_ctx->header.handle = exec_info->handle;
        write_ctx->exit_status.exec_exit_status = exec_info->exit_status;
        writebuf[0] = uv_buf_init((char *) &write_ctx->header, sizeof(write_ctx->header));
        writebuf[1] = uv_buf_init((char *) &write_ctx->exit_status, sizeof(write_ctx->exit_status));
        fprintf(stderr, "SERVER %s SPAWN_PROT_CMD_EXIT_STATUS\n", __func__);
        ret = uv_write(&write_ctx->write_req, (uv_stream_t *) &server_pipe, writebuf, 2, after_pipe_write);
        assert(ret == 0);

        freez(exec_info);
    }
//    uv_close((uv_handle_t *)async_handle, child_waited_async_close_cb);
//    uv_stop(async_handle->loop);
}

static void wait_children(void *arg)
{
    siginfo_t i;
    struct spawn_execution_info tmp, *exec_info;
    avl *ret_avl;

    (void)arg;
    while (!server_shutdown) {
        uv_mutex_lock(&wait_children_mutex);
        while (!spawned_processes) {
            uv_cond_wait(&wait_children_cond, &wait_children_mutex);
        }
        spawned_processes = 0;
        uv_mutex_unlock(&wait_children_mutex);

        while (1) {
            i.si_pid = 0;
            if (waitid(P_ALL, (id_t) 0, &i, WEXITED) == -1) {
                if (errno != ECHILD)
                    fprintf(stderr, "SPAWN: Failed to wait: %s\n", strerror(errno));
                break;
            }
            if (i.si_pid == 0) {
                fprintf(stderr, "No child exited.\n");
                break;
            }
            fprintf(stderr, "Successfully waited for pid:%d.\n", (int) i.si_pid);

            assert(CLD_EXITED == i.si_code);
            tmp.pid = (pid_t)i.si_pid;
            while (NULL == (ret_avl = avl_remove_lock(&spawn_outstanding_exec_tree, (avl *)&tmp))) {
                fprintf(stderr,
                        "SPAWN: race condition detected, waiting for child process %d to be indexed.\n",
                        (int)tmp.pid);
                (void)sleep_usec(10000); /* 10 msec */
            }
            exec_info = (struct spawn_execution_info *)ret_avl;
            exec_info->exit_status = i.si_status;
            enqueue_child_waited_list(exec_info);

            /* wake up event loop */
            assert(0 == uv_async_send(&child_waited_async));
        }
    }
}

void spawn_protocol_execute_command(void *handle, char *command_to_run, uint16_t command_length)
{
    uv_buf_t writebuf[2];
    int ret;
    avl *avl_ret;
    struct spawn_execution_info *exec_info;
    struct write_context *write_ctx;

    write_ctx = mallocz(sizeof(*write_ctx));
    write_ctx->write_req.data = write_ctx;

    command_to_run[command_length] = '\0';
    fprintf(stderr, "SPAWN: executing command '%s'\n", command_to_run);
    if (netdata_spawn(command_to_run, &write_ctx->spawn_result.exec_pid)) {
        fprintf(stderr, "SPAWN: Cannot spawn(\"%s\", \"r\").\n", command_to_run);
        write_ctx->spawn_result.exec_pid = 0;
    } else { /* successfully spawned command */
        write_ctx->spawn_result.exec_run_timestamp = now_realtime_sec();

        /* record it for when the process finishes execution */
        exec_info = mallocz(sizeof(*exec_info));
        exec_info->handle = handle;
        exec_info->pid = write_ctx->spawn_result.exec_pid;
        avl_ret = avl_insert_lock(&spawn_outstanding_exec_tree, (avl *)exec_info);
        assert(avl_ret == (avl *)exec_info);

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
    fprintf(stderr, "SERVER %s SPAWN_PROT_SPAWN_RESULT\n", __func__);
    ret = uv_write(&write_ctx->write_req, (uv_stream_t *)&server_pipe, writebuf, 2, after_pipe_write);
    assert(ret == 0);
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
            copy_to_prot_buffer(required_len - prot_buffer_len, &source, &source_len);
        if (prot_buffer_len < required_len)
            return; /* Source buffer ran out */

        header = (struct spawn_prot_header *)prot_buffer;
        assert(SPAWN_PROT_EXEC_CMD == header->opcode);
        assert(NULL != header->handle);

        required_len += sizeof(*payload);
        if (prot_buffer_len < required_len)
            copy_to_prot_buffer(required_len - prot_buffer_len, &source, &source_len);
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
            copy_to_prot_buffer(required_len - prot_buffer_len, &source, &source_len);
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

    if (nread < 0) { /* stop stream due to EOF or error */
        (void)uv_read_stop((uv_stream_t *)pipe);
    } else if (nread) {
        fprintf(stderr, "SERVER %s nread %u\n", __func__, (unsigned)nread);
        server_parse_spawn_protocol(nread, buf->base);
    }
    if (buf && buf->len) {
        freez(buf->base);
    }

    if (nread < 0 && UV_EOF != nread) { /* postpone until we write or something? */
//        uv_close((uv_handle_t *)pipe, pipe_close_cb);
    }
}

static void on_read_alloc(uv_handle_t *handle,
                          size_t suggested_size,
                          uv_buf_t* buf)
{
    buf->base = mallocz(suggested_size);
    buf->len = suggested_size;
}

void spawn_server(void)
{
    int ret, error;

    test_clock_boottime();
    test_clock_monotonic_coarse();

    // close all open file descriptors, except the standard ones
    // the caller may have left open files (lxc-attach has this issue)
    int fd;
    for(fd = (int)(sysconf(_SC_OPEN_MAX) - 1) ; fd > 2 ; --fd)
        if(fd_is_valid(fd))
            close(fd);

    // Have the libuv IPC pipe be closed when forking child processes
    (void) fcntl(0, F_SETFD, FD_CLOEXEC);
    fprintf(stderr, "SPAWN SERVER IS UP!\n");

    loop = uv_default_loop();
    loop->data = NULL;

    ret = uv_pipe_init(loop, &server_pipe, 1);
    if (ret) {
        fprintf(stderr, "uv_pipe_init(): %s\n", uv_strerror(ret));
        error = ret;
        goto error_after_pipe_init;
    }
    assert(server_pipe.ipc);

    ret = uv_pipe_open(&server_pipe, 0 /* UV_STDIN_FD */);
    if (ret) {
        fprintf(stderr, "uv_pipe_open(): %s\n", uv_strerror(ret));
        error = ret;
        goto error_after_pipe_open;
    }
    avl_init_lock(&spawn_outstanding_exec_tree, spawn_exec_compare);

    spawned_processes = 0;
    assert(0 == uv_cond_init(&wait_children_cond));
    assert(0 == uv_mutex_init(&wait_children_mutex));
    child_waited_list = NULL;
    assert(0 == uv_async_init(loop, &child_waited_async, child_waited_async_cb));

    error = uv_thread_create(&thread, wait_children, NULL);
    if (error) {
        fprintf(stderr, "uv_thread_create(): %s\n", uv_strerror(error));
        exit(1);
    }

    prot_buffer_len = 0;
    ret = uv_read_start((uv_stream_t *)&server_pipe, on_read_alloc, on_pipe_read);
    assert(ret == 0);

    while (!server_shutdown) {
        uv_run(loop, UV_RUN_DEFAULT);
    }
    /* cleanup operations of the event loop */
    fprintf(stderr, "Shutting down spawn server event loop.\n");
    uv_close((uv_handle_t *)&server_pipe, NULL);
    uv_run(loop, UV_RUN_DEFAULT); /* flush all libuv handles */

    fprintf(stderr, "Shutting down spawn server loop complete.\n");
    assert(0 == uv_loop_close(loop));

    exit(0);

error_after_pipe_open:
    uv_close((uv_handle_t*)&server_pipe, NULL);
error_after_pipe_init:
    uv_run(loop, UV_RUN_DEFAULT); /* flush all libuv handles */
    assert(0 == uv_loop_close(loop));

    exit(error);
}
