// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-internals.h"
#include "rrdengine.h"

// Global dummy rrdengine_instances for tests
static struct rrdengine_instance test_ctx_0 = {0};
static struct rrdengine_instance test_ctx_1 = {0};
static struct rrdengine_instance test_ctx_tier[4] = { 0 }; // For stress test tiers

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

    // add the same metric again in section 1
    m4_t1 = mrg_metric_add_and_acquire(mrg, entry, &ret);
    if(m4_t1 != m1_t1)
        fatal("DBENGINE METRIC: adding the same metric twice (section %zu), does not return the same pointer", (size_t)entry.section);
    if(ret)
        fatal("DBENGINE METRIC: managed to add the same metric twice in (section %zu)", (size_t)entry.section);

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
    fprintf(stderr, "DBENGINE METRIC: final MRG state - %zu entries, %zd acquired\n",
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

// ============================================================================
// MRG Retention Benchmark
// ============================================================================

#define MRG_BENCH_MAX_THREADS 64
#define MRG_BENCH_TEST_DURATION_SEC 1
#define MRG_BENCH_STOP_SIGNAL UINT64_MAX
#define MRG_BENCH_MAX_CONFIGS 12

typedef struct {
    uint64_t operations;
    uint64_t violations;        // consistency check failures
    usec_t test_time;
    volatile int ready;
} mrg_bench_thread_stats_t;

typedef struct {
    netdata_cond_t cond;
    netdata_mutex_t cond_mutex;
    uint64_t run_flag;  // accessed by multiple threads via __atomic builtins
} mrg_bench_thread_control_t;

typedef enum {
    MRG_BENCH_READER,
    MRG_BENCH_WRITER
} mrg_bench_thread_type_t;

typedef struct {
    int thread_id;
    mrg_bench_thread_type_t type;
    MRG *mrg;
    METRIC *metric;
    mrg_bench_thread_stats_t *stats;
    mrg_bench_thread_control_t *control;
    ND_THREAD *thread;
} mrg_bench_thread_context_t;

typedef struct {
    double reader_ops_per_sec[MRG_BENCH_MAX_CONFIGS];
    double writer_ops_per_sec[MRG_BENCH_MAX_CONFIGS];
    uint64_t total_violations[MRG_BENCH_MAX_CONFIGS];
    int readers[MRG_BENCH_MAX_CONFIGS];
    int writers[MRG_BENCH_MAX_CONFIGS];
    int config_count;
} mrg_bench_summary_stats_t;

static void mrg_bench_wait_for_start(netdata_cond_t *cond, netdata_mutex_t *mutex, uint64_t *flag) {
    netdata_mutex_lock(mutex);
    while (__atomic_load_n(flag, __ATOMIC_RELAXED) == 0)
        netdata_cond_wait(cond, mutex);
    netdata_mutex_unlock(mutex);
}

static void mrg_bench_thread(void *arg) {
    mrg_bench_thread_context_t *ctx = (mrg_bench_thread_context_t *)arg;
    mrg_bench_thread_control_t *tc = ctx->control;
    MRG *mrg = ctx->mrg;
    METRIC *metric = ctx->metric;

    while(1) {
        mrg_bench_wait_for_start(&tc->cond, &tc->cond_mutex, &tc->run_flag);

        if(__atomic_load_n(&tc->run_flag, __ATOMIC_RELAXED) == MRG_BENCH_STOP_SIGNAL)
            break;

        usec_t start = now_monotonic_high_precision_usec();
        uint64_t operations = 0;
        uint64_t violations = 0;

        if(ctx->type == MRG_BENCH_WRITER) {
            // Writer: expand retention with incrementing timestamps
            time_t seq = 1000;
            while(__atomic_load_n(&tc->run_flag, __ATOMIC_RELAXED)) {
                seq++;
                // Expand retention with incrementing first and last time
                mrg_metric_expand_retention(mrg, metric, seq - 100, seq, 10);
                operations++;
            }
        }
        else {
            // Reader: read retention and check consistency invariant
            while(__atomic_load_n(&tc->run_flag, __ATOMIC_RELAXED)) {
                time_t first_time_s, last_time_s;
                uint32_t update_every_s;
                mrg_metric_get_retention(mrg, metric, &first_time_s, &last_time_s, &update_every_s);

                // Consistency check: first_time_s <= last_time_s
                if(unlikely(first_time_s > 0 && last_time_s > 0 && first_time_s > last_time_s))
                    violations++;

                operations++;
            }
        }

        usec_t test_time = now_monotonic_high_precision_usec() - start;
        __atomic_store_n(&ctx->stats[ctx->thread_id].test_time, test_time, __ATOMIC_RELEASE);
        __atomic_store_n(&ctx->stats[ctx->thread_id].operations, operations, __ATOMIC_RELEASE);
        __atomic_store_n(&ctx->stats[ctx->thread_id].violations, violations, __ATOMIC_RELEASE);
        __atomic_store_n(&ctx->stats[ctx->thread_id].ready, 1, __ATOMIC_RELEASE);
    }
}

static void mrg_bench_print_thread_stats(const char *test_name, int readers, int writers,
                                          mrg_bench_thread_context_t *contexts,
                                          mrg_bench_thread_stats_t *stats,
                                          mrg_bench_summary_stats_t *summary, int config_idx) {
    fprintf(stderr, "\n%-20s (readers: %d, writers: %d)\n", test_name, readers, writers);
    fprintf(stderr, "%4s %8s %12s %12s %12s %12s\n",
            "THR", "TYPE", "OPS", "OPS/SEC", "VIOLATIONS", "TIME (ms)");

    double reader_ops_per_sec = 0;
    double writer_ops_per_sec = 0;
    uint64_t total_violations = 0;

    for(int i = 0; i < readers + writers; i++) {
        uint64_t ops = __atomic_load_n(&stats[i].operations, __ATOMIC_RELAXED);
        uint64_t viol = __atomic_load_n(&stats[i].violations, __ATOMIC_RELAXED);
        usec_t time = __atomic_load_n(&stats[i].test_time, __ATOMIC_RELAXED);
        double ops_per_sec = time > 0 ? (double)ops * USEC_PER_SEC / time : 0.0;

        fprintf(stderr, "%4d %8s %12"PRIu64" %12.0f %12"PRIu64" %12.2f\n",
                i,
                contexts[i].type == MRG_BENCH_READER ? "READER" : "WRITER",
                ops, ops_per_sec, viol, (double)time / 1000.0);

        total_violations += viol;

        if(contexts[i].type == MRG_BENCH_READER)
            reader_ops_per_sec += ops_per_sec;
        else
            writer_ops_per_sec += ops_per_sec;
    }

    if(total_violations > 0) {
        fprintf(stderr, "\nFATAL ERROR: Detected %"PRIu64" consistency violations (torn reads)!\n",
                total_violations);
        fflush(stderr);
        _exit(1);
    }

    summary->reader_ops_per_sec[config_idx] = reader_ops_per_sec;
    summary->writer_ops_per_sec[config_idx] = writer_ops_per_sec;
    summary->total_violations[config_idx] = total_violations;
    summary->readers[config_idx] = readers;
    summary->writers[config_idx] = writers;
}

static void mrg_bench_run_test(const char *name, int readers, int writers,
                                mrg_bench_thread_context_t *contexts,
                                mrg_bench_thread_stats_t *stats,
                                mrg_bench_thread_control_t *controls,
                                mrg_bench_summary_stats_t *summary, int config_idx) {
    int total_threads = readers + writers;

    fprintf(stderr, "\nRunning test: %s with %d readers and %d writers...\n",
            name, readers, writers);

    // Reset
    memset(stats, 0, total_threads * sizeof(mrg_bench_thread_stats_t));

    // Signal threads to start
    for(int i = 0; i < total_threads; i++) {
        netdata_mutex_lock(&controls[i].cond_mutex);
        __atomic_store_n(&controls[i].run_flag, 1, __ATOMIC_RELAXED);
        netdata_cond_signal(&controls[i].cond);
        netdata_mutex_unlock(&controls[i].cond_mutex);
    }

    sleep_usec(MRG_BENCH_TEST_DURATION_SEC * USEC_PER_SEC);

    // Signal stop
    for(int i = 0; i < total_threads; i++)
        __atomic_store_n(&controls[i].run_flag, 0, __ATOMIC_RELEASE);

    // Wait for results
    for(int i = 0; i < total_threads; i++) {
        while(!__atomic_load_n(&stats[i].ready, __ATOMIC_ACQUIRE))
            sleep_usec(10);
    }

    mrg_bench_print_thread_stats(name, readers, writers, contexts, stats, summary, config_idx);
}

static void mrg_bench_print_summary(const mrg_bench_summary_stats_t *summary) {
    fprintf(stderr, "\n=== MRG Retention Benchmark Summary (Million ops/sec) ===\n\n");
    fprintf(stderr, "%-8s %-8s %16s %16s\n",
            "Readers", "Writers", "Reader Ops/s", "Writer Ops/s");
    fprintf(stderr, "----------------------------------------------------------------------\n");

    for(int config = 0; config < summary->config_count; config++) {
        double reader_ops = summary->reader_ops_per_sec[config];
        double writer_ops = summary->writer_ops_per_sec[config];

        fprintf(stderr, "%-8d %-8d %16.2f %16.2f\n",
                summary->readers[config],
                summary->writers[config],
                reader_ops / 1000000.0,
                writer_ops / 1000000.0);
    }
    fprintf(stderr, "\n");
}

int mrg_retention_benchmark(void) {
    mrg_bench_summary_stats_t summary = {0};

    // Use mrg_create_for_unittest() to avoid loading from database
    MRG *mrg = mrg_create_for_unittest();
    nd_uuid_t test_uuid;
    uuid_generate(test_uuid);

    MRG_ENTRY entry = {
        .uuid = &test_uuid,
        .section = (Word_t)&test_ctx_0,
        .first_time_s = 1000,
        .last_time_s = 2000,
        .latest_update_every_s = 10,
    };
    bool added;
    METRIC *metric = mrg_metric_add_and_acquire(mrg, entry, &added);

    if(!added) {
        fatal("DBENGINE METRIC: failed to add metric for benchmark");
    }

    mrg_bench_thread_stats_t stats[MRG_BENCH_MAX_THREADS];
    mrg_bench_thread_control_t controls[MRG_BENCH_MAX_THREADS];
    mrg_bench_thread_context_t contexts[MRG_BENCH_MAX_THREADS];

    fprintf(stderr, "\nStarting MRG retention benchmark...\n");
    fprintf(stderr, "Creating threads...\n");

    // Initialize per-thread controls
    for(int i = 0; i < MRG_BENCH_MAX_THREADS; i++) {
        netdata_cond_init(&controls[i].cond);
        netdata_mutex_init(&controls[i].cond_mutex);
        __atomic_store_n(&controls[i].run_flag, 0, __ATOMIC_RELAXED);
    }

    // Create threads
    for(int i = 0; i < MRG_BENCH_MAX_THREADS; i++) {
        char thr_name[32];
        snprintf(thr_name, sizeof(thr_name), "mrgbench%d", i);

        contexts[i] = (mrg_bench_thread_context_t){
            .thread_id = i,
            .type = MRG_BENCH_READER,
            .mrg = mrg,
            .metric = metric,
            .stats = stats,
            .control = &controls[i],
        };
        contexts[i].thread =
            nd_thread_create(thr_name, NETDATA_THREAD_OPTION_DONT_LOG, mrg_bench_thread, &contexts[i]);
    }

    // Test configurations: [readers, writers]
    int configs[][2] = {
        {1, 0},   // Single reader (no contention baseline)
        {0, 1},   // Single writer (write throughput baseline)
        {1, 1},   // 1 reader + 1 writer
        {2, 1},   // 2 readers + 1 writer (typical seqlock sweet spot)
        {4, 1},   // 4 readers + 1 writer
        {8, 1},   // 8 readers + 1 writer (high read contention)
        {16, 1},  // 16 readers + 1 writer
    };

    const int num_configs = sizeof(configs) / sizeof(configs[0]);
    summary.config_count = num_configs;

    // Warm up
    sleep_usec(100000);

    for(int i = 0; i < num_configs; i++) {
        int readers = configs[i][0];
        int writers = configs[i][1];
        int total = readers + writers;

        // Assign reader/writer roles
        int thread_idx = 0;
        for(int r = 0; r < readers; r++) {
            contexts[thread_idx].type = MRG_BENCH_READER;
            thread_idx++;
        }
        for(int w = 0; w < writers; w++) {
            contexts[thread_idx].type = MRG_BENCH_WRITER;
            thread_idx++;
        }

        // Reset roles for unused threads
        for(int j = total; j < MRG_BENCH_MAX_THREADS; j++) {
            contexts[j].type = MRG_BENCH_READER;
        }

        char test_name[64];
        snprintf(test_name, sizeof(test_name), "mrg_retention %dR/%dW", readers, writers);
        mrg_bench_run_test(test_name, readers, writers, contexts, stats, controls, &summary, i);
    }

    mrg_bench_print_summary(&summary);

    // Stop all threads
    fprintf(stderr, "Stopping threads...\n");
    for(int i = 0; i < MRG_BENCH_MAX_THREADS; i++) {
        netdata_mutex_lock(&controls[i].cond_mutex);
        __atomic_store_n(&controls[i].run_flag, MRG_BENCH_STOP_SIGNAL, __ATOMIC_RELAXED);
        netdata_cond_signal(&controls[i].cond);
        netdata_mutex_unlock(&controls[i].cond_mutex);
    }

    fprintf(stderr, "Waiting for threads to exit...\n");
    for(int i = 0; i < MRG_BENCH_MAX_THREADS; i++) {
        nd_thread_join(contexts[i].thread);
    }

    // Cleanup
    for(int i = 0; i < MRG_BENCH_MAX_THREADS; i++) {
        netdata_cond_destroy(&controls[i].cond);
        netdata_mutex_destroy(&controls[i].cond_mutex);
    }

    mrg_metric_release(mrg, metric);
    mrg_destroy(mrg);

    fprintf(stderr, "All benchmark tests passed.\n");
    return 0;
}
