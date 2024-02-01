// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SPAWN_H
#define NETDATA_SPAWN_H 1

#include "daemon/common.h"

#define SPAWN_SERVER_COMMAND_LINE_ARGUMENT "--special-spawn-server"

typedef enum spawn_protocol {
    SPAWN_PROT_EXEC_CMD = 0,
    SPAWN_PROT_SPAWN_RESULT,
    SPAWN_PROT_CMD_EXIT_STATUS
} spawn_prot_t;

struct spawn_prot_exec_cmd {
    uint16_t command_length;
    char command_to_run[];
};

struct spawn_prot_spawn_result {
    pid_t exec_pid; /* 0 if failed to spawn */
    time_t exec_run_timestamp; /* time of successfully spawning the command */
};

struct spawn_prot_cmd_exit_status {
    int exec_exit_status;
};

struct spawn_prot_header {
    spawn_prot_t opcode;
    void *handle;
};

#undef SPAWN_DEBUG /* define to enable debug prints */

#define SPAWN_MAX_OUTSTANDING (32768)

#define SPAWN_CMD_PROCESSED         0x00000001
#define SPAWN_CMD_IN_PROGRESS       0x00000002
#define SPAWN_CMD_FAILED_TO_SPAWN   0x00000004
#define SPAWN_CMD_DONE              0x00000008

struct spawn_cmd_info {
    avl_t avl;

    /* concurrency control per command */
    uv_mutex_t mutex;
    uv_cond_t cond; /* users block here until command has finished */

    uint64_t serial;
    char *command_to_run;
    int exit_status;
    pid_t pid;
    unsigned long flags;
    time_t exec_run_timestamp; /* time of successfully spawning the command */
};

/* spawn command queue */
struct spawn_queue {
    avl_tree_type cmd_tree;

    /* concurrency control of command queue */
    uv_mutex_t mutex;
    uv_cond_t cond;

    volatile unsigned size;
    uint64_t latest_serial;
};

struct write_context {
    uv_write_t write_req;
    struct spawn_prot_header header;
    struct spawn_prot_cmd_exit_status exit_status;
    struct spawn_prot_spawn_result spawn_result;
    struct spawn_prot_exec_cmd payload;
};

extern int spawn_thread_error;
extern int spawn_thread_shutdown;
extern uv_async_t spawn_async;

void spawn_init(void);
void spawn_server(void);
void spawn_client(void *arg);
void destroy_spawn_cmd(struct spawn_cmd_info *cmdinfo);
uint64_t spawn_enq_cmd(const char *command_to_run);
void spawn_wait_cmd(uint64_t serial, int *exit_status, time_t *exec_run_timestamp);
void spawn_deq_cmd(struct spawn_cmd_info *cmdinfo);
struct spawn_cmd_info *spawn_get_unprocessed_cmd(void);
int create_spawn_server(uv_loop_t *loop, uv_pipe_t *spawn_channel, uv_process_t *process);

/*
 * Copies from the source buffer to the protocol buffer. It advances the source buffer by the amount copied. It
 * subtracts the amount copied from the source length.
 */
static inline void copy_to_prot_buffer(char *prot_buffer, unsigned *prot_buffer_len, unsigned max_to_copy,
                                       char **source, unsigned *source_len)
{
    unsigned to_copy;

    to_copy = MIN(max_to_copy, *source_len);
    memcpy(prot_buffer + *prot_buffer_len, *source, to_copy);
    *prot_buffer_len += to_copy;
    *source += to_copy;
    *source_len -= to_copy;
}

#endif //NETDATA_SPAWN_H
