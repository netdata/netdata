// SPDX-License-Identifier: GPL-3.0-or-later

#include "spawn.h"
#include "../database/engine/rrdenginelib.h"

static uv_thread_t thread;
int spawn_thread_error;
int spawn_thread_shutdown;

struct spawn_queue spawn_cmd_queue;

static struct spawn_cmd_info *create_spawn_cmd(char *command_to_run)
{
    struct spawn_cmd_info *cmdinfo;

    cmdinfo = mallocz(sizeof(*cmdinfo));
    fatal_assert(0 == uv_cond_init(&cmdinfo->cond));
    fatal_assert(0 == uv_mutex_init(&cmdinfo->mutex));
    cmdinfo->serial = 0; /* invalid */
    cmdinfo->command_to_run = strdupz(command_to_run);
    cmdinfo->exit_status = -1; /* invalid */
    cmdinfo->pid = -1; /* invalid */
    cmdinfo->flags = 0;

    return cmdinfo;
}

void destroy_spawn_cmd(struct spawn_cmd_info *cmdinfo)
{
    uv_cond_destroy(&cmdinfo->cond);
    uv_mutex_destroy(&cmdinfo->mutex);

    freez(cmdinfo->command_to_run);
    freez(cmdinfo);
}

int spawn_cmd_compare(void *a, void *b)
{
    struct spawn_cmd_info *cmda = a, *cmdb = b;

    /* No need for mutex, serial will never change and the entries cannot be deallocated yet */
    if (cmda->serial < cmdb->serial) return -1;
    if (cmda->serial > cmdb->serial) return 1;

    return 0;
}

static void init_spawn_cmd_queue(void)
{
    spawn_cmd_queue.cmd_tree.root = NULL;
    spawn_cmd_queue.cmd_tree.compar = spawn_cmd_compare;
    spawn_cmd_queue.size = 0;
    spawn_cmd_queue.latest_serial = 0;
    fatal_assert(0 == uv_cond_init(&spawn_cmd_queue.cond));
    fatal_assert(0 == uv_mutex_init(&spawn_cmd_queue.mutex));
}

/*
 * Returns serial number of the enqueued command
 */
uint64_t spawn_enq_cmd(char *command_to_run)
{
    unsigned queue_size;
    uint64_t serial;
    avl *avl_ret;
    struct spawn_cmd_info *cmdinfo;

    cmdinfo = create_spawn_cmd(command_to_run);

    /* wait for free space in queue */
    uv_mutex_lock(&spawn_cmd_queue.mutex);
    while ((queue_size = spawn_cmd_queue.size) == SPAWN_MAX_OUTSTANDING) {
        uv_cond_wait(&spawn_cmd_queue.cond, &spawn_cmd_queue.mutex);
    }
    fatal_assert(queue_size < SPAWN_MAX_OUTSTANDING);
    spawn_cmd_queue.size = queue_size + 1;

    serial = ++spawn_cmd_queue.latest_serial; /* 0 is invalid */
    cmdinfo->serial = serial; /* No need to take the cmd mutex since it is unreachable at the moment */

    /* enqueue command */
    avl_ret = avl_insert(&spawn_cmd_queue.cmd_tree, (avl *)cmdinfo);
    fatal_assert(avl_ret == (avl *)cmdinfo);
    uv_mutex_unlock(&spawn_cmd_queue.mutex);

    /* wake up event loop */
    fatal_assert(0 == uv_async_send(&spawn_async));
    return serial;
}

/*
 * Blocks until command with serial finishes running. Only one thread is allowed to wait per command.
 */
void spawn_wait_cmd(uint64_t serial, int *exit_status, time_t *exec_run_timestamp)
{
    avl *avl_ret;
    struct spawn_cmd_info tmp, *cmdinfo;

    tmp.serial = serial;

    uv_mutex_lock(&spawn_cmd_queue.mutex);
    avl_ret = avl_search(&spawn_cmd_queue.cmd_tree, (avl *)&tmp);
    uv_mutex_unlock(&spawn_cmd_queue.mutex);

    fatal_assert(avl_ret); /* Could be NULL if more than 1 threads wait for the command */
    cmdinfo = (struct spawn_cmd_info *)avl_ret;

    uv_mutex_lock(&cmdinfo->mutex);
    while (!(cmdinfo->flags & SPAWN_CMD_DONE)) {
        /* Only 1 thread is allowed to wait for this command to finish */
        uv_cond_wait(&cmdinfo->cond, &cmdinfo->mutex);
    }
    uv_mutex_unlock(&cmdinfo->mutex);

    spawn_deq_cmd(cmdinfo);
    *exit_status = cmdinfo->exit_status;
    *exec_run_timestamp = cmdinfo->exec_run_timestamp;

    destroy_spawn_cmd(cmdinfo);
}

void spawn_deq_cmd(struct spawn_cmd_info *cmdinfo)
{
    unsigned queue_size;
    avl *avl_ret;

    uv_mutex_lock(&spawn_cmd_queue.mutex);
    queue_size = spawn_cmd_queue.size;
    fatal_assert(queue_size);
    /* dequeue command */
    avl_ret = avl_remove(&spawn_cmd_queue.cmd_tree, (avl *)cmdinfo);
    fatal_assert(avl_ret);

    spawn_cmd_queue.size = queue_size - 1;

    /* wake up callers */
    uv_cond_signal(&spawn_cmd_queue.cond);
    uv_mutex_unlock(&spawn_cmd_queue.mutex);
}

/*
 * Must be called from the spawn client event loop context. This way no mutex is needed because the event loop is the
 * only writer as far as struct spawn_cmd_info entries are concerned.
 */
