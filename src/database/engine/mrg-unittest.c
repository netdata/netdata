// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-internals.h"

struct mrg_stress_entry {
    nd_uuid_t uuid;
    time_t after;
    time_t before;
};

struct mrg_stress {
    MRG *mrg;
    bool stop;
    size_t entries;
    struct mrg_stress_entry *array;
    size_t updates;
};

static void mrg_stress(void *ptr) {
    struct mrg_stress *t = ptr;
    MRG *mrg = t->mrg;

    ssize_t start = 0;
    ssize_t end = (ssize_t)t->entries;
    ssize_t step = 1;

    if(gettid_cached() % 2) {
        start = (ssize_t)t->entries - 1;
        end = -1;
        step = -1;
    }

    while(!__atomic_load_n(&t->stop, __ATOMIC_RELAXED) && !nd_thread_signaled_to_cancel()) {
        for (ssize_t i = start; i != end; i += step) {
            struct mrg_stress_entry *e = &t->array[i];

            time_t after = __atomic_sub_fetch(&e->after, 1, __ATOMIC_RELAXED);
            time_t before = __atomic_add_fetch(&e->before, 1, __ATOMIC_RELAXED);

            mrg_update_metric_retention_and_granularity_by_uuid(
                mrg, 0x01,
                &e->uuid,
                after,
                before,
                1,
                before);

            __atomic_add_fetch(&t->updates, 1, __ATOMIC_RELAXED);
        }
    }
}

