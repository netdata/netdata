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

struct mem_metric_handle {
    RRDDIM *rd;

    size_t counter;
    size_t entries;
    size_t current_entry;
    time_t last_updated_s;
    time_t update_every_s;

    int32_t refcount;
};

static void update_metric_handle_from_rrddim(struct mem_metric_handle *mh, RRDDIM *rd) {
    mh->counter        = rd->rrdset->counter;
    mh->entries        = rd->rrdset->entries;
    mh->current_entry  = rd->rrdset->current_entry;
    mh->last_updated_s = rd->rrdset->last_updated.tv_sec;
    mh->update_every_s = rd->rrdset->update_every;
}

static void check_metric_handle_from_rrddim(struct mem_metric_handle *mh) {
    RRDDIM *rd = mh->rd; (void)rd;
    internal_fatal(mh->entries != (size_t)rd->rrdset->entries, "RRDDIM: entries do not match");
    internal_fatal(mh->update_every_s != rd->rrdset->update_every, "RRDDIM: update every does not match");
}

STORAGE_METRIC_HANDLE *
rrddim_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *db_instance __maybe_unused) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)rrddim_metric_get(db_instance, &rd->metric_uuid);
    while(!mh) {
        netdata_rwlock_wrlock(&rrddim_JudyHS_rwlock);
        Pvoid_t *PValue = JudyHSIns(&rrddim_JudyHS_array, &rd->metric_uuid, sizeof(uuid_t), PJE0);
        mh = *PValue;
        if(!mh) {
            mh = callocz(1, sizeof(struct mem_metric_handle));
            mh->rd = rd;
            mh->refcount = 1;
            update_metric_handle_from_rrddim(mh, rd);
            *PValue = mh;
            __atomic_add_fetch(&rrddim_db_memory_size, sizeof(struct mem_metric_handle) + JUDYHS_INDEX_SIZE_ESTIMATE(sizeof(uuid_t)), __ATOMIC_RELAXED);
        }
        else {
            if(__atomic_add_fetch(&mh->refcount, 1, __ATOMIC_RELAXED) <= 0)
                mh = NULL;
        }
        netdata_rwlock_unlock(&rrddim_JudyHS_rwlock);
    }

    internal_fatal(mh->rd != rd, "RRDDIM_MEM: incorrect pointer returned from index.");

    return (STORAGE_METRIC_HANDLE *)mh;
}

STORAGE_METRIC_HANDLE *
rrddim_metric_get(STORAGE_INSTANCE *db_instance __maybe_unused, uuid_t *uuid) {
    struct mem_metric_handle *mh = NULL;
    netdata_rwlock_rdlock(&rrddim_JudyHS_rwlock);
    Pvoid_t *PValue = JudyHSGet(rrddim_JudyHS_array, uuid, sizeof(uuid_t));
    if (likely(NULL != PValue)) {
        mh = *PValue;
        if(__atomic_add_fetch(&mh->refcount, 1, __ATOMIC_RELAXED) <= 0)
            mh = NULL;
    }
    netdata_rwlock_unlock(&rrddim_JudyHS_rwlock);

    return (STORAGE_METRIC_HANDLE *)mh;
}

STORAGE_METRIC_HANDLE *rrddim_metric_dup(STORAGE_METRIC_HANDLE *db_metric_handle) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)db_metric_handle;
    __atomic_add_fetch(&mh->refcount, 1, __ATOMIC_RELAXED);
    return db_metric_handle;
}

void rrddim_metric_release(STORAGE_METRIC_HANDLE *db_metric_handle __maybe_unused) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)db_metric_handle;

    if(__atomic_sub_fetch(&mh->refcount, 1, __ATOMIC_RELAXED) == 0) {
        // we are the last one holding this

        int32_t expected = 0;
        if(__atomic_compare_exchange_n(&mh->refcount, &expected, -99999, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED)) {
            // we can delete it

            RRDDIM *rd = mh->rd;
            netdata_rwlock_wrlock(&rrddim_JudyHS_rwlock);
            JudyHSDel(&rrddim_JudyHS_array, &rd->metric_uuid, sizeof(uuid_t), PJE0);
            netdata_rwlock_unlock(&rrddim_JudyHS_rwlock);

            freez(mh);
            __atomic_sub_fetch(&rrddim_db_memory_size, sizeof(struct mem_metric_handle) + JUDYHS_INDEX_SIZE_ESTIMATE(sizeof(uuid_t)), __ATOMIC_RELAXED);
        }
    }
}

