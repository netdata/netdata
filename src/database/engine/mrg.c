// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-internals.h"

struct aral_statistics mrg_aral_statistics;

// ----------------------------------------------------------------------------
// public API

inline MRG *mrg_create(void) {
    MRG *mrg = callocz(1, sizeof(MRG));

    for(size_t i = 0; i < _countof(mrg->index) ; i++) {
        rw_spinlock_init(&mrg->index[i].rw_spinlock);

        char buf[ARAL_MAX_NAME + 1];
        snprintfz(buf, ARAL_MAX_NAME, "mrg[%zu]", i);

        mrg->index[i].aral = aral_create(buf, sizeof(METRIC), 0, 16384, &mrg_aral_statistics, NULL, NULL,
                                         false, false, true);
    }
    pulse_aral_register_statistics(&mrg_aral_statistics, "mrg");

    mrg_load(mrg);
    return mrg;
}

struct aral_statistics *mrg_aral_stats(void) {
    return &mrg_aral_statistics;
}

size_t mrg_destroy(MRG *mrg) {
    if (!mrg)
        return 0;

    size_t referenced = 0;

    // Traverse all partitions
    for (size_t partition = 0; partition < UUIDMAP_PARTITIONS; partition++) {
        // Lock the partition to prevent new entries while we're cleaning up
        mrg_index_write_lock(mrg, partition);

        Word_t uuid_index = 0;
        Pvoid_t *uuid_pvalue;

        // Traverse all UUIDs in this partition
        for (uuid_pvalue = JudyLFirst(mrg->index[partition].uuid_judy, &uuid_index, PJE0);
             uuid_pvalue != NULL && uuid_pvalue != PJERR;
             uuid_pvalue = JudyLNext(mrg->index[partition].uuid_judy, &uuid_index, PJE0)) {

            if (!(*uuid_pvalue))
                continue;

            // Get the sections judy for this UUID
            Pvoid_t sections_judy = *uuid_pvalue;
            Word_t section_index = 0;
            Pvoid_t *section_pvalue;

            // Traverse all sections for this UUID
            for (section_pvalue = JudyLFirst(sections_judy, &section_index, PJE0);
                 section_pvalue != NULL && section_pvalue != PJERR;
                 section_pvalue = JudyLNext(sections_judy, &section_index, PJE0)) {

                if (!(*section_pvalue))
                    continue;

                METRIC *metric = *section_pvalue;

                // Try to acquire metric for deletion
                if (!refcount_acquire_for_deletion(&metric->refcount))
                    referenced++;

                uuidmap_free(metric->uuid);
                MRG_STATS_DELETED_METRIC(mrg, partition, metric->section);
                aral_freez(mrg->index[partition].aral, metric);
            }

            JudyLFreeArray(&sections_judy, PJE0);
        }

        JudyLFreeArray(&mrg->index[partition].uuid_judy, PJE0);

        // Unlock the partition
        mrg_index_write_unlock(mrg, partition);

        // Destroy the aral for this partition
        aral_destroy(mrg->index[partition].aral);
    }

    // Unregister the aral statistics
    pulse_aral_unregister_statistics(&mrg_aral_statistics);

    // Free the MRG structure
    freez(mrg);

    return referenced;
}

ALWAYS_INLINE
METRIC *mrg_metric_add_and_acquire(MRG *mrg, MRG_ENTRY entry, bool *ret) {
//    internal_fatal(entry.latest_time_s > max_acceptable_collected_time(),
//        "DBENGINE METRIC: metric latest time is in the future");

    return metric_add_and_acquire(mrg, &entry, ret);
}

ALWAYS_INLINE
METRIC *mrg_metric_get_and_acquire_by_uuid(MRG *mrg, nd_uuid_t *uuid, Word_t section) {
    UUIDMAP_ID id = uuidmap_create(*uuid);
    METRIC *metric = metric_get_and_acquire_by_id(mrg, id, section);
    uuidmap_free(id);
    return metric;
}

