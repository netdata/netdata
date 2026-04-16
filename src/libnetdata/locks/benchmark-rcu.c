// SPDX-License-Identifier: GPL-3.0-or-later

#include "benchmark-rcu.h"

// ============================================================================
// RCU vs RW_SPINLOCK benchmark
//
// Compares read-side throughput between epoch-based RCU and RW_SPINLOCK
// under various reader/writer ratios.
//
// What each thread does:
//   Reader: acquires read lock, reads a shared counter, releases read lock.
//   Writer: acquires write lock, increments the counter, releases write lock.
//
// The counter value is irrelevant — we're measuring lock throughput.
// ============================================================================

#define MAX_THREADS 64
#define TEST_DURATION_SEC 1
#define STOP_SIGNAL UINT64_MAX
#define MAX_CONFIGS 12
#define NUM_LOCK_TYPES 2

typedef enum {
    BENCH_THREAD_READER,
    BENCH_THREAD_WRITER
} bench_thread_type_t;

typedef enum {
    BENCH_RW_SPINLOCK,
    BENCH_RCU
} bench_lock_type_t;

typedef struct {
    // Shared data that readers read and writers update
    uint64_t shared_value;

    // Per-thread stats
    struct {
        uint64_t operations;
        usec_t test_time;
        volatile int ready;
    } stats[MAX_THREADS];

    // Per-thread control
    struct {
        netdata_cond_t cond;
        netdata_mutex_t cond_mutex;
        uint64_t run_flag;
    } thread_controls[MAX_THREADS];
} bench_control_t;

typedef struct {
    double reader_ops_per_sec[NUM_LOCK_TYPES][MAX_CONFIGS];
    double writer_ops_per_sec[NUM_LOCK_TYPES][MAX_CONFIGS];
    int readers[MAX_CONFIGS];
    int writers[MAX_CONFIGS];
    int config_count;
} bench_summary_t;

typedef struct {
    int thread_id;
    bench_thread_type_t type;
    bench_lock_type_t lock_type;
    bench_control_t *control;

    // For RW_SPINLOCK mode
    RW_SPINLOCK *rw_spinlock;
    SPINLOCK *writer_spinlock; // serializes writers for RCU mode

    ND_THREAD *thread;
} bench_thread_ctx_t;

static inline uint64_t bench_run_flag_load(uint64_t *flag) {
    return __atomic_load_n(flag, __ATOMIC_ACQUIRE);
}

static void bench_wait_for_start(netdata_cond_t *cond, netdata_mutex_t *mutex, uint64_t *flag) {
    netdata_mutex_lock(mutex);
    while(bench_run_flag_load(flag) == 0)
        netdata_cond_wait(cond, mutex);
    netdata_mutex_unlock(mutex);
}

