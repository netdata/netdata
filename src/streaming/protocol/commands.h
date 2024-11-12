// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAMING_PROTCOL_COMMANDS_H
#define NETDATA_STREAMING_PROTCOL_COMMANDS_H

#include "database/rrd.h"
#include "../rrdpush.h"

typedef struct rrdset_stream_buffer {
    STREAM_CAPABILITIES capabilities;
    bool v2;
    bool begin_v2_added;
    time_t wall_clock_time;
    RRDSET_FLAGS rrdset_flags;
    time_t last_point_end_time_s;
    BUFFER *wb;
} RRDSET_STREAM_BUFFER;

RRDSET_STREAM_BUFFER rrdset_push_metric_initialize(RRDSET *st, time_t wall_clock_time);

void rrdpush_sender_get_node_and_claim_id_from_parent(struct sender_state *s);
void rrdpush_receiver_send_node_and_claim_id_to_child(RRDHOST *host);
void rrdpush_sender_clear_parent_claim_id(RRDHOST *host);

void rrdpush_sender_send_claimed_id(RRDHOST *host);

void rrdpush_send_global_functions(RRDHOST *host);
void rrdpush_send_host_labels(RRDHOST *host);

void rrdpush_sender_thread_send_custom_host_variables(RRDHOST *host);
void rrdpush_sender_send_this_host_variable_now(RRDHOST *host, const RRDVAR_ACQUIRED *rva);

bool rrdpush_chart_definition_to_pluginsd(BUFFER *wb, RRDSET *st);
bool rrdset_push_chart_definition_now(RRDSET *st);
bool should_send_chart_matching(RRDSET *st, RRDSET_FLAGS flags);

void rrdset_push_metrics_v1(RRDSET_STREAM_BUFFER *rsb, RRDSET *st);
void rrddim_push_metrics_v2(RRDSET_STREAM_BUFFER *rsb, RRDDIM *rd, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags);
void rrdset_push_metrics_finished(RRDSET_STREAM_BUFFER *rsb, RRDSET *st);

#endif //NETDATA_STREAMING_PROTCOL_COMMANDS_H