ALWAYS_INLINE
METRIC *mrg_metric_get_and_acquire_by_id(MRG *mrg, UUIDMAP_ID id, Word_t section) {
    return metric_get_and_acquire_by_id(mrg, id, section);
}

ALWAYS_INLINE
bool mrg_metric_release_and_delete(MRG *mrg, METRIC *metric) {
    return metric_release(mrg, metric);
}

ALWAYS_INLINE
METRIC *mrg_metric_dup(MRG *mrg, METRIC *metric) {
    metric_acquire(mrg, metric);
    return metric;
}

ALWAYS_INLINE
bool mrg_metric_release(MRG *mrg, METRIC *metric) {
    return metric_release(mrg, metric);
}

ALWAYS_INLINE
Word_t mrg_metric_id(MRG *mrg __maybe_unused, METRIC *metric) {
    return (Word_t)metric;
}

ALWAYS_INLINE
nd_uuid_t *mrg_metric_uuid(MRG *mrg __maybe_unused, METRIC *metric) {
    return uuidmap_uuid_ptr(metric->uuid);
}

ALWAYS_INLINE
UUIDMAP_ID mrg_metric_uuidmap_id_dup(MRG *mrg __maybe_unused, METRIC *metric) {
    return uuidmap_dup(metric->uuid);
}

ALWAYS_INLINE
Word_t mrg_metric_section(MRG *mrg __maybe_unused, METRIC *metric) {
    return metric->section;
}

ALWAYS_INLINE
bool mrg_metric_set_first_time_s(MRG *mrg __maybe_unused, METRIC *metric, time_t first_time_s) {
    internal_fatal(first_time_s < 0, "DBENGINE METRIC: timestamp is negative");

    if(first_time_s == LONG_MAX)
        first_time_s = 0;

    if(unlikely(first_time_s < 0))
        return false;

    __atomic_store_n(&metric->first_time_s, first_time_s, __ATOMIC_RELAXED);

    return true;
}

ALWAYS_INLINE
void mrg_metric_expand_retention(MRG *mrg __maybe_unused, METRIC *metric, time_t first_time_s, time_t last_time_s, uint32_t update_every_s) {
    internal_fatal(first_time_s < 0 || last_time_s < 0,
                   "DBENGINE METRIC: timestamp is negative");
    internal_fatal(first_time_s > max_acceptable_collected_time(),
                   "DBENGINE METRIC: metric first time is in the future");
    internal_fatal(last_time_s > max_acceptable_collected_time(),
                   "DBENGINE METRIC: metric last time is in the future");

    if(first_time_s > 0 && first_time_s != LONG_MAX)
        set_metric_field_with_condition(metric->first_time_s, first_time_s, _current <= 0 || (_wanted != 0 && _wanted != LONG_MAX && _wanted < _current));

    if(last_time_s > 0) {
        if(set_metric_field_with_condition(metric->latest_time_s_clean, last_time_s, _current <= 0 || _wanted > _current) &&
            update_every_s > 0)
            // set the latest update every too
            set_metric_field_with_condition(metric->latest_update_every_s, update_every_s, true);
    }
    else if(update_every_s > 0)
        // set it only if it is invalid
        set_metric_field_with_condition(metric->latest_update_every_s, update_every_s, _current <= 0);
}

ALWAYS_INLINE
bool mrg_metric_set_first_time_s_if_bigger(MRG *mrg __maybe_unused, METRIC *metric, time_t first_time_s) {
    internal_fatal(first_time_s < 0, "DBENGINE METRIC: timestamp is negative");
    return set_metric_field_with_condition(metric->first_time_s, first_time_s, _wanted != 0 && _wanted != LONG_MAX && _wanted > _current);
}

ALWAYS_INLINE
time_t mrg_metric_get_first_time_s(MRG *mrg __maybe_unused, METRIC *metric) {
    return mrg_metric_get_first_time_s_smart(mrg, metric);
}

