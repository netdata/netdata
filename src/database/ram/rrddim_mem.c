// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrddim_mem.h"
#include "Judy.h"

static Pvoid_t rrddim_Judy_array = NULL;
static netdata_rwlock_t rrddim_Judy_rwlock;

static void __attribute__((constructor)) init_lock(void) {
    netdata_rwlock_init(&rrddim_Judy_rwlock);
}

static void __attribute__((destructor)) destroy_lock(void) {
    netdata_rwlock_destroy(&rrddim_Judy_rwlock);
}

// ----------------------------------------------------------------------------
// metrics groups

STORAGE_METRICS_GROUP *rrddim_metrics_group_get(STORAGE_INSTANCE *si __maybe_unused, nd_uuid_t *uuid __maybe_unused) {
    return NULL;
}

void rrddim_metrics_group_release(STORAGE_INSTANCE *si __maybe_unused, STORAGE_METRICS_GROUP *smg __maybe_unused) {
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

    REFCOUNT refcount;
};

static void update_metric_handle_from_rrddim(struct mem_metric_handle *mh, RRDDIM *rd) {
    mh->counter        = rd->rrdset->counter;
    mh->entries        = rd->rrdset->db.entries;
    mh->current_entry  = rd->rrdset->db.current_entry;
    mh->last_updated_s = rd->rrdset->last_updated.tv_sec;
    mh->update_every_s = rd->rrdset->update_every;
}

static void check_metric_handle_from_rrddim(struct mem_metric_handle *mh) {
    RRDDIM *rd = mh->rd; (void)rd;
    internal_fatal(mh->entries != (size_t)rd->rrdset->db.entries, "RRDDIM: entries do not match");
    internal_fatal(mh->update_every_s != rd->rrdset->update_every, "RRDDIM: update every does not match");
}

STORAGE_METRIC_HANDLE *rrddim_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *si) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)rrddim_metric_get_by_id(si, rd->uuid);
    while(!mh) {
        netdata_rwlock_wrlock(&rrddim_Judy_rwlock);
        JudyAllocThreadPulseReset();
        Pvoid_t *PValue = JudyLIns(&rrddim_Judy_array, rd->uuid, PJE0);
        int64_t judy_mem = JudyAllocThreadPulseGetAndReset();
        mh = *PValue;
        if(!mh) {
            mh = callocz(1, sizeof(struct mem_metric_handle));
            mh->rd = rd;
            mh->refcount = 1;
            update_metric_handle_from_rrddim(mh, rd);
            *PValue = mh;
            pulse_db_rrd_memory_change(judy_mem + (int64_t)sizeof(struct mem_metric_handle));
        }
        else {
            if(!refcount_acquire(&mh->refcount))
                mh = NULL;
        }
        netdata_rwlock_wrunlock(&rrddim_Judy_rwlock);
    }

    if(unlikely(mh->rd != rd)) {
        // this can happen when the old RRDDIM is being deleted,
        // but the dictionary has not yet run the destructors
        netdata_rwlock_wrlock(&rrddim_Judy_rwlock);
        mh->rd = rd;
        netdata_rwlock_wrunlock(&rrddim_Judy_rwlock);
    }

    return (STORAGE_METRIC_HANDLE *)mh;
}

STORAGE_METRIC_HANDLE *rrddim_metric_get_by_id(STORAGE_INSTANCE *si __maybe_unused, UUIDMAP_ID id) {
    struct mem_metric_handle *mh = NULL;

    netdata_rwlock_rdlock(&rrddim_Judy_rwlock);
    {
        Pvoid_t *PValue = JudyLGet(rrddim_Judy_array, id, PJE0);
        if (unlikely(PValue == PJERR))
            fatal("DB_RAM_ALLOC: corrupted judy array!");

        if (likely(NULL != PValue)) {
            mh = *PValue;
            if (!refcount_acquire(&mh->refcount))
                mh = NULL;
        }
    }
    netdata_rwlock_rdunlock(&rrddim_Judy_rwlock);

    return (STORAGE_METRIC_HANDLE *)mh;
}

STORAGE_METRIC_HANDLE *rrddim_metric_get_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *uuid) {
    UUIDMAP_ID id = uuidmap_create(*uuid);
    STORAGE_METRIC_HANDLE *mh = rrddim_metric_get_by_id(si, id);
    uuidmap_free(id);
    return mh;
}

