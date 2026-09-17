// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "ram.h"
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

STORAGE_METRICS_GROUP *ram_metrics_group_get(STORAGE_INSTANCE *si __maybe_unused, nd_uuid_t *uuid __maybe_unused) {
    return NULL;
}

void ram_metrics_group_release(STORAGE_INSTANCE *si __maybe_unused, STORAGE_METRICS_GROUP *smg __maybe_unused) {
    // if(!smg) return; // smg may be NULL
    ;
}

// ----------------------------------------------------------------------------
// RRDDIM legacy data collection functions

struct ram_metric_handle {
    RRDDIM *rd;
    storage_number *data;
    size_t memsize;
    RRD_DB_MODE memory_mode;
    bool indexed;

    size_t counter;
    size_t entries;
    size_t current_entry;
    time_t last_updated_s;
    time_t update_every_s;

    UUIDMAP_ID uuid_id;             // stored locally so cleanup doesn't need rd
    REFCOUNT refcount;
};

static RRDDIM *ram_metric_handle_rrddim_load(struct ram_metric_handle *mh) {
    return __atomic_load_n(&mh->rd, __ATOMIC_ACQUIRE);
}

static void ram_metric_handle_rrddim_store(struct ram_metric_handle *mh, RRDDIM *rd) {
    __atomic_store_n(&mh->rd, rd, __ATOMIC_RELEASE);
}

static storage_number *ram_metric_handle_data_load(const struct ram_metric_handle *mh) {
    return __atomic_load_n(&mh->data, __ATOMIC_RELAXED);
}

static void ram_metric_handle_data_store(struct ram_metric_handle *mh, storage_number *data) {
    __atomic_store_n(&mh->data, data, __ATOMIC_RELAXED);
}

static storage_number ram_metric_handle_slot_load(const struct ram_metric_handle *mh, size_t slot) {
    storage_number *data = ram_metric_handle_data_load(mh);
    return __atomic_load_n(&data[slot], __ATOMIC_RELAXED);
}

static void ram_metric_handle_slot_store(struct ram_metric_handle *mh, size_t slot, storage_number value) {
    storage_number *data = ram_metric_handle_data_load(mh);
    __atomic_store_n(&data[slot], value, __ATOMIC_RELAXED);
}

static size_t ram_metric_handle_counter_load(const struct ram_metric_handle *mh) {
    return __atomic_load_n(&mh->counter, __ATOMIC_RELAXED);
}

static void ram_metric_handle_counter_store(struct ram_metric_handle *mh, size_t counter) {
    __atomic_store_n(&mh->counter, counter, __ATOMIC_RELAXED);
}

static size_t ram_metric_handle_entries_load(const struct ram_metric_handle *mh) {
    return __atomic_load_n(&mh->entries, __ATOMIC_RELAXED);
}

static void ram_metric_handle_entries_store(struct ram_metric_handle *mh, size_t entries) {
    __atomic_store_n(&mh->entries, entries, __ATOMIC_RELAXED);
}

static size_t ram_metric_handle_current_entry_load(const struct ram_metric_handle *mh) {
    return __atomic_load_n(&mh->current_entry, __ATOMIC_RELAXED);
}

static void ram_metric_handle_current_entry_store(struct ram_metric_handle *mh, size_t current_entry) {
    __atomic_store_n(&mh->current_entry, current_entry, __ATOMIC_RELAXED);
}

static time_t ram_metric_handle_last_updated_s_load(const struct ram_metric_handle *mh) {
    return __atomic_load_n(&mh->last_updated_s, __ATOMIC_RELAXED);
}

static void ram_metric_handle_last_updated_s_store(struct ram_metric_handle *mh, time_t last_updated_s) {
    __atomic_store_n(&mh->last_updated_s, last_updated_s, __ATOMIC_RELAXED);
}

static time_t ram_metric_handle_update_every_s_load(const struct ram_metric_handle *mh) {
    return __atomic_load_n(&mh->update_every_s, __ATOMIC_RELAXED);
}

static void ram_metric_handle_update_every_s_store(struct ram_metric_handle *mh, time_t update_every_s) {
    __atomic_store_n(&mh->update_every_s, update_every_s, __ATOMIC_RELAXED);
}

