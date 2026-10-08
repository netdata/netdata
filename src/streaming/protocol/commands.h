// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAMING_PROTCOL_COMMANDS_H
#define NETDATA_STREAMING_PROTCOL_COMMANDS_H

#include "database/rrd.h"
#include "../stream.h"

typedef struct rrdset_stream_buffer {
    STREAM_CAPABILITIES capabilities;
    bool v2;
    bool begin_v2_added;
    time_t wall_clock_time;
    RRDSET_FLAGS rrdset_flags;
    time_t last_point_end_time_s;
    BUFFER *wb;
} RRDSET_STREAM_BUFFER;

RRDSET_STREAM_BUFFER stream_send_metrics_init(RRDSET *st, time_t wall_clock_time);

void stream_sender_get_node_and_claim_id_from_parent(struct sender_state *s, const char *claim_id_str, const char *node_id_str, const char *url);
void stream_receiver_send_node_and_claim_id_to_child(RRDHOST *host);
void stream_sender_clear_parent_claim_id(RRDHOST *host);

void stream_sender_send_claimed_id(RRDHOST *host);

void stream_send_global_functions(RRDHOST *host);
void stream_send_host_labels(RRDHOST *host);

void stream_sender_send_custom_host_variables(RRDHOST *host);
void stream_sender_send_this_host_variable_now(RRDHOST *host, const RRDVAR_ACQUIRED *rva);

bool stream_sender_send_rrdset_definition(BUFFER *wb, RRDSET *st);
bool stream_sender_send_rrdset_definition_now(RRDSET *st);
bool should_send_rrdset_matching(RRDSET *st, RRDSET_FLAGS flags);

void stream_send_rrdset_metrics_v1(RRDSET_STREAM_BUFFER *rsb, RRDSET *st);
void stream_send_rrddim_metrics_v2(RRDSET_STREAM_BUFFER *rsb, RRDDIM *rd, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags);
void stream_send_rrdset_metrics_finished(RRDSET_STREAM_BUFFER *rsb, RRDSET *st);

#endif //NETDATA_STREAMING_PROTCOL_COMMANDS_H