void mrg_metric_clear_retention(MRG *mrg __maybe_unused, METRIC *metric) {
    __atomic_store_n(&metric->first_time_s, 0, __ATOMIC_RELAXED);
    __atomic_store_n(&metric->latest_time_s_clean, 0, __ATOMIC_RELAXED);
    __atomic_store_n(&metric->latest_time_s_hot, 0, __ATOMIC_RELAXED);
}

ALWAYS_INLINE_HOT
void mrg_metric_get_retention(MRG *mrg __maybe_unused, METRIC *metric, time_t *first_time_s, time_t *last_time_s, uint32_t *update_every_s) {
    time_t clean = __atomic_load_n(&metric->latest_time_s_clean, __ATOMIC_RELAXED);
    time_t hot = __atomic_load_n(&metric->latest_time_s_hot, __ATOMIC_RELAXED);

    *last_time_s = MAX(clean, hot);
    *first_time_s = mrg_metric_get_first_time_s_smart(mrg, metric);
    if (update_every_s)
        *update_every_s = __atomic_load_n(&metric->latest_update_every_s, __ATOMIC_RELAXED);
}

ALWAYS_INLINE
bool mrg_metric_set_clean_latest_time_s(MRG *mrg __maybe_unused, METRIC *metric, time_t latest_time_s) {
    internal_fatal(latest_time_s < 0, "DBENGINE METRIC: timestamp is negative");

//    internal_fatal(latest_time_s > max_acceptable_collected_time(),
//                   "DBENGINE METRIC: metric latest time is in the future");

//    internal_fatal(metric->latest_time_s_clean > latest_time_s,
//                   "DBENGINE METRIC: metric new clean latest time is older than the previous one");

    if(latest_time_s > 0) {
        if(set_metric_field_with_condition(metric->latest_time_s_clean, latest_time_s, true)) {
            set_metric_field_with_condition(metric->first_time_s, latest_time_s, _current <= 0 || _wanted < _current);

            return true;
        }
    }

    return false;
}

// returns true when metric still has retention
ALWAYS_INLINE
bool mrg_metric_has_zero_disk_retention(MRG *mrg __maybe_unused, METRIC *metric) {
    Word_t section = mrg_metric_section(mrg, metric);
    bool do_again = false;
    size_t countdown = 5;

    do {
        time_t min_first_time_s = LONG_MAX;
        time_t max_end_time_s = 0;
        PGC_PAGE *page;
        PGC_SEARCH method = PGC_SEARCH_FIRST;
        time_t page_first_time_s = 0;
        while ((page = pgc_page_get_and_acquire(main_cache, section, (Word_t)metric, page_first_time_s, method))) {
            method = PGC_SEARCH_NEXT;

            bool is_hot = pgc_is_page_hot(page);
            bool is_dirty = pgc_is_page_dirty(page);
            page_first_time_s = pgc_page_start_time_s(page);
            time_t page_end_time_s = pgc_page_end_time_s(page);

            if ((is_hot || is_dirty) && page_first_time_s > 0 && page_first_time_s < min_first_time_s)
                min_first_time_s = page_first_time_s;

            if (is_dirty && page_end_time_s > max_end_time_s)
                max_end_time_s = page_end_time_s;

            pgc_page_release(main_cache, page);
        }

        if (min_first_time_s == LONG_MAX)
            min_first_time_s = 0;

        if (--countdown && !min_first_time_s && __atomic_load_n(&metric->latest_time_s_hot, __ATOMIC_RELAXED))
            do_again = true;
        else {
            internal_error(!countdown, "METRIC: giving up on updating the retention of metric without disk retention");

            do_again = false;
            set_metric_field_with_condition(metric->first_time_s, min_first_time_s, true);
            set_metric_field_with_condition(metric->latest_time_s_clean, max_end_time_s, true);
        }
    } while(do_again);

    time_t first, last;
    mrg_metric_get_retention(mrg, metric, &first, &last, NULL);
    return (first && last && first < last);
}