static time_t ram_metric_handle_duration_s(const struct ram_metric_handle *mh) {
    time_t counter = (time_t)ram_metric_handle_counter_load(mh);
    time_t entries = (time_t)ram_metric_handle_entries_load(mh);

    return MIN(counter, entries) * ram_metric_handle_update_every_s_load(mh);
}

static size_t ram_metric_handle_last_slot(const struct ram_metric_handle *mh) {
    size_t current_entry = ram_metric_handle_current_entry_load(mh);
    size_t entries = ram_metric_handle_entries_load(mh);

    return current_entry == 0 ? entries - 1 : current_entry - 1;
}

static size_t ram_metric_handle_first_slot(const struct ram_metric_handle *mh) {
    size_t counter = ram_metric_handle_counter_load(mh);
    size_t entries = ram_metric_handle_entries_load(mh);

    return counter >= entries ? ram_metric_handle_current_entry_load(mh) : 0;
}

static void update_metric_handle_from_rrddim(struct ram_metric_handle *mh, RRDDIM *rd) {
    ram_metric_handle_data_store(mh, rd->db.data);
    mh->memsize        = rd->db.memsize;
    mh->memory_mode    = rd->rrd_memory_mode;
    ram_metric_handle_counter_store(mh, rd->rrdset->counter);
    ram_metric_handle_entries_store(mh, rd->rrdset->db.entries);
    ram_metric_handle_current_entry_store(mh, rd->rrdset->db.current_entry);
    ram_metric_handle_last_updated_s_store(mh, rd->rrdset->last_updated.tv_sec);
    ram_metric_handle_update_every_s_store(mh, rd->rrdset->update_every);
}

static void check_metric_handle_from_rrddim(struct ram_metric_handle *mh) {
    RRDDIM *rd = ram_metric_handle_rrddim_load(mh); (void)rd;
    if(!rd)
        return;

    internal_fatal(ram_metric_handle_entries_load(mh) != (size_t)rd->rrdset->db.entries,
                   "RRDDIM: entries do not match");
    internal_fatal(ram_metric_handle_update_every_s_load(mh) != rd->rrdset->update_every,
                   "RRDDIM: update every does not match");
}

static int64_t ram_metric_remove_from_index(struct ram_metric_handle *mh) {
    int64_t judy_mem = 0;

    netdata_rwlock_wrlock(&rrddim_Judy_rwlock);
    if(mh->indexed) {
        JudyAllocThreadPulseReset();
        JudyLDel(&rrddim_Judy_array, mh->uuid_id, PJE0);
        judy_mem = JudyAllocThreadPulseGetAndReset();
        mh->indexed = false;
    }
    netdata_rwlock_wrunlock(&rrddim_Judy_rwlock);

    return judy_mem;
}

static void ram_metric_free_data(struct ram_metric_handle *mh) {
    storage_number *data = ram_metric_handle_data_load(mh);
    if(!data)
        return;

    pulse_db_rrd_memory_sub(mh->memsize);

    if(mh->memory_mode == RRD_DB_MODE_RAM)
        nd_munmap(data, mh->memsize);
    else
        freez(data);

    ram_metric_handle_data_store(mh, NULL);
    mh->memsize = 0;
}

static void ram_metric_free_handle(struct ram_metric_handle *mh) {
    int64_t judy_mem = ram_metric_remove_from_index(mh);

    ram_metric_free_data(mh);
    freez(mh);
    pulse_db_rrd_memory_change(judy_mem - (int64_t)sizeof(struct ram_metric_handle));
}