STORAGE_METRIC_HANDLE *rrddim_metric_dup(STORAGE_METRIC_HANDLE *smh) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)smh;

    if(!refcount_acquire(&mh->refcount))
        fatal("DB_RAM_ALLOC: cannot acquire an already acquired refcount");

    return smh;
}

void rrddim_metric_release(STORAGE_METRIC_HANDLE *smh) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)smh;

    if(refcount_release_and_acquire_for_deletion(&mh->refcount)) {
        // we can delete it

        int64_t judy_mem = 0;
        RRDDIM *rd = mh->rd;
        netdata_rwlock_wrlock(&rrddim_Judy_rwlock);
        {
            JudyAllocThreadPulseReset();
            JudyLDel(&rrddim_Judy_array, rd->uuid, PJE0);
            judy_mem = JudyAllocThreadPulseGetAndReset();
        }
        netdata_rwlock_wrunlock(&rrddim_Judy_rwlock);

        freez(mh);
        pulse_db_rrd_memory_change(judy_mem - (int64_t)sizeof(struct mem_metric_handle));
    }
}

bool rrddim_metric_retention_by_uuid(STORAGE_INSTANCE *si __maybe_unused, nd_uuid_t *uuid, time_t *first_entry_s, time_t *last_entry_s) {
    STORAGE_METRIC_HANDLE *smh = rrddim_metric_get_by_uuid(si, uuid);
    if(!smh)
        return false;

    *first_entry_s = rrddim_query_oldest_time_s(smh);
    *last_entry_s = rrddim_query_latest_time_s(smh);

    return true;
}

bool rrddim_metric_retention_by_id(STORAGE_INSTANCE *si __maybe_unused, UUIDMAP_ID id, time_t *first_entry_s, time_t *last_entry_s) {
    STORAGE_METRIC_HANDLE *smh = rrddim_metric_get_by_id(si, id);
    if(!smh)
        return false;

    *first_entry_s = rrddim_query_oldest_time_s(smh);
    *last_entry_s = rrddim_query_latest_time_s(smh);

    return true;
}

void rrddim_retention_delete_by_id(STORAGE_INSTANCE *si __maybe_unused, UUIDMAP_ID id __maybe_unused) {
    ;
}

void rrddim_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *sch, int update_every) {
    struct mem_collect_handle *ch = (struct mem_collect_handle *)sch;
    struct mem_metric_handle *mh = (struct mem_metric_handle *)ch->smh;

    rrddim_store_metric_flush(sch);
    mh->update_every_s = update_every;
}

STORAGE_COLLECT_HANDLE *rrddim_collect_init(STORAGE_METRIC_HANDLE *smh, uint32_t update_every __maybe_unused, STORAGE_METRICS_GROUP *smg __maybe_unused) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)smh;
    RRDDIM *rd = mh->rd;

    update_metric_handle_from_rrddim(mh, rd);
    internal_fatal((uint32_t)mh->update_every_s != update_every, "RRDDIM: update requested does not match the dimension");

    struct mem_collect_handle *ch = callocz(1, sizeof(struct mem_collect_handle));
    ch->common.seb = STORAGE_ENGINE_BACKEND_RRDDIM;
    ch->rd = rd;
    ch->smh = smh;

    pulse_db_rrd_memory_add(sizeof(struct mem_collect_handle));

    return (STORAGE_COLLECT_HANDLE *)ch;
}

void rrddim_store_metric_flush(STORAGE_COLLECT_HANDLE *sch) {
    struct mem_collect_handle *ch = (struct mem_collect_handle *)sch;
    struct mem_metric_handle *mh = (struct mem_metric_handle *)ch->smh;

    RRDDIM *rd = mh->rd;
    size_t entries = mh->entries;
    storage_number empty = pack_storage_number(NAN, SN_FLAG_NONE);

    for(size_t i = 0; i < entries ;i++)
        rd->db.data[i] = empty;

    mh->counter = 0;
    mh->last_updated_s = 0;
    mh->current_entry = 0;
}