ALWAYS_INLINE_HOT
bool mrg_metric_set_hot_latest_time_s(MRG *mrg __maybe_unused, METRIC *metric, time_t latest_time_s) {
    internal_fatal(latest_time_s < 0, "DBENGINE METRIC: timestamp is negative");

//    internal_fatal(latest_time_s > max_acceptable_collected_time(),
//                   "DBENGINE METRIC: metric latest time is in the future");

    if(likely(latest_time_s > 0)) {
        __atomic_store_n(&metric->latest_time_s_hot, latest_time_s, __ATOMIC_RELAXED);
        return true;
    }

    return false;
}

ALWAYS_INLINE
time_t mrg_metric_get_latest_clean_time_s(MRG *mrg __maybe_unused, METRIC *metric) {
    time_t clean = __atomic_load_n(&metric->latest_time_s_clean, __ATOMIC_RELAXED);
    return clean;
}

ALWAYS_INLINE_HOT
time_t mrg_metric_get_latest_time_s(MRG *mrg __maybe_unused, METRIC *metric) {
    time_t clean = __atomic_load_n(&metric->latest_time_s_clean, __ATOMIC_RELAXED);
    time_t hot = __atomic_load_n(&metric->latest_time_s_hot, __ATOMIC_RELAXED);

    return MAX(clean, hot);
}

ALWAYS_INLINE
bool mrg_metric_set_update_every(MRG *mrg __maybe_unused, METRIC *metric, uint32_t update_every_s) {
    if(likely(update_every_s > 0))
        return set_metric_field_with_condition(metric->latest_update_every_s, update_every_s, true);

    return false;
}

ALWAYS_INLINE_HOT
bool mrg_metric_set_update_every_s_if_zero(MRG *mrg __maybe_unused, METRIC *metric, uint32_t update_every_s) {
    if(likely(update_every_s > 0))
        return set_metric_field_with_condition(metric->latest_update_every_s, update_every_s, _current <= 0);

    return false;
}

ALWAYS_INLINE
uint32_t mrg_metric_get_update_every_s(MRG *mrg __maybe_unused, METRIC *metric) {
    return __atomic_load_n(&metric->latest_update_every_s, __ATOMIC_RELAXED);
}

