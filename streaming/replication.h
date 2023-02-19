// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef REPLICATION_H
#define REPLICATION_H

#include "daemon/common.h"

struct replication_query_statistics {
    SPINLOCK spinlock;
    size_t queries_started;
    size_t queries_finished;
    size_t points_read;
    size_t points_generated;
};

struct replication_query_statistics replication_get_query_statistics(void);

bool replicate_chart_response(RRDHOST *rh, RRDSET *rs, bool start_streaming, time_t after, time_t before);

typedef int (*send_command)(const char *txt, void *data);

bool replicate_chart_request(send_command callback, void *callback_data,
                             RRDHOST *rh, RRDSET *rs,
                             time_t child_first_entry, time_t child_last_entry, time_t child_wall_clock_time,
                             time_t response_first_start_time, time_t response_last_end_time);

void replication_init_sender(struct sender_state *sender);
void replication_cleanup_sender(struct sender_state *sender);
void replication_sender_delete_pending_requests(struct sender_state *sender);
void replication_add_request(struct sender_state *sender, const char *chart_id, time_t after, time_t before, bool start_streaming);
void replication_recalculate_buffer_used_ratio_unsafe(struct sender_state *s);

size_t replication_allocated_memory(void);
size_t replication_allocated_buffers(void);

#endif /* REPLICATION_H */