static inline void rrddim_fill_the_gap(STORAGE_COLLECT_HANDLE *sch, time_t now_collect_s) {
    struct mem_collect_handle *ch = (struct mem_collect_handle *)sch;
    struct mem_metric_handle *mh = (struct mem_metric_handle *)ch->smh;

    RRDDIM *rd = mh->rd;

    internal_fatal(ch->rd != mh->rd, "RRDDIM: dimensions do not match");
    check_metric_handle_from_rrddim(mh);

    size_t entries = mh->entries;
    time_t update_every_s = mh->update_every_s;
    time_t last_stored_s = mh->last_updated_s;
    size_t gap_entries = (now_collect_s - last_stored_s) / update_every_s;
    if(gap_entries >= entries)
        rrddim_store_metric_flush(sch);

    else {
        storage_number empty = pack_storage_number(NAN, SN_FLAG_NONE);
        size_t current_entry = mh->current_entry;
        time_t now_store_s = last_stored_s + update_every_s;

        // fill the dimension
        size_t c;
        for(c = 0; c < entries && now_store_s <= now_collect_s ; now_store_s += update_every_s, c++) {
            rd->db.data[current_entry++] = empty;

            if(unlikely(current_entry >= entries))
                current_entry = 0;
        }
        mh->counter += c;
        mh->current_entry = current_entry;
        mh->last_updated_s = now_store_s;
    }
}

void rrddim_collect_store_metric(STORAGE_COLLECT_HANDLE *sch,
                                 usec_t point_in_time_ut,
                                 NETDATA_DOUBLE n,
                                 NETDATA_DOUBLE min_value __maybe_unused,
                                 NETDATA_DOUBLE max_value __maybe_unused,
                                 uint16_t count __maybe_unused,
                                 uint16_t anomaly_count __maybe_unused,
                                 SN_FLAGS flags)
{
    struct mem_collect_handle *ch = (struct mem_collect_handle *)sch;
    struct mem_metric_handle *mh = (struct mem_metric_handle *)ch->smh;

    RRDDIM *rd = ch->rd;
    time_t point_in_time_s = (time_t)(point_in_time_ut / USEC_PER_SEC);

    internal_fatal(ch->rd != mh->rd, "RRDDIM: dimensions do not match");
    check_metric_handle_from_rrddim(mh);

    if(unlikely(point_in_time_s <= mh->last_updated_s))
        return;

    if(unlikely(mh->last_updated_s && point_in_time_s - mh->update_every_s > mh->last_updated_s))
        rrddim_fill_the_gap(sch, point_in_time_s);

    rd->db.data[mh->current_entry] = pack_storage_number(n, flags);
    mh->counter++;
    mh->current_entry = (mh->current_entry + 1) >= mh->entries ? 0 : mh->current_entry + 1;
    mh->last_updated_s = point_in_time_s;
}

int rrddim_collect_finalize(STORAGE_COLLECT_HANDLE *sch) {
    freez(sch);
    pulse_db_rrd_memory_sub(sizeof(struct mem_collect_handle));
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
static inline size_t rrddim_time2slot(STORAGE_METRIC_HANDLE *smh, time_t t) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)smh;
    RRDDIM *rd = mh->rd;

    size_t ret = 0;
    time_t last_entry_s  = rrddim_query_latest_time_s(smh);
    time_t first_entry_s = rrddim_query_oldest_time_s(smh);
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
        netdata_log_error("INTERNAL ERROR: rrddim_time2slot() on %s returns values outside entries", rrddim_name(rd));
        ret = entries - 1;
    }

    return ret;
}

// get the timestamp of a specific slot in the round-robin database
// only valid when not using dbengine
static inline time_t rrddim_slot2time(STORAGE_METRIC_HANDLE *smh, size_t slot) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)smh;
    RRDDIM *rd = mh->rd;

    time_t ret;
    time_t last_entry_s  = rrddim_query_latest_time_s(smh);
    time_t first_entry_s = rrddim_query_oldest_time_s(smh);
    size_t entries       = mh->entries;
    size_t last_slot     = rrddim_last_slot(mh);
    size_t update_every  = mh->update_every_s;

    if(slot >= entries) {
        netdata_log_error("INTERNAL ERROR: caller of rrddim_slot2time() gives invalid slot %zu", slot);
        slot = entries - 1;
    }

    if(slot > last_slot)
        ret = last_entry_s - (time_t)(update_every * (last_slot - slot + entries));
    else
        ret = last_entry_s - (time_t)(update_every * (last_slot - slot));

    if(unlikely(ret < first_entry_s)) {
        netdata_log_error("INTERNAL ERROR: rrddim_slot2time() on dimension '%s' of chart '%s' returned time (%ld) too far in the past (before first_entry_s %ld) for slot %zu",
              rrddim_name(rd), rrdset_id(rd->rrdset), ret, first_entry_s, slot);

        ret = first_entry_s;
    }

    if(unlikely(ret > last_entry_s)) {
        netdata_log_error("INTERNAL ERROR: rrddim_slot2time() on dimension '%s' of chart '%s' returned time (%ld) too far into the future (after last_entry_s %ld) for slot %zu",
              rrddim_name(rd), rrdset_id(rd->rrdset), ret, last_entry_s, slot);

        ret = last_entry_s;
    }

    return ret;
}

