// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef REPLICATION_H
#define REPLICATION_H

#include "database/rrd.h"
#include "stream-circular-buffer.h"

#ifdef __cplusplus
extern "C" {
#endif

#define MAX_REPLICATION_THREADS 256
#define MAX_REPLICATION_PREFETCH 256

struct parser;

struct replication_query_statistics {
    SPINLOCK spinlock;
    size_t queries_started;
    size_t queries_finished;
    size_t points_read;
    size_t points_generated;
};

struct replication_query_statistics replication_get_query_statistics(void);

void replication_sender_init(struct sender_state *sender);
void replication_sender_cleanup(struct sender_state *sender);
void replication_sender_delete_pending_requests(struct sender_state *sender);
void replication_sender_request_add(struct sender_state *sender, const char *chart_id, time_t after, time_t before, bool start_streaming);
void replication_sender_recalculate_buffer_used_ratio_unsafe(struct sender_state *s);

int64_t replication_sender_allocated_memory(void);
size_t replication_sender_allocated_buffers(void);

int replication_prefetch_default(void);
int replication_threads_default(void);

#ifdef __cplusplus
}
#endif

#endif /* REPLICATION_H */
