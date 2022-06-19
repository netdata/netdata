// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrddim_mem.h"

// ----------------------------------------------------------------------------
// RRDDIM legacy data collection functions

void rrddim_collect_init(RRDDIM *rd) {
    rd->values[rd->rrdset->current_entry] = SN_EMPTY_SLOT;
    rd->state->handle = calloc(1, sizeof(struct mem_collect_handle));
}
void rrddim_collect_store_metric(RRDDIM *rd, usec_t point_in_time, calculated_number number, SN_FLAGS flags) {
    (void)point_in_time;
    rd->values[rd->rrdset->current_entry] = pack_storage_number(number, flags);
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
    h->last_slot = rrdset_time2slot(rd->rrdset, end_time);
    h->dt = rd->update_every;

    h->next_timestamp = start_time;
    h->slot_timestamp = rrdset_slot2time(rd->rrdset, h->slot);
    h->last_timestamp = rrdset_slot2time(rd->rrdset, h->last_slot);

    // info("QUERY: start %ld, end %ld, next %ld, first %ld, last %ld", start_time, end_time, h->next_timestamp, h->slot_timestamp, h->last_timestamp);

    handle->handle = (STORAGE_QUERY_HANDLE *)h;
}

calculated_number rrddim_query_next_metric(struct rrddim_query_handle *handle, time_t *current_time, time_t *end_time, SN_FLAGS *flags) {
    RRDDIM *rd = handle->rd;
    struct mem_query_handle* h = (struct mem_query_handle*)handle->handle;
    size_t entries = rd->rrdset->entries;
    size_t slot = h->slot;

    time_t this_timestamp = h->next_timestamp;
    h->next_timestamp += h->dt;

    // set this timestamp for our caller
    *current_time = this_timestamp;
    *end_time = h->next_timestamp;

    if(unlikely(this_timestamp < h->slot_timestamp)) {
        *flags = SN_EMPTY_SLOT;
        return NAN;
    }

    if(unlikely(this_timestamp > h->last_timestamp)) {
        *flags = SN_EMPTY_SLOT;
        return NAN;
    }

    storage_number n = rd->values[slot++];
    if(unlikely(slot >= entries)) slot = 0;

    h->slot = slot;
    h->slot_timestamp += h->dt;

    *flags = (n & SN_ALL_FLAGS);
    return unpack_storage_number(n);
}

int rrddim_query_is_finished(struct rrddim_query_handle *handle) {
    struct mem_query_handle* h = (struct mem_query_handle*)handle->handle;
    return (h->next_timestamp >= handle->end_time);
}

void rrddim_query_finalize(struct rrddim_query_handle *handle) {
#ifdef NETDATA_INTERNAL_CHECKS
    struct mem_query_handle* h = (struct mem_query_handle*)handle->handle;
    if(!rrddim_query_is_finished(handle))
        error("QUERY: query for chart '%s' dimension '%s' has been stopped unfinished", handle->rd->rrdset->id, handle->rd->name);
#endif
    freez(handle->handle);
}

time_t rrddim_query_latest_time(RRDDIM *rd) {
    return rrdset_last_entry_t_nolock(rd->rrdset);
}

time_t rrddim_query_oldest_time(RRDDIM *rd) {
    return rrdset_first_entry_t_nolock(rd->rrdset);
}
