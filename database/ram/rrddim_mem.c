// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrddim_mem.h"

// ----------------------------------------------------------------------------
// RRDDIM legacy data collection functions

STORAGE_METRIC_HANDLE *rrddim_metric_init(RRDDIM *rd, STORAGE_INSTANCE *db_instance __maybe_unused) {
    return (STORAGE_METRIC_HANDLE *)rd;
}

void rrddim_metric_free(STORAGE_METRIC_HANDLE *db_metric_handle __maybe_unused) {
    ;
}

STORAGE_COLLECT_HANDLE *rrddim_collect_init(STORAGE_METRIC_HANDLE *db_metric_handle) {
    RRDDIM *rd = (RRDDIM *)db_metric_handle;
    rd->db[rd->rrdset->current_entry] = SN_EMPTY_SLOT;
    struct mem_collect_handle *ch = calloc(1, sizeof(struct mem_collect_handle));
    ch->rd = rd;
    return (STORAGE_COLLECT_HANDLE *)ch;
}

void rrddim_collect_store_metric(STORAGE_COLLECT_HANDLE *collection_handle, usec_t point_in_time, NETDATA_DOUBLE number,
        NETDATA_DOUBLE min_value,
        NETDATA_DOUBLE max_value,
        uint16_t count,
        uint16_t anomaly_count,
        SN_FLAGS flags)
{
    UNUSED(point_in_time);
    UNUSED(min_value);
    UNUSED(max_value);
    UNUSED(count);
    UNUSED(anomaly_count);

    struct mem_collect_handle *ch = (struct mem_collect_handle *)collection_handle;
    RRDDIM *rd = ch->rd;
    rd->db[rd->rrdset->current_entry] = pack_storage_number(number, flags);
}

void rrddim_store_metric_flush(STORAGE_COLLECT_HANDLE *collection_handle) {
    struct mem_collect_handle *ch = (struct mem_collect_handle *)collection_handle;
    RRDDIM *rd = ch->rd;
    memset(rd->db, 0, rd->entries * sizeof(storage_number));
}

int rrddim_collect_finalize(STORAGE_COLLECT_HANDLE *collection_handle) {
    free(collection_handle);
    return 0;
}

// ----------------------------------------------------------------------------
// RRDDIM legacy database query functions

void rrddim_query_init(STORAGE_METRIC_HANDLE *db_metric_handle, struct rrddim_query_handle *handle, time_t start_time, time_t end_time, TIER_QUERY_FETCH tier_query_fetch_type) {
    UNUSED(tier_query_fetch_type);

    RRDDIM *rd = (RRDDIM *)db_metric_handle;

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

    // info("RRDDIM QUERY INIT: start %ld, end %ld, next %ld, first %ld, last %ld, dt %ld", start_time, end_time, h->next_timestamp, h->slot_timestamp, h->last_timestamp, h->dt);

    handle->handle = (STORAGE_QUERY_HANDLE *)h;
}

// Returns the metric and sets its timestamp into current_time
// IT IS REQUIRED TO **ALWAYS** SET ALL RETURN VALUES (current_time, end_time, flags)
// IT IS REQUIRED TO **ALWAYS** KEEP TRACK OF TIME, EVEN OUTSIDE THE DATABASE BOUNDARIES
NETDATA_DOUBLE rrddim_query_next_metric(struct rrddim_query_handle *handle, time_t *start_time, time_t *end_time, SN_FLAGS *flags, uint16_t *count, uint16_t *anomaly_count) {
    RRDDIM *rd = handle->rd;
    struct mem_query_handle* h = (struct mem_query_handle*)handle->handle;
    size_t entries = rd->rrdset->entries;
    size_t slot = h->slot;
    *count = 1;

    time_t this_timestamp = h->next_timestamp;
    h->next_timestamp += h->dt;

    // set this timestamp for our caller
    *start_time = this_timestamp - h->dt;
    *end_time = this_timestamp;

    if(unlikely(this_timestamp < h->slot_timestamp)) {
        *flags = SN_EMPTY_SLOT;
        *anomaly_count = 0;
        return NAN;
    }

    if(unlikely(this_timestamp > h->last_timestamp)) {
        *flags = SN_EMPTY_SLOT;
        *anomaly_count = 0;
        return NAN;
    }

    storage_number n = rd->db[slot++];
    if(unlikely(slot >= entries)) slot = 0;

    h->slot = slot;
    h->slot_timestamp += h->dt;

    *anomaly_count = (!(n & SN_ANOMALY_BIT));
    *flags = (n & SN_ALL_FLAGS);
    return unpack_storage_number(n);
}

int rrddim_query_is_finished(struct rrddim_query_handle *handle) {
    struct mem_query_handle* h = (struct mem_query_handle*)handle->handle;
    return (h->next_timestamp > handle->end_time);
}

void rrddim_query_finalize(struct rrddim_query_handle *handle) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(!rrddim_query_is_finished(handle))
        error("QUERY: query for chart '%s' dimension '%s' has been stopped unfinished", handle->rd->rrdset->id, handle->rd->name);
#endif
    freez(handle->handle);
}

time_t rrddim_query_latest_time(STORAGE_METRIC_HANDLE *db_metric_handle) {
    RRDDIM *rd = (RRDDIM *)db_metric_handle;
    return rd->rrdset->last_updated.tv_sec;
}

time_t rrddim_query_oldest_time(STORAGE_METRIC_HANDLE *db_metric_handle) {
    RRDDIM *rd = (RRDDIM *)db_metric_handle;
    return (time_t)(rd->rrdset->last_updated.tv_sec - rrdset_duration(rd->rrdset));
}