static void rcu_benchmark_thread(void *arg) {
    bench_thread_ctx_t *ctx = (bench_thread_ctx_t *)arg;
    bench_control_t *ctl = ctx->control;

    // Register with RCU subsystem if this is an RCU test
    if(ctx->lock_type == BENCH_RCU)
        rcu_register_thread();

    while(1) {
        bench_wait_for_start(&ctl->thread_controls[ctx->thread_id].cond,
                             &ctl->thread_controls[ctx->thread_id].cond_mutex,
                             &ctl->thread_controls[ctx->thread_id].run_flag);

        if(bench_run_flag_load(&ctl->thread_controls[ctx->thread_id].run_flag) == STOP_SIGNAL)
            break;

        usec_t start = now_monotonic_high_precision_usec();
        uint64_t operations = 0;

        if(ctx->lock_type == BENCH_RW_SPINLOCK) {
            RW_SPINLOCK *lock = ctx->rw_spinlock;

            if(ctx->type == BENCH_THREAD_READER) {
                while(bench_run_flag_load(&ctl->thread_controls[ctx->thread_id].run_flag)) {
                    rw_spinlock_read_lock(lock);
                    // Read shared data
                    uint64_t v __maybe_unused = ctl->shared_value;
                    (void)v;
                    rw_spinlock_read_unlock(lock);
                    operations++;
                }
            }
            else {
                while(bench_run_flag_load(&ctl->thread_controls[ctx->thread_id].run_flag)) {
                    rw_spinlock_write_lock(lock);
                    ctl->shared_value++;
                    rw_spinlock_write_unlock(lock);
                    operations++;
                }
            }
        }
        else { // BENCH_RCU
            if(ctx->type == BENCH_THREAD_READER) {
                while(bench_run_flag_load(&ctl->thread_controls[ctx->thread_id].run_flag)) {
                    rcu_read_lock();
                    // Read shared data
                    uint64_t v __maybe_unused = __atomic_load_n(&ctl->shared_value, __ATOMIC_ACQUIRE);
                    (void)v;
                    rcu_read_unlock();
                    operations++;
                }
            }
            else {
                while(bench_run_flag_load(&ctl->thread_controls[ctx->thread_id].run_flag)) {
                    // Writers still need mutual exclusion among themselves
                    spinlock_lock(ctx->writer_spinlock);
                    __atomic_add_fetch(&ctl->shared_value, 1, __ATOMIC_RELEASE);
                    spinlock_unlock(ctx->writer_spinlock);
                    // RCU synchronize not needed in tight benchmark loop —
                    // in real code you'd call it after replacing a pointer
                    // before freeing old data.
                    operations++;
                }
            }
        }

        usec_t test_time = now_monotonic_high_precision_usec() - start;
        __atomic_store_n(&ctl->stats[ctx->thread_id].test_time, test_time, __ATOMIC_RELEASE);
        __atomic_store_n(&ctl->stats[ctx->thread_id].operations, operations, __ATOMIC_RELEASE);
        __atomic_store_n(&ctl->stats[ctx->thread_id].ready, 1, __ATOMIC_RELEASE);
    }

    if(ctx->lock_type == BENCH_RCU)
        rcu_unregister_thread();
}

static void bench_print_thread_stats(const char *test_name, int readers, int writers,
                                     bench_thread_ctx_t *contexts, bench_control_t *ctl,
                                     bench_summary_t *summary, int config_idx, int lock_type) {
    fprintf(stderr, "\n%-20s (readers: %d, writers: %d)\n", test_name, readers, writers);
    fprintf(stderr, "%4s %8s %12s %12s %12s\n",
            "THR", "TYPE", "OPS", "OPS/SEC", "TIME (ms)");

    double reader_ops_per_sec = 0;
    double writer_ops_per_sec = 0;
    int total = readers + writers;

    for(int i = 0; i < total; i++) {
        uint64_t ops = __atomic_load_n(&ctl->stats[i].operations, __ATOMIC_RELAXED);
        usec_t time = __atomic_load_n(&ctl->stats[i].test_time, __ATOMIC_RELAXED);
        double ops_sec = (double)ops * USEC_PER_SEC / time;

        fprintf(stderr, "%4d %8s %12"PRIu64" %12.0f %12.2f\n",
                i,
                contexts[i].type == BENCH_THREAD_READER ? "READER" : "WRITER",
                ops, ops_sec, (double)time / 1000.0);

        if(contexts[i].type == BENCH_THREAD_READER)
            reader_ops_per_sec += ops_sec;
        else
            writer_ops_per_sec += ops_sec;
    }

    fprintf(stderr, "%4s %8s %12s %12.0f\n",
            "", "READER", "TOTAL", reader_ops_per_sec);
    fprintf(stderr, "%4s %8s %12s %12.0f\n",
            "", "WRITER", "TOTAL", writer_ops_per_sec);

    summary->reader_ops_per_sec[lock_type][config_idx] = reader_ops_per_sec;
    summary->writer_ops_per_sec[lock_type][config_idx] = writer_ops_per_sec;
    summary->readers[config_idx] = readers;
    summary->writers[config_idx] = writers;
}