int mrg_unittest(void) {
    MRG *mrg = mrg_create();
    METRIC *m1_t0, *m2_t0, *m3_t0, *m4_t0;
    METRIC *m1_t1, *m2_t1, *m3_t1, *m4_t1;
    bool ret;

    nd_uuid_t test_uuid;
    uuid_generate(test_uuid);
    MRG_ENTRY entry = {
        .uuid = &test_uuid,
        .section = 0,
        .first_time_s = 2,
        .last_time_s = 3,
        .latest_update_every_s = 4,
    };
    m1_t0 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(!ret)
        fatal("DBENGINE METRIC: failed to add metric");

    // add the same metric again
    m2_t0 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(m2_t0 != m1_t0)
        fatal("DBENGINE METRIC: adding the same metric twice, does not return the same pointer");
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice");

    m3_t0 = mrg_metric_get_and_acquire_by_uuid(mrg, entry.uuid, entry.section);
    if(m3_t0 != m1_t0)
        fatal("DBENGINE METRIC: cannot find the metric added");

    // add the same metric again
    m4_t0 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(m4_t0 != m1_t0)
        fatal("DBENGINE METRIC: adding the same metric twice, does not return the same pointer");
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice");

    // add the same metric in another section
    entry.section = 1;
    m1_t1 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(!ret)
        fatal("DBENGINE METRIC: failed to add metric in section %zu", (size_t)entry.section);

    // add the same metric again
    m2_t1 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(m2_t1 != m1_t1)
        fatal("DBENGINE METRIC: adding the same metric twice (section %zu), does not return the same pointer", (size_t)entry.section);
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice in (section 0)");

    m3_t1 = mrg_metric_get_and_acquire_by_uuid(mrg, entry.uuid, entry.section);
    if(m3_t1 != m1_t1)
        fatal("DBENGINE METRIC: cannot find the metric added (section %zu)", (size_t)entry.section);

    // delete the first metric
    mrg_metric_release(mrg, m2_t0);
    mrg_metric_release(mrg, m3_t0);
    mrg_metric_release(mrg, m4_t0);
    mrg_metric_set_first_time_s(mrg, m1_t0, 0);
    mrg_metric_set_clean_latest_time_s(mrg, m1_t0, 0);
    mrg_metric_set_hot_latest_time_s(mrg, m1_t0, 0);
    if(!mrg_metric_release_and_delete(mrg, m1_t0))
        fatal("DBENGINE METRIC: cannot delete the first metric");

    m4_t1 = mrg_metric_get_and_acquire_by_uuid(mrg, entry.uuid, entry.section);
    if(m4_t1 != m1_t1)
        fatal("DBENGINE METRIC: cannot find the metric added (section %zu), after deleting the first one", (size_t)entry.section);

    // delete the second metric
    mrg_metric_release(mrg, m2_t1);
    mrg_metric_release(mrg, m3_t1);
    mrg_metric_release(mrg, m4_t1);
    mrg_metric_set_first_time_s(mrg, m1_t1, 0);
    mrg_metric_set_clean_latest_time_s(mrg, m1_t1, 0);
    mrg_metric_set_hot_latest_time_s(mrg, m1_t1, 0);
    if(!mrg_metric_release_and_delete(mrg, m1_t1))
        fatal("DBENGINE METRIC: cannot delete the second metric");

    struct mrg_statistics s;
    mrg_get_statistics(mrg, &s);
    if(s.entries != 0)
        fatal("DBENGINE METRIC: invalid entries counter");

    size_t entries = 1000000;
    size_t threads = _countof(mrg->index) / 3 + 1;
    size_t tiers = 3;
    size_t run_for_secs = 5;
    netdata_log_info("preparing stress test of %zu entries...", entries);
    struct mrg_stress t = {
        .mrg = mrg,
        .entries = entries,
        .array = callocz(entries, sizeof(struct mrg_stress_entry)),
    };

    time_t now = max_acceptable_collected_time();
    for(size_t i = 0; i < entries ;i++) {
        uuid_generate_random(t.array[i].uuid);
        t.array[i].after = now / 3;
        t.array[i].before = now / 2;
    }
    netdata_log_info("stress test is populating MRG with 3 tiers...");
    for(size_t i = 0; i < entries ;i++) {
        struct mrg_stress_entry *e = &t.array[i];
        for(size_t tier = 1; tier <= tiers ;tier++) {
            mrg_update_metric_retention_and_granularity_by_uuid(
                mrg, tier,
                &e->uuid,
                e->after,
                e->before,
                1,
                e->before);
        }
    }
    netdata_log_info("stress test ready to run...");

    usec_t started_ut = now_monotonic_usec();

    ND_THREAD *th[threads];
    for(size_t i = 0; i < threads ; i++) {
        char buf[15 + 1];
        snprintfz(buf, sizeof(buf) - 1, "TH[%zu]", i);
        th[i] = nd_thread_create(buf, NETDATA_THREAD_OPTION_DONT_LOG, mrg_stress, &t);
    }

    sleep_usec(run_for_secs * USEC_PER_SEC);
    __atomic_store_n(&t.stop, true, __ATOMIC_RELAXED);

    for(size_t i = 0; i < threads ; i++)
        nd_thread_signal_cancel(th[i]);

    for(size_t i = 0; i < threads ; i++)
        nd_thread_join(th[i]);

    usec_t ended_ut = now_monotonic_usec();

    struct mrg_statistics stats;
    mrg_get_statistics(mrg, &stats);

    netdata_log_info("DBENGINE METRIC: did %zu additions, %zu duplicate additions, "
                     "%zu deletions, %zu wrong deletions, "
                     "%zu successful searches, %zu wrong searches, "
                     "in %"PRIu64" usecs",
                     stats.additions, stats.additions_duplicate,
                     stats.deletions, stats.delete_misses,
                     stats.search_hits, stats.search_misses,
                     ended_ut - started_ut);

    netdata_log_info("DBENGINE METRIC: updates performance: %0.2fk/sec total, %0.2fk/sec/thread",
                     (double)t.updates / (double)((ended_ut - started_ut) / USEC_PER_SEC) / 1000.0,
                     (double)t.updates / (double)((ended_ut - started_ut) / USEC_PER_SEC) / 1000.0 / threads);

    mrg_destroy(mrg);

    netdata_log_info("DBENGINE METRIC: all tests passed!");

    return 0;
}
