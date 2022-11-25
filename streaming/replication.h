// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef REPLICATION_H
#define REPLICATION_H

#include "daemon/common.h"

bool replicate_chart_response(RRDHOST *rh, RRDSET *rs, bool start_streaming, time_t after, time_t before);

typedef int (*send_command)(const char *txt, void *data);

bool replicate_chart_request(send_command callback, void *callback_data,
                             RRDHOST *rh, RRDSET *rs,
                             time_t first_entry_child, time_t last_entry_child, time_t child_world_time,
                             time_t response_first_start_time, time_t response_last_end_time);

void replication_init_sender(struct sender_state *sender);
void replication_cleanup_sender(struct sender_state *sender);
void replication_sender_delete_pending_requests(struct sender_state *sender);
void replication_add_request(struct sender_state *sender, const char *chart_id, time_t after, time_t before, bool start_streaming);
void replication_recalculate_buffer_used_ratio_unsafe(struct sender_state *s);

#endif /* REPLICATION_H */
