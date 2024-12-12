// SPDX-License-Identifier: GPL-3.0-or-later

#define STREAM_INTERNALS
#include "stream-waiting-list.h"

// the thread calls us every 100ms
#define ITERATIONS_TO_GET_ONE 20

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

void stream_thread_process_waiting_list_unsafe(struct stream_thread *sth) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    Word_t idx = 0;
    struct receiver_state *rpt = RECEIVERS_FIRST(&sth->queue.receivers, &idx);
    if(!rpt) return;

    size_t n_metadata = normalize_value(throttle.metadata);
    size_t n_replication = normalize_value(throttle.replication);

    if(sth->waiting_list.metadata == n_metadata && sth->waiting_list.replication == n_replication) {
        if(sth->waiting_list.decrement-- == 0) {

            if(stream_control_children_should_be_accepted()) {
                RECEIVERS_DEL(&sth->queue.receivers, idx);
                stream_receiver_move_to_running_unsafe(sth, rpt);
                sth->queue.receivers_waiting--;
                sth->waiting_list.decrement = ITERATIONS_TO_GET_ONE;
            }
            else
                sth->waiting_list.decrement = 1;
        }
    }
    else
        sth->waiting_list.decrement = ITERATIONS_TO_GET_ONE;

    sth->waiting_list.metadata = n_metadata;
    sth->waiting_list.replication = n_replication;
}