STORAGE_METRIC_HANDLE *ram_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *si) {
    while(true) {
        struct ram_metric_handle *mh = (struct ram_metric_handle *)ram_metric_get_by_id(si, rd->uuid);

        if(!mh) {
            netdata_rwlock_wrlock(&rrddim_Judy_rwlock);
            JudyAllocThreadPulseReset();
            Pvoid_t *PValue = JudyLIns(&rrddim_Judy_array, rd->uuid, PJE0);
            int64_t judy_mem = JudyAllocThreadPulseGetAndReset();
            mh = *PValue;
            if(!mh) {
                mh = callocz(1, sizeof(struct ram_metric_handle));
                ram_metric_handle_rrddim_store(mh, rd);
                mh->uuid_id = rd->uuid;
                mh->refcount = 1;
                mh->indexed = true;
                update_metric_handle_from_rrddim(mh, rd);
                *PValue = mh;
                pulse_db_rrd_memory_change(judy_mem + (int64_t)sizeof(struct ram_metric_handle));
            }
            else {
                if(!refcount_acquire(&mh->refcount))
                    mh = NULL;
            }
            netdata_rwlock_wrunlock(&rrddim_Judy_rwlock);

            // The indexed handle is being deleted concurrently (acquire failed);
            // retry the lookup. Do NOT re-run get_by_id here: mh already carries
            // this caller's single reference (either refcount = 1 from the create
            // branch, or the +1 from refcount_acquire in the else branch), and
            // re-fetching would acquire a second reference that nothing releases.
            if(!mh)
                continue;
        }

        if(unlikely(ram_metric_handle_rrddim_load(mh) != rd)) {
            bool retry = false;
            int64_t judy_mem = 0;

            netdata_rwlock_wrlock(&rrddim_Judy_rwlock);
            if(ram_metric_handle_rrddim_load(mh) != rd) {
                // this can happen when the old RRDDIM is being deleted,
                // but the dictionary has not yet run the destructors
                if(mh->indexed) {
                    JudyAllocThreadPulseReset();
                    JudyLDel(&rrddim_Judy_array, mh->uuid_id, PJE0);
                    judy_mem = JudyAllocThreadPulseGetAndReset();
                    mh->indexed = false;
                }
                retry = true;
            }
            netdata_rwlock_wrunlock(&rrddim_Judy_rwlock);

            if(judy_mem)
                pulse_db_rrd_memory_change(judy_mem);

            if(retry) {
                ram_metric_release((STORAGE_METRIC_HANDLE *)mh);
                continue;
            }
        }

        return (STORAGE_METRIC_HANDLE *)mh;
    }
}

STORAGE_METRIC_HANDLE *ram_metric_get_by_id(STORAGE_INSTANCE *si __maybe_unused, UUIDMAP_ID id) {
    struct ram_metric_handle *mh = NULL;

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

STORAGE_METRIC_HANDLE *ram_metric_get_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *uuid) {
    UUIDMAP_ID id = uuidmap_create(*uuid);
    STORAGE_METRIC_HANDLE *mh = ram_metric_get_by_id(si, id);
    uuidmap_free(id);
    return mh;
}

STORAGE_METRIC_HANDLE *ram_metric_dup(STORAGE_METRIC_HANDLE *smh) {
    struct ram_metric_handle *mh = (struct ram_metric_handle *)smh;

    if(!refcount_acquire(&mh->refcount))
        fatal("DB_RAM_ALLOC: cannot acquire an already acquired refcount");

    return smh;
}

void ram_metric_release(STORAGE_METRIC_HANDLE *smh) {
    struct ram_metric_handle *mh = (struct ram_metric_handle *)smh;

    if(refcount_release_and_acquire_for_deletion(&mh->refcount))
        ram_metric_free_handle(mh);
}

bool ram_metric_release_from_rrddim(STORAGE_METRIC_HANDLE *smh, RRDDIM *rd) {
    struct ram_metric_handle *mh = (struct ram_metric_handle *)smh;
    bool data_transferred = false;
    int64_t judy_mem = 0;

    netdata_rwlock_wrlock(&rrddim_Judy_rwlock);
    if(ram_metric_handle_rrddim_load(mh) == rd) {
        ram_metric_handle_rrddim_store(mh, NULL);
        data_transferred = (ram_metric_handle_data_load(mh) == rd->db.data);

        if(mh->indexed) {
            JudyAllocThreadPulseReset();
            JudyLDel(&rrddim_Judy_array, mh->uuid_id, PJE0);
            judy_mem = JudyAllocThreadPulseGetAndReset();
            mh->indexed = false;
        }
    }
    netdata_rwlock_wrunlock(&rrddim_Judy_rwlock);

    if(judy_mem)
        pulse_db_rrd_memory_change(judy_mem);

    if(refcount_release_and_acquire_for_deletion(&mh->refcount))
        ram_metric_free_handle(mh);

    return data_transferred;
}

bool ram_metric_retention_by_uuid(STORAGE_INSTANCE *si __maybe_unused, nd_uuid_t *uuid, time_t *first_entry_s, time_t *last_entry_s) {
    STORAGE_METRIC_HANDLE *smh = ram_metric_get_by_uuid(si, uuid);
    if(!smh)
        return false;

    *first_entry_s = ram_oldest_time_s(smh);
    *last_entry_s = ram_latest_time_s(smh);
    ram_metric_release(smh);

    return true;
}

