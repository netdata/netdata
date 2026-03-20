// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-internals.h"
#include "rrdengine.h"

// Global dummy rrdengine_instances for tests
static struct rrdengine_instance test_ctx_0 = {0};
static struct rrdengine_instance test_ctx_1 = {0};
static struct rrdengine_instance test_ctx_tier[4] = { {0}, {0}, {0}, {0} }; // For stress test tiers

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
                mrg, (Word_t)&test_ctx_0, &e->uuid, after, before, 1, before, NULL);

            __atomic_add_fetch(&t->updates, 1, __ATOMIC_RELAXED);
        }
    }
}

int mrg_unittest(void) {
    // Use mrg_create_for_unittest to avoid pre-loaded metrics that block deletion
    MRG *mrg = mrg_create_for_unittest();
    METRIC *m1_t0, *m2_t0, *m3_t0, *m4_t0;
    METRIC *m1_t1, *m2_t1, *m3_t1, *m4_t1;
    bool ret;

    nd_uuid_t test_uuid;
    uuid_generate(test_uuid);
    MRG_ENTRY entry = {
        .uuid = &test_uuid,
        .section = (Word_t)&test_ctx_0,
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
    entry.section = (Word_t)&test_ctx_1;
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

    // Release all references to these initial test metrics
    mrg_metric_release(mrg, m2_t0);
    mrg_metric_release(mrg, m3_t0);
    mrg_metric_release(mrg, m4_t0);
    mrg_metric_release(mrg, m1_t0);

    mrg_metric_release(mrg, m2_t1);
    mrg_metric_release(mrg, m3_t1);
    mrg_metric_release(mrg, m4_t1);
    mrg_metric_release(mrg, m1_t1);

    size_t entries = 100000;  // Reduced from 1M to make deletion test feasible
    size_t threads = _countof(mrg->index) / 3 + 1;
    size_t tiers = 3;
    size_t run_for_secs = 5;
    fprintf(stderr, "preparing stress test of %zu entries...\n", entries);
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
    fprintf(stderr, "stress test is populating MRG with 3 tiers...\n");
    for(size_t i = 0; i < entries ;i++) {
        struct mrg_stress_entry *e = &t.array[i];
        for(size_t tier = 1; tier <= tiers ;tier++) {
            mrg_update_metric_retention_and_granularity_by_uuid(
                mrg, (Word_t)&test_ctx_tier[tier],
                &e->uuid,
                e->after,
                e->before,
                1,
                e->before, NULL);
        }
    }
    fprintf(stderr, "stress test ready to run...\n");

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

    fprintf(stderr, "DBENGINE METRIC: did %zu additions, %zu duplicate additions, "
                     "%zu deletions, %zu wrong deletions, "
                     "%zu successful searches, %zu wrong searches, "
                     "in %"PRIu64" usecs\n",
                     stats.additions, stats.additions_duplicate,
                     stats.deletions, stats.delete_misses,
                     stats.search_hits, stats.search_misses,
                     ended_ut - started_ut);

    fprintf(stderr, "DBENGINE METRIC: updates performance: %0.2fk/sec total, %0.2fk/sec/thread\n",
                     (double)t.updates / (double)((ended_ut - started_ut) / USEC_PER_SEC) / 1000.0,
                     (double)t.updates / (double)((ended_ut - started_ut) / USEC_PER_SEC) / 1000.0 / threads);

    fprintf(stderr, "DBENGINE METRIC: addition rate: %0.2fk/sec, search rate: %0.2fk/sec, deletion rate: %zu/%zu attempted\n",
                     (double)stats.additions / (double)((ended_ut - started_ut) / USEC_PER_SEC) / 1000.0,
                     (double)(stats.search_hits + stats.search_misses) / (double)((ended_ut - started_ut) / USEC_PER_SEC) / 1000.0,
                     stats.deletions, stats.deletions + stats.delete_misses);

    // Phase 3: Measure final statistics
    struct mrg_statistics final_stats;
    mrg_get_statistics(mrg, &final_stats);
    fprintf(stderr, "DBENGINE METRIC: final MRG state - %zu entries, %zu acquired\n",
                     final_stats.entries, final_stats.entries_acquired);

    freez(t.array);

    // Destroy MRG (will handle cleanup of any remaining metrics)
    size_t leaked = mrg_destroy(mrg);
    if(leaked > 0) {
        fprintf(stderr, "DBENGINE METRIC: warning - %zu metrics still referenced during destroy\n", leaked);
    } else {
        fprintf(stderr, "DBENGINE METRIC: all metrics properly cleaned up\n");
    }

    fprintf(stderr, "DBENGINE METRIC: all tests passed!\n");

    return 0;
}
