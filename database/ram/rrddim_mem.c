// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrddim_mem.h"

// ----------------------------------------------------------------------------
// RRDDIM legacy data collection functions

void rrddim_collect_init(RRDDIM *rd) {
    rd->values[rd->rrdset->current_entry] = SN_EMPTY_SLOT;
    rd->state->handle = calloc(1, sizeof(struct mem_collect_handle));
}
void rrddim_collect_store_metric(RRDDIM *rd, usec_t point_in_time, storage_number number) {
    (void)point_in_time;
    rd->values[rd->rrdset->current_entry] = number;
}
int rrddim_collect_finalize(RRDDIM *rd) {
    free((struct mem_collect_handle*)rd->state->handle);
    return 0;
}

// ----------------------------------------------------------------------------
// RRDDIM legacy database query functions

void rrddim_query_init(RRDDIM *rd, struct rrddim_query_handle *handle, time_t start_time, time_t end_time) {
    handle->rd = rd;
    handle->start_time = start_time;
    handle->end_time = end_time;
    struct mem_query_handle* h = calloc(1, sizeof(struct mem_query_handle));
    h->slot = rrdset_time2slot(rd->rrdset, start_time);
    h->slot_timestamp = rrdset_slot2time(rd->rrdset, h->slot);
    h->last_slot = rrdset_time2slot(rd->rrdset, end_time);
    h->dt = rd->update_every;
    h->finished = 0;
    handle->handle = (STORAGE_QUERY_HANDLE *)h;
}

storage_number rrddim_query_next_metric(struct rrddim_query_handle *handle, time_t *current_time) {
    RRDDIM *rd = handle->rd;
    struct mem_query_handle* h = (struct mem_query_handle*)handle->handle;
    long entries = rd->rrdset->entries;
    long slot = h->slot;

    if(unlikely(h->finished || h->slot_timestamp > *current_time))
        return SN_EMPTY_SLOT;

    if (unlikely(h->slot == h->last_slot))
        h->finished = 1;

    storage_number n = rd->values[slot++];

    if(unlikely(slot >= entries)) slot = 0;

    h->slot = slot;
    h->slot_timestamp += h->dt;

    return n;
}

int rrddim_query_is_finished(struct rrddim_query_handle *handle) {
    struct mem_query_handle* h = (struct mem_query_handle*)handle->handle;
    return h->finished;
}

void rrddim_query_finalize(struct rrddim_query_handle *handle) {
    freez(handle->handle);
}

time_t rrddim_query_latest_time(RRDDIM *rd) {
    return rrdset_last_entry_t_nolock(rd->rrdset);
}

time_t rrddim_query_oldest_time(RRDDIM *rd) {
    return rrdset_first_entry_t_nolock(rd->rrdset);
}