bool rrddim_metric_retention_by_uuid(STORAGE_INSTANCE *db_instance __maybe_unused, uuid_t *uuid, time_t *first_entry_s, time_t *last_entry_s) {
    STORAGE_METRIC_HANDLE *db_metric_handle = rrddim_metric_get(db_instance, uuid);
    if(!db_metric_handle)
        return false;

    *first_entry_s = rrddim_query_oldest_time_s(db_metric_handle);
    *last_entry_s = rrddim_query_latest_time_s(db_metric_handle);

    return true;
}

void rrddim_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *collection_handle, int update_every) {
    struct mem_collect_handle *ch = (struct mem_collect_handle *)collection_handle;
    struct mem_metric_handle *mh = (struct mem_metric_handle *)ch->db_metric_handle;

    rrddim_store_metric_flush(collection_handle);
    mh->update_every_s = update_every;
}

STORAGE_COLLECT_HANDLE *rrddim_collect_init(STORAGE_METRIC_HANDLE *db_metric_handle, uint32_t update_every __maybe_unused, STORAGE_METRICS_GROUP *smg __maybe_unused) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)db_metric_handle;
    RRDDIM *rd = mh->rd;

    update_metric_handle_from_rrddim(mh, rd);
    internal_fatal((uint32_t)mh->update_every_s != update_every, "RRDDIM: update requested does not match the dimension");

    struct mem_collect_handle *ch = callocz(1, sizeof(struct mem_collect_handle));
    ch->common.backend = STORAGE_ENGINE_BACKEND_RRDDIM;
    ch->rd = rd;
    ch->db_metric_handle = db_metric_handle;

    __atomic_add_fetch(&rrddim_db_memory_size, sizeof(struct mem_collect_handle), __ATOMIC_RELAXED);

    return (STORAGE_COLLECT_HANDLE *)ch;
}

void rrddim_store_metric_flush(STORAGE_COLLECT_HANDLE *collection_handle) {
    struct mem_collect_handle *ch = (struct mem_collect_handle *)collection_handle;
    struct mem_metric_handle *mh = (struct mem_metric_handle *)ch->db_metric_handle;

    RRDDIM *rd = mh->rd;
    size_t entries = mh->entries;
    storage_number empty = pack_storage_number(NAN, SN_FLAG_NONE);

    for(size_t i = 0; i < entries ;i++)
        rd->db[i] = empty;

    mh->counter = 0;
    mh->last_updated_s = 0;
    mh->current_entry = 0;
}

static inline void rrddim_fill_the_gap(STORAGE_COLLECT_HANDLE *collection_handle, time_t now_collect_s) {
    struct mem_collect_handle *ch = (struct mem_collect_handle *)collection_handle;
    struct mem_metric_handle *mh = (struct mem_metric_handle *)ch->db_metric_handle;

    RRDDIM *rd = mh->rd;

    internal_fatal(ch->rd != mh->rd, "RRDDIM: dimensions do not match");
    check_metric_handle_from_rrddim(mh);

    size_t entries = mh->entries;
    time_t update_every_s = mh->update_every_s;
    time_t last_stored_s = mh->last_updated_s;
    size_t gap_entries = (now_collect_s - last_stored_s) / update_every_s;
    if(gap_entries >= entries)
        rrddim_store_metric_flush(collection_handle);

    else {
        storage_number empty = pack_storage_number(NAN, SN_FLAG_NONE);
        size_t current_entry = mh->current_entry;
        time_t now_store_s = last_stored_s + update_every_s;

        // fill the dimension
        size_t c;
        for(c = 0; c < entries && now_store_s <= now_collect_s ; now_store_s += update_every_s, c++) {
            rd->db[current_entry++] = empty;

            if(unlikely(current_entry >= entries))
                current_entry = 0;
        }
        mh->counter += c;
        mh->current_entry = current_entry;
        mh->last_updated_s = now_store_s;
    }
}

void rrddim_collect_store_metric(STORAGE_COLLECT_HANDLE *collection_handle,
                                 usec_t point_in_time_ut,
                                 NETDATA_DOUBLE n,
                                 NETDATA_DOUBLE min_value __maybe_unused,
                                 NETDATA_DOUBLE max_value __maybe_unused,
                                 uint16_t count __maybe_unused,
                                 uint16_t anomaly_count __maybe_unused,
                                 SN_FLAGS flags)
{
    struct mem_collect_handle *ch = (struct mem_collect_handle *)collection_handle;
    struct mem_metric_handle *mh = (struct mem_metric_handle *)ch->db_metric_handle;

    RRDDIM *rd = ch->rd;
    time_t point_in_time_s = (time_t)(point_in_time_ut / USEC_PER_SEC);

    internal_fatal(ch->rd != mh->rd, "RRDDIM: dimensions do not match");
    check_metric_handle_from_rrddim(mh);

    if(unlikely(point_in_time_s <= mh->last_updated_s))
        return;

    if(unlikely(mh->last_updated_s && point_in_time_s - mh->update_every_s > mh->last_updated_s))
        rrddim_fill_the_gap(collection_handle, point_in_time_s);

    rd->db[mh->current_entry] = pack_storage_number(n, flags);
    mh->counter++;
    mh->current_entry = (mh->current_entry + 1) >= mh->entries ? 0 : mh->current_entry + 1;
    mh->last_updated_s = point_in_time_s;
}

