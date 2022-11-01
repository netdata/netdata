// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrddim_mem.h"
#include "Judy.h"

static Pvoid_t rrddim_JudyHS_array = NULL;
static netdata_rwlock_t rrddim_JudyHS_rwlock = NETDATA_RWLOCK_INITIALIZER;

// ----------------------------------------------------------------------------
// metrics groups

STORAGE_METRICS_GROUP *rrddim_metrics_group_get(STORAGE_INSTANCE *db_instance __maybe_unused, uuid_t *uuid __maybe_unused) {
    return NULL;
}

void rrddim_metrics_group_release(STORAGE_INSTANCE *db_instance __maybe_unused, STORAGE_METRICS_GROUP *smg __maybe_unused) {
    // if(!smg) return; // smg may be NULL
    ;
}

// ----------------------------------------------------------------------------
// RRDDIM legacy data collection functions

STORAGE_METRIC_HANDLE *
rrddim_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *db_instance __maybe_unused, STORAGE_METRICS_GROUP *smg __maybe_unused) {
    STORAGE_METRIC_HANDLE *t = rrddim_metric_get(db_instance, &rd->metric_uuid, smg);
    if(!t) {
        netdata_rwlock_wrlock(&rrddim_JudyHS_rwlock);
        Pvoid_t *PValue = JudyHSIns(&rrddim_JudyHS_array, &rd->metric_uuid, sizeof(uuid_t), PJE0);
        fatal_assert(NULL == *PValue);
        *PValue = rd;
        t = (STORAGE_METRIC_HANDLE *)rd;
        netdata_rwlock_unlock(&rrddim_JudyHS_rwlock);
    }

    if((RRDDIM *)t != rd)
        fatal("RRDDIM_MEM: incorrect pointer returned from index.");

    return (STORAGE_METRIC_HANDLE *)rd;
}

STORAGE_METRIC_HANDLE *
rrddim_metric_get(STORAGE_INSTANCE *db_instance __maybe_unused, uuid_t *uuid, STORAGE_METRICS_GROUP *smg __maybe_unused) {
    RRDDIM *rd = NULL;
    netdata_rwlock_rdlock(&rrddim_JudyHS_rwlock);
    Pvoid_t *PValue = JudyHSGet(rrddim_JudyHS_array, uuid, sizeof(uuid_t));
    if (likely(NULL != PValue))
        rd = *PValue;
    netdata_rwlock_unlock(&rrddim_JudyHS_rwlock);

    return (STORAGE_METRIC_HANDLE *)rd;
}

STORAGE_METRIC_HANDLE *rrddim_metric_dup(STORAGE_METRIC_HANDLE *db_metric_handle) {
    return db_metric_handle;
}

void rrddim_metric_release(STORAGE_METRIC_HANDLE *db_metric_handle __maybe_unused) {
    RRDDIM *rd = (RRDDIM *)db_metric_handle;

    netdata_rwlock_wrlock(&rrddim_JudyHS_rwlock);
    JudyHSDel(&rrddim_JudyHS_array, &rd->metric_uuid, sizeof(uuid_t), PJE0);
    netdata_rwlock_unlock(&rrddim_JudyHS_rwlock);
}

void rrddim_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *collection_handle, int update_every __maybe_unused) {
    rrddim_store_metric_flush(collection_handle);
}

STORAGE_COLLECT_HANDLE *rrddim_collect_init(STORAGE_METRIC_HANDLE *db_metric_handle, uint32_t update_every __maybe_unused) {
    RRDDIM *rd = (RRDDIM *)db_metric_handle;
    rd->db[rd->rrdset->current_entry] = pack_storage_number(NAN, SN_FLAG_NONE);
    struct mem_collect_handle *ch = callocz(1, sizeof(struct mem_collect_handle));
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
    for(int i = 0; i < rd->rrdset->entries ;i++)
        rd->db[i] = SN_EMPTY_SLOT;

}

int rrddim_collect_finalize(STORAGE_COLLECT_HANDLE *collection_handle) {
    freez(collection_handle);
    return 0;
}

// ----------------------------------------------------------------------------

// get the total duration in seconds of the round robin database
#define rrddim_duration(st) (( (time_t)(rd)->rrdset->counter >= (time_t)(rd)->rrdset->entries ? (time_t)(rd)->rrdset->entries : (time_t)(rd)->rrdset->counter ) * (time_t)(rd)->rrdset->update_every)

// get the last slot updated in the round robin database
#define rrddim_last_slot(rd) ((size_t)(((rd)->rrdset->current_entry == 0) ? (rd)->rrdset->entries - 1 : (rd)->rrdset->current_entry - 1))

// return the slot that has the oldest value
#define rrddim_first_slot(rd) ((size_t)((rd)->rrdset->counter >= (size_t)(rd)->rrdset->entries ? (rd)->rrdset->current_entry : 0))

// get the slot of the round robin database, for the given timestamp (t)
// it always returns a valid slot, although may not be for the time requested if the time is outside the round robin database
// only valid when not using dbengine
static inline size_t rrddim_time2slot(RRDDIM *rd, time_t t) {
    size_t ret = 0;
    time_t last_entry_t  = rrddim_query_latest_time((STORAGE_METRIC_HANDLE *)rd);
    time_t first_entry_t = rrddim_query_oldest_time((STORAGE_METRIC_HANDLE *)rd);
    size_t entries       = rd->rrdset->entries;
    size_t first_slot    = rrddim_first_slot(rd);
    size_t last_slot     = rrddim_last_slot(rd);
    size_t update_every  = rd->rrdset->update_every;

    if(t >= last_entry_t) {
        // the requested time is after the last entry we have
        ret = last_slot;
    }
    else {
        if(t <= first_entry_t) {
            // the requested time is before the first entry we have
            ret = first_slot;
        }
        else {
            if(last_slot >= (size_t)((last_entry_t - t) / update_every))
                ret = last_slot - ((last_entry_t - t) / update_every);
            else
                ret = last_slot - ((last_entry_t - t) / update_every) + entries;
        }
    }

    if(unlikely(ret >= entries)) {
        error("INTERNAL ERROR: rrddim_time2slot() on %s returns values outside entries", rrddim_name(rd));
        ret = entries - 1;
    }

    return ret;
}