bool ram_metric_retention_by_id(STORAGE_INSTANCE *si __maybe_unused, UUIDMAP_ID id, time_t *first_entry_s, time_t *last_entry_s) {
    STORAGE_METRIC_HANDLE *smh = ram_metric_get_by_id(si, id);
    if(!smh)
        return false;

    *first_entry_s = ram_oldest_time_s(smh);
    *last_entry_s = ram_latest_time_s(smh);
    ram_metric_release(smh);

    return true;
}

void ram_retention_delete_by_id(STORAGE_INSTANCE *si __maybe_unused, UUIDMAP_ID id __maybe_unused) {
    ;
}

void ram_store_change_collection_frequency(STORAGE_COLLECT_HANDLE *sch, int update_every) {
    struct ram_collect_handle *ch = (struct ram_collect_handle *)sch;
    struct ram_metric_handle *mh = (struct ram_metric_handle *)ch->smh;

    ram_store_flush(sch);
    ram_metric_handle_update_every_s_store(mh, update_every);
}

STORAGE_COLLECT_HANDLE *ram_store_init(STORAGE_METRIC_HANDLE *smh, uint32_t update_every __maybe_unused, STORAGE_METRICS_GROUP *smg __maybe_unused) {
    struct ram_metric_handle *mh = (struct ram_metric_handle *)smh;
    RRDDIM *rd = ram_metric_handle_rrddim_load(mh);

    update_metric_handle_from_rrddim(mh, rd);
    internal_fatal((uint32_t)ram_metric_handle_update_every_s_load(mh) != update_every,
                   "RRDDIM: update requested does not match the dimension");

    struct ram_collect_handle *ch = callocz(1, sizeof(struct ram_collect_handle));
    ch->common.seb = STORAGE_ENGINE_BACKEND_RAM;
    ch->rd = rd;
    ch->smh = smh;

    pulse_db_rrd_memory_add(sizeof(struct ram_collect_handle));

    return (STORAGE_COLLECT_HANDLE *)ch;
}

void ram_store_flush(STORAGE_COLLECT_HANDLE *sch) {
    struct ram_collect_handle *ch = (struct ram_collect_handle *)sch;
    struct ram_metric_handle *mh = (struct ram_metric_handle *)ch->smh;

    size_t entries = ram_metric_handle_entries_load(mh);
    storage_number empty = pack_storage_number(NAN, SN_FLAG_NONE);

    for(size_t i = 0; i < entries ;i++)
        ram_metric_handle_slot_store(mh, i, empty);

    ram_metric_handle_counter_store(mh, 0);
    ram_metric_handle_last_updated_s_store(mh, 0);
    ram_metric_handle_current_entry_store(mh, 0);
}

static inline void ram_fill_the_gap(STORAGE_COLLECT_HANDLE *sch, time_t now_collect_s) {
    struct ram_collect_handle *ch = (struct ram_collect_handle *)sch;
    struct ram_metric_handle *mh = (struct ram_metric_handle *)ch->smh;

    internal_fatal(ch->rd != ram_metric_handle_rrddim_load(mh), "RRDDIM: dimensions do not match");
    check_metric_handle_from_rrddim(mh);

    size_t entries = ram_metric_handle_entries_load(mh);
    time_t update_every_s = ram_metric_handle_update_every_s_load(mh);
    time_t last_stored_s = ram_metric_handle_last_updated_s_load(mh);
    size_t gap_entries = (now_collect_s - last_stored_s) / update_every_s;
    if(gap_entries >= entries)
        ram_store_flush(sch);

    else {
        storage_number empty = pack_storage_number(NAN, SN_FLAG_NONE);
        size_t current_entry = ram_metric_handle_current_entry_load(mh);
        time_t now_store_s = last_stored_s + update_every_s;

        // fill the dimension
        size_t c;
        for(c = 0; c < entries && now_store_s <= now_collect_s ; now_store_s += update_every_s, c++) {
            ram_metric_handle_slot_store(mh, current_entry++, empty);

            if(unlikely(current_entry >= entries))
                current_entry = 0;
        }
        ram_metric_handle_counter_store(mh, ram_metric_handle_counter_load(mh) + c);
        ram_metric_handle_current_entry_store(mh, current_entry);
        ram_metric_handle_last_updated_s_store(mh, now_store_s);
    }
}