static void bench_run_test(const char *name, int readers, int writers,
                           bench_thread_ctx_t *contexts, bench_control_t *ctl,
                           bench_summary_t *summary, int config_idx, int lock_type) {
    int total = readers + writers;

    fprintf(stderr, "\nRunning test: %s with %d readers, %d writers...\n",
            name, readers, writers);

    // Reset
    memset(ctl->stats, 0, sizeof(ctl->stats));
    ctl->shared_value = 0;

    // Signal threads to start
    for(int i = 0; i < total; i++) {
        netdata_mutex_lock(&ctl->thread_controls[i].cond_mutex);
        __atomic_store_n(&ctl->thread_controls[i].run_flag, 1, __ATOMIC_RELEASE);
        netdata_cond_signal(&ctl->thread_controls[i].cond);
        netdata_mutex_unlock(&ctl->thread_controls[i].cond_mutex);
    }

    sleep_usec(TEST_DURATION_SEC * USEC_PER_SEC);

    // Stop
    for(int i = 0; i < total; i++)
        __atomic_store_n(&ctl->thread_controls[i].run_flag, 0, __ATOMIC_RELEASE);

    // Wait for results
    for(int i = 0; i < total; i++)
        while(!__atomic_load_n(&ctl->stats[i].ready, __ATOMIC_ACQUIRE))
            sleep_usec(10);

    bench_print_thread_stats(name, readers, writers, contexts, ctl,
                             summary, config_idx, lock_type);
}

static void bench_print_summary(const bench_summary_t *summary) {
    const char *lock_names[] = {"rw_spinlock", "rcu"};

    fprintf(stderr, "\n=== RCU vs RW_SPINLOCK: Reader Throughput (Million ops/sec) ===\n\n");
    fprintf(stderr, "%-14s %-8s %-8s %-14s %-14s\n",
            "Lock", "Readers", "Writers", "Reader Mops/s", "Writer Mops/s");
    fprintf(stderr, "--------------------------------------------------------------\n");

    for(int config = 0; config < summary->config_count; config++) {
        for(int lock = 0; lock < NUM_LOCK_TYPES; lock++) {
            fprintf(stderr, "%-14s %-8d %-8d %-14.2f %-14.2f\n",
                    lock_names[lock],
                    summary->readers[config],
                    summary->writers[config],
                    summary->reader_ops_per_sec[lock][config] / 1000000.0,
                    summary->writer_ops_per_sec[lock][config] / 1000000.0);
        }
        if(config < summary->config_count - 1)
            fprintf(stderr, "--------------------------------------------------------------\n");
    }
    fprintf(stderr, "\n");
}

static void bench_stop_threads(bench_control_t *ctl, bench_thread_ctx_t *contexts, int created_threads) {
    for(int i = 0; i < created_threads; i++) {
        netdata_mutex_lock(&ctl->thread_controls[i].cond_mutex);
        __atomic_store_n(&ctl->thread_controls[i].run_flag, STOP_SIGNAL, __ATOMIC_RELEASE);
        netdata_cond_signal(&ctl->thread_controls[i].cond);
        netdata_mutex_unlock(&ctl->thread_controls[i].cond_mutex);
    }

    for(int i = 0; i < created_threads; i++)
        nd_thread_join(contexts[i].thread);
}