#ifdef NETDATA_INTERNAL_CHECKS
ALWAYS_INLINE bool mrg_metric_set_writer(MRG *mrg, METRIC *metric) {
    pid_t expected = __atomic_load_n(&metric->writer, __ATOMIC_RELAXED);
    pid_t wanted = gettid_cached();
    bool done = true;

    do {
        if(expected != 0) {
            done = false;
            break;
        }
    } while(!__atomic_compare_exchange_n(&metric->writer, &expected, wanted, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

    if(done)
        __atomic_add_fetch(&mrg->index[metric->partition].stats.writers, 1, __ATOMIC_RELAXED);
    else
        __atomic_add_fetch(&mrg->index[metric->partition].stats.writers_conflicts, 1, __ATOMIC_RELAXED);

    return done;
}

ALWAYS_INLINE bool mrg_metric_clear_writer(MRG *mrg, METRIC *metric) {
    // this function can be called from a different thread than the one than the writer

    pid_t expected = __atomic_load_n(&metric->writer, __ATOMIC_RELAXED);
    pid_t wanted = 0;
    bool done = true;

    do {
        if(!expected) {
            done = false;
            break;
        }
    } while(!__atomic_compare_exchange_n(&metric->writer, &expected, wanted, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

    if(done)
        __atomic_sub_fetch(&mrg->index[metric->partition].stats.writers, 1, __ATOMIC_RELAXED);

    return done;
}
#endif

inline void mrg_update_metric_retention_and_granularity_by_uuid(
    MRG *mrg,
    Word_t section,
    nd_uuid_t(*uuid),
    time_t first_time_s,
    time_t last_time_s,
    uint32_t update_every_s,
    time_t now_s,
    uint64_t *journal_samples)
{
    if(unlikely(last_time_s > now_s)) {
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_WARNING,
                     "DBENGINE JV2: wrong last time on-disk (%ld - %ld, now %ld), "
                     "fixing last time to now",
                     first_time_s, last_time_s, now_s);
        last_time_s = now_s;
    }

    if (unlikely(first_time_s > last_time_s)) {
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_WARNING,
                     "DBENGINE JV2: wrong first time on-disk (%ld - %ld, now %ld), "
                     "fixing first time to last time",
                     first_time_s, last_time_s, now_s);

        first_time_s = last_time_s;
    }

    if (unlikely(first_time_s == 0 || last_time_s == 0)) {
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_WARNING,
                     "DBENGINE JV2: zero on-disk timestamps (%ld - %ld, now %ld), "
                     "using them as-is",
                     first_time_s, last_time_s, now_s);
    }

    bool added = false;
    METRIC *metric = mrg_metric_get_and_acquire_by_uuid(mrg, uuid, section);
    if (!metric) {
        MRG_ENTRY entry = {
            .uuid = uuid,
            .section = section,
            .first_time_s = first_time_s,
            .last_time_s = last_time_s,
            .latest_update_every_s = update_every_s,
        };
        metric = mrg_metric_add_and_acquire(mrg, entry, &added);
    }

    if (likely(!added)) {
        uint64_t old_samples = 0;

        if (update_every_s && metric->latest_update_every_s && metric->latest_time_s_clean)
            old_samples = (metric->latest_time_s_clean - metric->first_time_s) / metric->latest_update_every_s;

        mrg_metric_expand_retention(mrg, metric, first_time_s, last_time_s, update_every_s);

        uint64_t new_samples = 0;
        if (update_every_s && metric->latest_update_every_s && metric->latest_time_s_clean)
            new_samples = (metric->latest_time_s_clean - metric->first_time_s) / metric->latest_update_every_s;

        if (journal_samples)
            *journal_samples += (new_samples - old_samples);
    }
    else {
        // Newly added
        if (update_every_s) {
            uint64_t samples = (last_time_s - first_time_s) / update_every_s;
            if (journal_samples)
                *journal_samples += samples;
        }
    }

    mrg_metric_release(mrg, metric);
}

inline void mrg_get_statistics(MRG *mrg, struct mrg_statistics *s) {
    memset(s, 0, sizeof(struct mrg_statistics));

    for(size_t i = 0; i < _countof(mrg->index) ;i++) {
        s->entries += __atomic_load_n(&mrg->index[i].stats.entries, __ATOMIC_RELAXED);
        s->entries_acquired += __atomic_load_n(&mrg->index[i].stats.entries_acquired, __ATOMIC_RELAXED);
        s->size += __atomic_load_n(&mrg->index[i].stats.size, __ATOMIC_RELAXED);
        s->current_references += __atomic_load_n(&mrg->index[i].stats.current_references, __ATOMIC_RELAXED);
        s->additions += __atomic_load_n(&mrg->index[i].stats.additions, __ATOMIC_RELAXED);
        s->additions_duplicate += __atomic_load_n(&mrg->index[i].stats.additions_duplicate, __ATOMIC_RELAXED);
        s->deletions += __atomic_load_n(&mrg->index[i].stats.deletions, __ATOMIC_RELAXED);
        s->delete_having_retention_or_referenced += __atomic_load_n(&mrg->index[i].stats.delete_having_retention_or_referenced, __ATOMIC_RELAXED);
        s->delete_misses += __atomic_load_n(&mrg->index[i].stats.delete_misses, __ATOMIC_RELAXED);
        s->search_hits += __atomic_load_n(&mrg->index[i].stats.search_hits, __ATOMIC_RELAXED);
        s->search_misses += __atomic_load_n(&mrg->index[i].stats.search_misses, __ATOMIC_RELAXED);
        s->writers += __atomic_load_n(&mrg->index[i].stats.writers, __ATOMIC_RELAXED);
        s->writers_conflicts += __atomic_load_n(&mrg->index[i].stats.writers_conflicts, __ATOMIC_RELAXED);
    }

    s->size += sizeof(MRG);
}