void ram_store_next(STORAGE_COLLECT_HANDLE *sch,
                    usec_t point_in_time_ut,
                    NETDATA_DOUBLE n,
                    NETDATA_DOUBLE min_value __maybe_unused,
                    NETDATA_DOUBLE max_value __maybe_unused,
                    uint16_t count __maybe_unused,
                    uint16_t anomaly_count __maybe_unused,
                    SN_FLAGS flags)
{
    struct ram_collect_handle *ch = (struct ram_collect_handle *)sch;
    struct ram_metric_handle *mh = (struct ram_metric_handle *)ch->smh;

    time_t point_in_time_s = (time_t)(point_in_time_ut / USEC_PER_SEC);

    internal_fatal(ch->rd != ram_metric_handle_rrddim_load(mh), "RRDDIM: dimensions do not match");
    check_metric_handle_from_rrddim(mh);

    time_t last_updated_s = ram_metric_handle_last_updated_s_load(mh);
    time_t update_every_s = ram_metric_handle_update_every_s_load(mh);

    if(unlikely(point_in_time_s <= last_updated_s))
        return;

    if(unlikely(last_updated_s && point_in_time_s - update_every_s > last_updated_s))
        ram_fill_the_gap(sch, point_in_time_s);

    size_t current_entry = ram_metric_handle_current_entry_load(mh);
    size_t entries = ram_metric_handle_entries_load(mh);

    ram_metric_handle_slot_store(mh, current_entry, pack_storage_number(n, flags));
    ram_metric_handle_counter_store(mh, ram_metric_handle_counter_load(mh) + 1);
    ram_metric_handle_current_entry_store(mh, (current_entry + 1) >= entries ? 0 : current_entry + 1);
    ram_metric_handle_last_updated_s_store(mh, point_in_time_s);
}

int ram_store_finalize(STORAGE_COLLECT_HANDLE *sch) {
    freez(sch);
    pulse_db_rrd_memory_sub(sizeof(struct ram_collect_handle));
    return 0;
}

// ----------------------------------------------------------------------------

// get the slot of the round-robin database, for the given timestamp (t)
// it always returns a valid slot, although it may not be for the time requested if the time is outside the round-robin database
// only valid when not using dbengine
static inline size_t ram_time2slot(STORAGE_METRIC_HANDLE *smh, time_t t) {
    struct ram_metric_handle *mh = (struct ram_metric_handle *)smh;

    size_t ret = 0;
    time_t last_entry_s  = ram_latest_time_s(smh);
    time_t first_entry_s = ram_oldest_time_s(smh);
    size_t entries       = ram_metric_handle_entries_load(mh);
    size_t first_slot    = ram_metric_handle_first_slot(mh);
    size_t last_slot     = ram_metric_handle_last_slot(mh);
    time_t update_every  = ram_metric_handle_update_every_s_load(mh);

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
        netdata_log_error("INTERNAL ERROR: ram_time2slot() returns values outside entries");
        ret = entries - 1;
    }

    return ret;
}

// get the timestamp of a specific slot in the round-robin database
// only valid when not using dbengine
static inline time_t ram_slot2time(STORAGE_METRIC_HANDLE *smh, size_t slot) {
    struct ram_metric_handle *mh = (struct ram_metric_handle *)smh;

    time_t ret;
    time_t last_entry_s  = ram_latest_time_s(smh);
    time_t first_entry_s = ram_oldest_time_s(smh);
    size_t entries       = ram_metric_handle_entries_load(mh);
    size_t last_slot     = ram_metric_handle_last_slot(mh);
    time_t update_every  = ram_metric_handle_update_every_s_load(mh);

    if(slot >= entries) {
        netdata_log_error("INTERNAL ERROR: caller of ram_slot2time() gives invalid slot %zu", slot);
        slot = entries - 1;
    }

    if(slot > last_slot)
        ret = last_entry_s - (time_t)(update_every * (last_slot - slot + entries));
    else
        ret = last_entry_s - (time_t)(update_every * (last_slot - slot));

    if(unlikely(ret < first_entry_s)) {
        netdata_log_error("INTERNAL ERROR: ram_slot2time() returned time (%ld) too far in the past (before first_entry_s %ld) for slot %zu",
              ret, first_entry_s, slot);

        ret = first_entry_s;
    }

    if(unlikely(ret > last_entry_s)) {
        netdata_log_error("INTERNAL ERROR: ram_slot2time() returned time (%ld) too far into the future (after last_entry_s %ld) for slot %zu",
              ret, last_entry_s, slot);

        ret = last_entry_s;
    }

    return ret;
}

