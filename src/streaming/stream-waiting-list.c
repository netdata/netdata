// SPDX-License-Identifier: GPL-3.0-or-later

#define STREAM_INTERNALS
#include "stream-waiting-list.h"

// the thread calls us every 10ms, so we wait for 10 iterations (1 second)
#define ITERATIONS_TO_GET_ONE 10

static __thread struct {
    size_t metadata;
} throttle = { 0 };

void stream_thread_received_metadata(void) {
    throttle.metadata++;
}

void stream_thread_process_waiting_list_unsafe(struct stream_thread *sth) {
    internal_fatal(sth->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    Word_t idx = 0;
    struct receiver_state *rpt = RECEIVERS_FIRST(&sth->queue.receivers, &idx);
    if(!rpt) return;

    if(sth->waiting_list.metadata == throttle.metadata) {
        if(sth->waiting_list.decrement-- == 0) {
            sth->waiting_list.decrement = ITERATIONS_TO_GET_ONE;

            if(stream_control_children_should_be_accepted()) {
                RECEIVERS_DEL(&sth->queue.receivers, idx);
                stream_receiver_move_to_running_unsafe(sth, rpt);
                sth->queue.receivers_waiting--;
            }
        }
    }
    else {
        sth->waiting_list.metadata = throttle.metadata;
        sth->waiting_list.decrement = ITERATIONS_TO_GET_ONE;
    }
}