// get the timestamp of a specific slot in the round robin database
// only valid when not using dbengine
static inline time_t rrddim_slot2time(RRDDIM *rd, size_t slot) {
    time_t ret;
    time_t last_entry_t  = rrddim_query_latest_time((STORAGE_METRIC_HANDLE *)rd);
    time_t first_entry_t = rrddim_query_oldest_time((STORAGE_METRIC_HANDLE *)rd);
    size_t entries       = rd->rrdset->entries;
    size_t last_slot     = rrddim_last_slot(rd);
    size_t update_every  = rd->rrdset->update_every;

    if(slot >= entries) {
        error("INTERNAL ERROR: caller of rrddim_slot2time() gives invalid slot %zu", slot);
        slot = entries - 1;
    }

    if(slot > last_slot)
        ret = last_entry_t - (time_t)(update_every * (last_slot - slot + entries));
    else
        ret = last_entry_t - (time_t)(update_every * (last_slot - slot));

    if(unlikely(ret < first_entry_t)) {
        error("INTERNAL ERROR: rrddim_slot2time() on %s returns time too far in the past", rrddim_name(rd));
        ret = first_entry_t;
    }

    if(unlikely(ret > last_entry_t)) {
        error("INTERNAL ERROR: rrddim_slot2time() on %s returns time into the future", rrddim_name(rd));
        ret = last_entry_t;
    }

    return ret;
}

// ----------------------------------------------------------------------------
// RRDDIM legacy database query functions

void rrddim_query_init(STORAGE_METRIC_HANDLE *db_metric_handle, struct storage_engine_query_handle *handle, time_t start_time, time_t end_time) {
    RRDDIM *rd = (RRDDIM *)db_metric_handle;

    handle->rd = rd;
    handle->start_time_s = start_time;
    handle->end_time_s = end_time;
    struct mem_query_handle* h = mallocz(sizeof(struct mem_query_handle));
    h->slot           = rrddim_time2slot(rd, start_time);
    h->last_slot      = rrddim_time2slot(rd, end_time);
    h->dt = rd->rrdset->update_every;

    h->next_timestamp = start_time;
    h->slot_timestamp = rrddim_slot2time(rd, h->slot);
    h->last_timestamp = rrddim_slot2time(rd, h->last_slot);

    // info("RRDDIM QUERY INIT: start %ld, end %ld, next %ld, first %ld, last %ld, dt %ld", start_time, end_time, h->next_timestamp, h->slot_timestamp, h->last_timestamp, h->dt);

    handle->handle = (STORAGE_QUERY_HANDLE *)h;
}

// Returns the metric and sets its timestamp into current_time
// IT IS REQUIRED TO **ALWAYS** SET ALL RETURN VALUES (current_time, end_time, flags)
// IT IS REQUIRED TO **ALWAYS** KEEP TRACK OF TIME, EVEN OUTSIDE THE DATABASE BOUNDARIES
STORAGE_POINT rrddim_query_next_metric(struct storage_engine_query_handle *handle) {
    RRDDIM *rd = handle->rd;
    struct mem_query_handle* h = (struct mem_query_handle*)handle->handle;
    size_t entries = rd->rrdset->entries;
    size_t slot = h->slot;

    STORAGE_POINT sp;
    sp.count = 1;

    time_t this_timestamp = h->next_timestamp;
    h->next_timestamp += h->dt;

    // set this timestamp for our caller
    sp.start_time = this_timestamp - h->dt;
    sp.end_time = this_timestamp;

    if(unlikely(this_timestamp < h->slot_timestamp)) {
        storage_point_empty(sp, sp.start_time, sp.end_time);
        return sp;
    }

    if(unlikely(this_timestamp > h->last_timestamp)) {
        storage_point_empty(sp, sp.start_time, sp.end_time);
        return sp;
    }

    storage_number n = rd->db[slot++];
    if(unlikely(slot >= entries)) slot = 0;

    h->slot = slot;
    h->slot_timestamp += h->dt;

    sp.anomaly_count = is_storage_number_anomalous(n) ? 1 : 0;
    sp.flags = (n & SN_USER_FLAGS);
    sp.min = sp.max = sp.sum = unpack_storage_number(n);

    return sp;
}

int rrddim_query_is_finished(struct storage_engine_query_handle *handle) {
    struct mem_query_handle* h = (struct mem_query_handle*)handle->handle;
    return (h->next_timestamp > handle->end_time_s);
}

void rrddim_query_finalize(struct storage_engine_query_handle *handle) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(!rrddim_query_is_finished(handle))
        error("QUERY: query for chart '%s' dimension '%s' has been stopped unfinished", rrdset_id(handle->rd->rrdset), rrddim_name(handle->rd));
#endif
    freez(handle->handle);
}

time_t rrddim_query_latest_time(STORAGE_METRIC_HANDLE *db_metric_handle) {
    RRDDIM *rd = (RRDDIM *)db_metric_handle;
    return rd->rrdset->last_updated.tv_sec;
}

time_t rrddim_query_oldest_time(STORAGE_METRIC_HANDLE *db_metric_handle) {
    RRDDIM *rd = (RRDDIM *)db_metric_handle;
    return (time_t)(rd->rrdset->last_updated.tv_sec - rrddim_duration(rd));
}
