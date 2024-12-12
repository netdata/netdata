// SPDX-License-Identifier: GPL-3.0-or-later

#define STREAM_INTERNALS
#include "stream-waiting-list.h"

#define ACCEPT_NODES_EVERY_UT (5 * USEC_PER_SEC)

static __thread struct {
    size_t metadata;
    size_t replication;
} throttle = { 0 };

void stream_thread_received_metadata(void) {
    throttle.metadata++;
}
void stream_thread_received_replication(void) {
    throttle.replication++;
}

static inline size_t normalize_value(size_t v) {
    return (v / 100) * 100;
}

void stream_thread_process_waiting_list_unsafe(struct stream_thread *sth, usec_t now_ut) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    Word_t idx = 0;
    struct receiver_state *rpt = RECEIVERS_FIRST(&sth->queue.receivers, &idx);
    if(!rpt) return;

    if(sth->waiting_list.last_accepted_ut + ACCEPT_NODES_EVERY_UT > now_ut ||
        !stream_control_children_should_be_accepted())
        return;

    size_t n_metadata = normalize_value(throttle.metadata);
    size_t n_replication = normalize_value(throttle.replication);

    if(sth->waiting_list.metadata != n_metadata ||
        sth->waiting_list.replication != n_replication) {
        sth->waiting_list.metadata = n_metadata;
        sth->waiting_list.replication = n_replication;
        return;
    }

    RECEIVERS_DEL(&sth->queue.receivers, idx);
    stream_receiver_move_to_running_unsafe(sth, rpt);
    sth->queue.receivers_waiting--;
}