int rrddim_collect_finalize(STORAGE_COLLECT_HANDLE *collection_handle) {
    freez(collection_handle);
    __atomic_sub_fetch(&rrddim_db_memory_size, sizeof(struct mem_collect_handle), __ATOMIC_RELAXED);
    return 0;
}

// ----------------------------------------------------------------------------

// get the total duration in seconds of the round-robin database
#define metric_duration(mh) (( (time_t)(mh)->counter >= (time_t)(mh)->entries ? (time_t)(mh)->entries : (time_t)(mh)->counter ) * (time_t)(mh)->update_every_s)

// get the last slot updated in the round-robin database
#define rrddim_last_slot(mh) ((size_t)(((mh)->current_entry == 0) ? (mh)->entries - 1 : (mh)->current_entry - 1))

// return the slot that has the oldest value
#define rrddim_first_slot(mh) ((size_t)((mh)->counter >= (size_t)(mh)->entries ? (mh)->current_entry : 0))

// get the slot of the round-robin database, for the given timestamp (t)
// it always returns a valid slot, although it may not be for the time requested if the time is outside the round-robin database
// only valid when not using dbengine
static inline size_t rrddim_time2slot(STORAGE_METRIC_HANDLE *db_metric_handle, time_t t) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)db_metric_handle;
    RRDDIM *rd = mh->rd;

    size_t ret = 0;
    time_t last_entry_s  = rrddim_query_latest_time_s(db_metric_handle);
    time_t first_entry_s = rrddim_query_oldest_time_s(db_metric_handle);
    size_t entries       = mh->entries;
    size_t first_slot    = rrddim_first_slot(mh);
    size_t last_slot     = rrddim_last_slot(mh);
    size_t update_every  = mh->update_every_s;

    if(t >= last_entry_s) {
        // the requested time is after the last entry we have
        ret = last_slot;
    }
    else {
        if(t <= first_entry_s) {
            // the requested time is before the first entry we have
            ret = first_slot;
        }
        else {
            if(last_slot >= (size_t)((last_entry_s - t) / update_every))
                ret = last_slot - ((last_entry_s - t) / update_every);
            else
                ret = last_slot - ((last_entry_s - t) / update_every) + entries;
        }
    }

    if(unlikely(ret >= entries)) {
        error("INTERNAL ERROR: rrddim_time2slot() on %s returns values outside entries", rrddim_name(rd));
        ret = entries - 1;
    }

    return ret;
}

// get the timestamp of a specific slot in the round-robin database
// only valid when not using dbengine
static inline time_t rrddim_slot2time(STORAGE_METRIC_HANDLE *db_metric_handle, size_t slot) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)db_metric_handle;
    RRDDIM *rd = mh->rd;

    time_t ret;
    time_t last_entry_s  = rrddim_query_latest_time_s(db_metric_handle);
    time_t first_entry_s = rrddim_query_oldest_time_s(db_metric_handle);
    size_t entries       = mh->entries;
    size_t last_slot     = rrddim_last_slot(mh);
    size_t update_every  = mh->update_every_s;

    if(slot >= entries) {
        error("INTERNAL ERROR: caller of rrddim_slot2time() gives invalid slot %zu", slot);
        slot = entries - 1;
    }

    if(slot > last_slot)
        ret = last_entry_s - (time_t)(update_every * (last_slot - slot + entries));
    else
        ret = last_entry_s - (time_t)(update_every * (last_slot - slot));

    if(unlikely(ret < first_entry_s)) {
        error("INTERNAL ERROR: rrddim_slot2time() on dimension '%s' of chart '%s' returned time (%ld) too far in the past (before first_entry_s %ld) for slot %zu",
              rrddim_name(rd), rrdset_id(rd->rrdset), ret, first_entry_s, slot);

        ret = first_entry_s;
    }

    if(unlikely(ret > last_entry_s)) {
        error("INTERNAL ERROR: rrddim_slot2time() on dimension '%s' of chart '%s' returned time (%ld) too far into the future (after last_entry_s %ld) for slot %zu",
              rrddim_name(rd), rrdset_id(rd->rrdset), ret, last_entry_s, slot);

        ret = last_entry_s;
    }

    return ret;
}