static int find_unprocessed_spawn_cmd_cb(void *entry, void *data)
{
    struct spawn_cmd_info **cmdinfop = data, *cmdinfo = entry;

    if (!(cmdinfo->flags & SPAWN_CMD_PROCESSED)) {
        *cmdinfop = cmdinfo;
        return -1; /* break tree traversal */
    }
    return 0; /* continue traversing */
}

struct spawn_cmd_info *spawn_get_unprocessed_cmd(void)
{
    struct spawn_cmd_info *cmdinfo;
    unsigned queue_size;
    int ret;

    uv_mutex_lock(&spawn_cmd_queue.mutex);
    queue_size = spawn_cmd_queue.size;
    if (queue_size == 0) {
        uv_mutex_unlock(&spawn_cmd_queue.mutex);
        return NULL;
    }
    /* find command */
    cmdinfo = NULL;
    ret = avl_traverse(&spawn_cmd_queue.cmd_tree, find_unprocessed_spawn_cmd_cb, (void *)&cmdinfo);
    if (-1 != ret) { /* no commands available for processing */
        uv_mutex_unlock(&spawn_cmd_queue.mutex);
        return NULL;
    }
    uv_mutex_unlock(&spawn_cmd_queue.mutex);

    return cmdinfo;
}

/**
 * This function spawns a process that shares a libuv IPC pipe with the caller and performs spawn server duties.
 * The spawn server process will close all open file descriptors except for the pipe, UV_STDOUT_FD, and UV_STDERR_FD.
 * The caller has to be the netdata user as configured.
 *
 * @param loop the libuv loop of the caller context
 * @param spawn_channel the birectional libuv IPC pipe that the server and the caller will share
 * @param process the spawn server libuv process context
 * @return 0 on success or the libuv error code
 */
int create_spawn_server(uv_loop_t *loop, uv_pipe_t *spawn_channel, uv_process_t *process)
{
    uv_process_options_t options = {0};
    char *args[3];
    int ret;
#define SPAWN_SERVER_DESCRIPTORS (3)
    uv_stdio_container_t stdio[SPAWN_SERVER_DESCRIPTORS];
    struct passwd *passwd = NULL;
    char *user = NULL;

    passwd = getpwuid(getuid());
    user = (passwd && passwd->pw_name) ? passwd->pw_name : "";

    args[0] = exepath;
    args[1] = SPAWN_SERVER_COMMAND_LINE_ARGUMENT;
    args[2] = NULL;

    memset(&options, 0, sizeof(options));
    options.file = exepath;
    options.args = args;
    options.exit_cb = NULL; //exit_cb;
    options.stdio = stdio;
    options.stdio_count = SPAWN_SERVER_DESCRIPTORS;

    stdio[0].flags = UV_CREATE_PIPE | UV_READABLE_PIPE | UV_WRITABLE_PIPE;
    stdio[0].data.stream = (uv_stream_t *)spawn_channel; /* bidirectional libuv pipe */
    stdio[1].flags = UV_INHERIT_FD;
    stdio[1].data.fd = 1 /* UV_STDOUT_FD */;
    stdio[2].flags = UV_INHERIT_FD;
    stdio[2].data.fd = 2 /* UV_STDERR_FD */;

    ret = uv_spawn(loop, process, &options); /* execute the netdata binary again as the netdata user */
    if (0 != ret) {
        error("uv_spawn (process: \"%s\") (user: %s) failed (%s).", exepath, user, uv_strerror(ret));
        fatal("Cannot start netdata without the spawn server.");
    }

    return ret;
}

#define CONCURRENT_SPAWNS 16
#define SPAWN_ITERATIONS  10000
#undef CONCURRENT_STRESS_TEST

void spawn_init(void)
{
    struct completion completion;
    int error;

    info("Initializing spawn client.");

    init_spawn_cmd_queue();

    init_completion(&completion);
    error = uv_thread_create(&thread, spawn_client, &completion);
    if (error) {
        error("uv_thread_create(): %s", uv_strerror(error));
        goto after_error;
    }
    /* wait for spawn client thread to initialize */
    wait_for_completion(&completion);
    destroy_completion(&completion);
    uv_thread_set_name_np(thread, "DAEMON_SPAWN");

    if (spawn_thread_error) {
        error = uv_thread_join(&thread);
        if (error) {
            error("uv_thread_create(): %s", uv_strerror(error));
        }
        goto after_error;
    }
#ifdef CONCURRENT_STRESS_TEST
    signals_reset();
    signals_unblock();

    sleep(60);
    uint64_t serial[CONCURRENT_SPAWNS];
    for (int j = 0 ; j < SPAWN_ITERATIONS ; ++j) {
        for (int i = 0; i < CONCURRENT_SPAWNS; ++i) {
            char cmd[64];
            sprintf(cmd, "echo CONCURRENT_STRESS_TEST %d 1>&2", j * CONCURRENT_SPAWNS + i + 1);
            serial[i] = spawn_enq_cmd(cmd);
            info("Queued command %s for spawning.", cmd);
        }
        int exit_status;
        time_t exec_run_timestamp;
        for (int i = 0; i < CONCURRENT_SPAWNS; ++i) {
            info("Started waiting for serial %llu exit status %d run timestamp %llu.", serial[i], exit_status,
                 exec_run_timestamp);
            spawn_wait_cmd(serial[i], &exit_status, &exec_run_timestamp);
            info("Finished waiting for serial %llu exit status %d run timestamp %llu.", serial[i], exit_status,
                 exec_run_timestamp);
        }
    }
    exit(0);
#endif
    return;

    after_error:
    error("Failed to initialize spawn service. The alarms notifications will not be spawned.");
}