// ----------------------------------------------------------------------------
// RRDDIM legacy database query functions

void ram_query_init(STORAGE_METRIC_HANDLE *smh, struct storage_engine_query_handle *seqh, time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority __maybe_unused) {
    struct ram_metric_handle *mh = (struct ram_metric_handle *)smh;

    seqh->start_time_s = start_time_s;
    seqh->end_time_s = end_time_s;
    seqh->priority = priority;
    seqh->seb = STORAGE_ENGINE_BACKEND_RAM;
    struct ram_query_handle* h = mallocz(sizeof(struct ram_query_handle));
    h->smh = smh;

    h->slot           = ram_time2slot(smh, start_time_s);
    h->last_slot      = ram_time2slot(smh, end_time_s);
    h->dt             = ram_metric_handle_update_every_s_load(mh);

    h->next_timestamp = start_time_s;
    h->slot_timestamp = ram_slot2time(smh, h->slot);
    h->last_timestamp = ram_slot2time(smh, h->last_slot);

    // netdata_log_info("RRDDIM QUERY INIT: start %ld, end %ld, next %ld, first %ld, last %ld, dt %ld", start_time, end_time, h->next_timestamp, h->slot_timestamp, h->last_timestamp, h->dt);

    pulse_db_rrd_memory_add(sizeof(struct ram_query_handle));
    seqh->handle = (STORAGE_QUERY_HANDLE *)h;
}

// Returns the metric and sets its timestamp into current_time
// IT IS REQUIRED TO **ALWAYS** SET ALL RETURN VALUES (current_time, end_time, flags)
// IT IS REQUIRED TO **ALWAYS** KEEP TRACK OF TIME, EVEN OUTSIDE THE DATABASE BOUNDARIES
ALWAYS_INLINE STORAGE_POINT ram_query_next(struct storage_engine_query_handle *seqh) {
    struct ram_query_handle* h = (struct ram_query_handle*)seqh->handle;
    struct ram_metric_handle *mh = (struct ram_metric_handle *)h->smh;

    size_t entries = ram_metric_handle_entries_load(mh);
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

    storage_number n = ram_metric_handle_slot_load(mh, slot++);
    if(unlikely(slot >= entries)) slot = 0;

    h->slot = slot;
    h->slot_timestamp += h->dt;

    sp.anomaly_count = is_storage_number_anomalous(n) ? 1 : 0;
    sp.flags = (n & SN_USER_FLAGS);
    sp.min = sp.max = sp.sum = unpack_storage_number(n);

    return sp;
}

int ram_query_is_finished(struct storage_engine_query_handle *seqh) {
    struct ram_query_handle *h = (struct ram_query_handle*)seqh->handle;
    return (h->next_timestamp > seqh->end_time_s);
}

void ram_query_finalize(struct storage_engine_query_handle *seqh) {
#ifdef NETDATA_INTERNAL_CHECKS
    internal_error(!ram_query_is_finished(seqh),
                   "QUERY: query for RRDDIM storage has been stopped unfinished");

#endif
    freez(seqh->handle);
    pulse_db_rrd_memory_sub(sizeof(struct ram_query_handle));
}

time_t ram_query_align_to_optimal_before(struct storage_engine_query_handle *seqh) {
    return seqh->end_time_s;
}

time_t ram_latest_time_s(STORAGE_METRIC_HANDLE *smh) {
    struct ram_metric_handle *mh = (struct ram_metric_handle *)smh;
    return ram_metric_handle_last_updated_s_load(mh);
}

time_t ram_oldest_time_s(STORAGE_METRIC_HANDLE *smh) {
    struct ram_metric_handle *mh = (struct ram_metric_handle *)smh;
    return (time_t)(ram_metric_handle_last_updated_s_load(mh) - ram_metric_handle_duration_s(mh));
}