int rcu_stress_test(void) {
    bench_summary_t summary = {0};
    int ret = 0;

    RW_SPINLOCK rw_spinlock = RW_SPINLOCK_INITIALIZER;
    SPINLOCK rcu_writer_spinlock = SPINLOCK_INITIALIZER;

    rcu_init();

    // Test configurations: {readers, writers}
    int configs[][2] = {
        {1, 0},
        {2, 0},
        {4, 0},
        {4, 1},
        {8, 1},
        {8, 2},
        {16, 1},
        {16, 2},
        {32, 1},
    };

    int num_configs = (int)(sizeof(configs) / sizeof(configs[0]));
    summary.config_count = num_configs;

    // One control structure per lock type
    bench_control_t ctl[NUM_LOCK_TYPES];
    memset(ctl, 0, sizeof(ctl));

    bench_thread_ctx_t contexts[NUM_LOCK_TYPES][MAX_THREADS];
    memset(contexts, 0, sizeof(contexts));
    int created_threads[NUM_LOCK_TYPES] = {0};

    for(int lock = 0; lock < NUM_LOCK_TYPES; lock++) {
        for(int i = 0; i < MAX_THREADS; i++) {
            netdata_cond_init(&ctl[lock].thread_controls[i].cond);
            netdata_mutex_init(&ctl[lock].thread_controls[i].cond_mutex);
            __atomic_store_n(&ctl[lock].thread_controls[i].run_flag, 0, __ATOMIC_RELEASE);

            contexts[lock][i] = (bench_thread_ctx_t){
                .thread_id = i,
                .type = BENCH_THREAD_READER, // will be overridden per config
                .lock_type = lock,
                .control = &ctl[lock],
                .rw_spinlock = &rw_spinlock,
                .writer_spinlock = &rcu_writer_spinlock,
            };
        }
    }

    // Create all threads
    fprintf(stderr, "\nStarting RCU benchmark...\n");
    for(int lock = 0; lock < NUM_LOCK_TYPES; lock++) {
        for(int i = 0; i < MAX_THREADS; i++) {
            char name[32];
            snprintf(name, sizeof(name), "%s_%d",
                     lock == BENCH_RW_SPINLOCK ? "rwsp" : "rcu", i);
            ND_THREAD *thread =
                nd_thread_create(name, NETDATA_THREAD_OPTION_DONT_LOG,
                                 rcu_benchmark_thread, &contexts[lock][i]);
            if(unlikely(!thread)) {
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "RCU benchmark: failed to create %s thread %d",
                       lock == BENCH_RW_SPINLOCK ? "rw_spinlock" : "rcu", i);
                ret = 1;
                goto cleanup;
            }

            contexts[lock][i].thread = thread;
            created_threads[lock]++;
        }
    }

    sleep_usec(100000); // warm up

    for(int c = 0; c < num_configs; c++) {
        int readers = configs[c][0];
        int writers = configs[c][1];
        int total = readers + writers;

        for(int lock = 0; lock < NUM_LOCK_TYPES; lock++) {
            // Assign thread roles
            int idx = 0;
            for(int r = 0; r < readers; r++)
                contexts[lock][idx++].type = BENCH_THREAD_READER;
            for(int w = 0; w < writers; w++)
                contexts[lock][idx++].type = BENCH_THREAD_WRITER;

            char test_name[64];
            snprintf(test_name, sizeof(test_name), "%s %dR/%dW",
                     lock == BENCH_RW_SPINLOCK ? "rw_spinlock" : "rcu",
                     readers, writers);

            bench_run_test(test_name, readers, writers,
                           contexts[lock], &ctl[lock], &summary, c, lock);
        }
    }

    bench_print_summary(&summary);

    // Stop all threads
    fprintf(stderr, "\nStopping threads...\n");
    for(int lock = 0; lock < NUM_LOCK_TYPES; lock++) {
        bench_stop_threads(&ctl[lock], contexts[lock], created_threads[lock]);
        created_threads[lock] = 0;
    }

    // Cleanup
cleanup:
    if(created_threads[0] || created_threads[1]) {
        fprintf(stderr, "\nWaiting for threads to exit...\n");
        for(int lock = 0; lock < NUM_LOCK_TYPES; lock++)
            bench_stop_threads(&ctl[lock], contexts[lock], created_threads[lock]);
    }

    for(int lock = 0; lock < NUM_LOCK_TYPES; lock++) {
        for(int i = 0; i < MAX_THREADS; i++) {
            netdata_cond_destroy(&ctl[lock].thread_controls[i].cond);
            netdata_mutex_destroy(&ctl[lock].thread_controls[i].cond_mutex);
        }
    }

    rcu_destroy();
    return ret;
}