// ----------------------------------------------------------------------------
// RRDDIM legacy database query functions

void rrddim_query_init(STORAGE_METRIC_HANDLE *db_metric_handle, struct storage_engine_query_handle *handle, time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority __maybe_unused) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)db_metric_handle;

    check_metric_handle_from_rrddim(mh);

    handle->start_time_s = start_time_s;
    handle->end_time_s = end_time_s;
    handle->priority = priority;
    handle->backend = STORAGE_ENGINE_BACKEND_RRDDIM;
    struct mem_query_handle* h = mallocz(sizeof(struct mem_query_handle));
    h->db_metric_handle = db_metric_handle;

    h->slot           = rrddim_time2slot(db_metric_handle, start_time_s);
    h->last_slot      = rrddim_time2slot(db_metric_handle, end_time_s);
    h->dt             = mh->update_every_s;

    h->next_timestamp = start_time_s;
    h->slot_timestamp = rrddim_slot2time(db_metric_handle, h->slot);
    h->last_timestamp = rrddim_slot2time(db_metric_handle, h->last_slot);

    // info("RRDDIM QUERY INIT: start %ld, end %ld, next %ld, first %ld, last %ld, dt %ld", start_time, end_time, h->next_timestamp, h->slot_timestamp, h->last_timestamp, h->dt);

    __atomic_add_fetch(&rrddim_db_memory_size, sizeof(struct mem_query_handle), __ATOMIC_RELAXED);
    handle->handle = (STORAGE_QUERY_HANDLE *)h;
}

// Returns the metric and sets its timestamp into current_time
// IT IS REQUIRED TO **ALWAYS** SET ALL RETURN VALUES (current_time, end_time, flags)
// IT IS REQUIRED TO **ALWAYS** KEEP TRACK OF TIME, EVEN OUTSIDE THE DATABASE BOUNDARIES
STORAGE_POINT rrddim_query_next_metric(struct storage_engine_query_handle *handle) {
    struct mem_query_handle* h = (struct mem_query_handle*)handle->handle;
    struct mem_metric_handle *mh = (struct mem_metric_handle *)h->db_metric_handle;
    RRDDIM *rd = mh->rd;

    size_t entries = mh->entries;
    size_t slot = h->slot;

    STORAGE_POINT sp;
    sp.count = 1;

    time_t this_timestamp = h->next_timestamp;
    h->next_timestamp += h->dt;

    // set this timestamp for our caller
    sp.start_time_s = this_timestamp - h->dt;
    sp.end_time_s = this_timestamp;

    if(unlikely(this_timestamp < h->slot_timestamp)) {
        storage_point_empty(sp, sp.start_time_s, sp.end_time_s);
        return sp;
    }

    if(unlikely(this_timestamp > h->last_timestamp)) {
        storage_point_empty(sp, sp.start_time_s, sp.end_time_s);
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
    struct mem_query_handle *h = (struct mem_query_handle*)handle->handle;
    return (h->next_timestamp > handle->end_time_s);
}

void rrddim_query_finalize(struct storage_engine_query_handle *handle) {
#ifdef NETDATA_INTERNAL_CHECKS
    struct mem_query_handle *h = (struct mem_query_handle*)handle->handle;
    struct mem_metric_handle *mh = (struct mem_metric_handle *)h->db_metric_handle;

    internal_error(!rrddim_query_is_finished(handle),
                   "QUERY: query for chart '%s' dimension '%s' has been stopped unfinished",
                   rrdset_id(mh->rd->rrdset), rrddim_name(mh->rd));

#endif
    freez(handle->handle);
    __atomic_sub_fetch(&rrddim_db_memory_size, sizeof(struct mem_query_handle), __ATOMIC_RELAXED);
}

time_t rrddim_query_align_to_optimal_before(struct storage_engine_query_handle *rrddim_handle) {
    return rrddim_handle->end_time_s;
}

time_t rrddim_query_latest_time_s(STORAGE_METRIC_HANDLE *db_metric_handle) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)db_metric_handle;
    return mh->last_updated_s;
}

time_t rrddim_query_oldest_time_s(STORAGE_METRIC_HANDLE *db_metric_handle) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)db_metric_handle;
    return (time_t)(mh->last_updated_s - metric_duration(mh));
}