// ----------------------------------------------------------------------------
// RRDDIM legacy database query functions

void rrddim_query_init(STORAGE_METRIC_HANDLE *smh, struct storage_engine_query_handle *seqh, time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority __maybe_unused) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)smh;

    check_metric_handle_from_rrddim(mh);

    seqh->start_time_s = start_time_s;
    seqh->end_time_s = end_time_s;
    seqh->priority = priority;
    seqh->seb = STORAGE_ENGINE_BACKEND_RRDDIM;
    struct mem_query_handle* h = mallocz(sizeof(struct mem_query_handle));
    h->smh = smh;

    h->slot           = rrddim_time2slot(smh, start_time_s);
    h->last_slot      = rrddim_time2slot(smh, end_time_s);
    h->dt             = mh->update_every_s;

    h->next_timestamp = start_time_s;
    h->slot_timestamp = rrddim_slot2time(smh, h->slot);
    h->last_timestamp = rrddim_slot2time(smh, h->last_slot);

    // netdata_log_info("RRDDIM QUERY INIT: start %ld, end %ld, next %ld, first %ld, last %ld, dt %ld", start_time, end_time, h->next_timestamp, h->slot_timestamp, h->last_timestamp, h->dt);

    pulse_db_rrd_memory_add(sizeof(struct mem_query_handle));
    seqh->handle = (STORAGE_QUERY_HANDLE *)h;
}

// Returns the metric and sets its timestamp into current_time
// IT IS REQUIRED TO **ALWAYS** SET ALL RETURN VALUES (current_time, end_time, flags)
// IT IS REQUIRED TO **ALWAYS** KEEP TRACK OF TIME, EVEN OUTSIDE THE DATABASE BOUNDARIES
ALWAYS_INLINE STORAGE_POINT rrddim_query_next_metric(struct storage_engine_query_handle *seqh) {
    struct mem_query_handle* h = (struct mem_query_handle*)seqh->handle;
    struct mem_metric_handle *mh = (struct mem_metric_handle *)h->smh;
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

    storage_number n = rd->db.data[slot++];
    if(unlikely(slot >= entries)) slot = 0;

    h->slot = slot;
    h->slot_timestamp += h->dt;

    sp.anomaly_count = is_storage_number_anomalous(n) ? 1 : 0;
    sp.flags = (n & SN_USER_FLAGS);
    sp.min = sp.max = sp.sum = unpack_storage_number(n);

    return sp;
}

int rrddim_query_is_finished(struct storage_engine_query_handle *seqh) {
    struct mem_query_handle *h = (struct mem_query_handle*)seqh->handle;
    return (h->next_timestamp > seqh->end_time_s);
}

void rrddim_query_finalize(struct storage_engine_query_handle *seqh) {
#ifdef NETDATA_INTERNAL_CHECKS
    struct mem_query_handle *h = (struct mem_query_handle*)seqh->handle;
    struct mem_metric_handle *mh = (struct mem_metric_handle *)h->smh;

    internal_error(!rrddim_query_is_finished(seqh),
                   "QUERY: query for chart '%s' dimension '%s' has been stopped unfinished",
                   rrdset_id(mh->rd->rrdset), rrddim_name(mh->rd));

#endif
    freez(seqh->handle);
    pulse_db_rrd_memory_sub(sizeof(struct mem_query_handle));
}

time_t rrddim_query_align_to_optimal_before(struct storage_engine_query_handle *seqh) {
    return seqh->end_time_s;
}

time_t rrddim_query_latest_time_s(STORAGE_METRIC_HANDLE *smh) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)smh;
    return mh->last_updated_s;
}

time_t rrddim_query_oldest_time_s(STORAGE_METRIC_HANDLE *smh) {
    struct mem_metric_handle *mh = (struct mem_metric_handle *)smh;
    return (time_t)(mh->last_updated_s - metric_duration(mh));
}
