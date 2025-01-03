// SPDX-License-Identifier: GPL-3.0-or-later

#include "benchmark-rw.h"

#define MAX_THREADS 64
#define TEST_DURATION_SEC 1
#define STOP_SIGNAL UINT64_MAX
#define MAX_CONFIGS 10

// Structure to store summary statistics
typedef struct {
    double ops_per_sec[2][MAX_CONFIGS];          // [lock_type][config_index], Total ops/sec
    double reader_ops_per_sec[2][MAX_CONFIGS];   // [lock_type][config_index], Reader ops/sec
    double writer_ops_per_sec[2][MAX_CONFIGS];   // [lock_type][config_index], Writer ops/sec
    int readers[MAX_CONFIGS];                    // Number of readers for each config
    int writers[MAX_CONFIGS];                    // Number of writers for each config
    int config_count;                            // Number of configurations tested
} summary_stats_t;

typedef struct {
    // Protected state to validate reader/writer mutual exclusion
    volatile int readers;           // Number of active readers
    volatile int writers;           // Number of active writers
    volatile uint64_t violations;   // Counter for reader/writer violations

    // Protected counter for actual work
    uint64_t counter;

    // Statistics per thread
    struct {
        uint64_t operations;        // Number of read/write operations
        usec_t test_time;          // Time spent in test
        volatile int ready;         // Thread completed flag
    } stats[MAX_THREADS];

    // Per-thread control
    struct {
        pthread_cond_t cond;              // Thread start condition
        pthread_mutex_t cond_mutex;       // Mutex for condition
        uint64_t run_flag;               // Thread run control
    } thread_controls[MAX_THREADS];
} rwlock_control_t;

typedef enum {
    THREAD_READER,
    THREAD_WRITER
} thread_type_t;

typedef struct {
    int thread_id;
    thread_type_t type;
    void *lock;                  // Points to either pthread_rwlock_t or RW_SPINLOCK
    bool is_spinlock;            // true for RW_SPINLOCK, false for pthread_rwlock_t
    rwlock_control_t *control;
    ND_THREAD *thread;
} thread_context_t;

static inline void verify_no_violations(rwlock_control_t *control) {
    if(__atomic_load_n(&control->violations, __ATOMIC_RELAXED) > 0) {
        fprintf(stderr, "\nFATAL ERROR: Detected %"PRIu64" read/write violations!\n"
                        "This indicates readers and writers were concurrently inside the lock.\n",
                control->violations);
        exit(1);
    }
}

static inline void check_access_safety(rwlock_control_t *control, thread_type_t type) {
    if(type == THREAD_READER) {
        // Reader entering critical section
        __atomic_add_fetch(&control->readers, 1, __ATOMIC_RELAXED);

        // Check if we have any writers - this would be a violation
        if(__atomic_load_n(&control->writers, __ATOMIC_RELAXED) > 0) {
            __atomic_add_fetch(&control->violations, 1, __ATOMIC_RELAXED);
        }
    }
    else {
        // Writer entering critical section
        int writers = __atomic_add_fetch(&control->writers, 1, __ATOMIC_RELAXED);

        // Check for other writers - violation!
        if(writers > 1) {
            __atomic_add_fetch(&control->violations, 1, __ATOMIC_RELAXED);
        }

        // Check if we have any readers - this would be a violation
        if(__atomic_load_n(&control->readers, __ATOMIC_RELAXED) > 0) {
            __atomic_add_fetch(&control->violations, 1, __ATOMIC_RELAXED);
        }
    }
}

static void release_access(rwlock_control_t *control, thread_type_t type) {
    if(type == THREAD_READER) {
        __atomic_sub_fetch(&control->readers, 1, __ATOMIC_RELAXED);
    }
    else {
        __atomic_sub_fetch(&control->writers, 1, __ATOMIC_RELAXED);
    }
}

static void wait_for_start(pthread_cond_t *cond, pthread_mutex_t *mutex, uint64_t *flag) {
    pthread_mutex_lock(mutex);
    while (*flag == 0)
        pthread_cond_wait(cond, mutex);
    pthread_mutex_unlock(mutex);
}

static void* benchmark_thread(void *arg) {
    thread_context_t *ctx = (thread_context_t *)arg;
    rwlock_control_t *control = ctx->control;

    while(1) {
        // Wait for start signal
        wait_for_start(&control->thread_controls[ctx->thread_id].cond,
                       &control->thread_controls[ctx->thread_id].cond_mutex,
                       &control->thread_controls[ctx->thread_id].run_flag);

        if (control->thread_controls[ctx->thread_id].run_flag == STOP_SIGNAL)
            break;

        usec_t start = now_monotonic_high_precision_usec();
        uint64_t operations = 0;

        while (control->thread_controls[ctx->thread_id].run_flag) {
            if(ctx->is_spinlock) {
                RW_SPINLOCK *spinlock = ctx->lock;
                if(ctx->type == THREAD_READER) {
                    rw_spinlock_read_lock(spinlock);
                    check_access_safety(control, THREAD_READER);
                    control->counter++;  // Just to do some work
                    release_access(control, THREAD_READER);
                    rw_spinlock_read_unlock(spinlock);
                }
                else {
                    rw_spinlock_write_lock(spinlock);
                    check_access_safety(control, THREAD_WRITER);
                    control->counter++;
                    release_access(control, THREAD_WRITER);
                    rw_spinlock_write_unlock(spinlock);
                }
            }
            else {
                pthread_rwlock_t *rwlock = ctx->lock;
                if(ctx->type == THREAD_READER) {
                    pthread_rwlock_rdlock(rwlock);
                    check_access_safety(control, THREAD_READER);
                    control->counter++;
                    release_access(control, THREAD_READER);
                    pthread_rwlock_unlock(rwlock);
                }
                else {
                    pthread_rwlock_wrlock(rwlock);
                    check_access_safety(control, THREAD_WRITER);
                    control->counter++;
                    release_access(control, THREAD_WRITER);
                    pthread_rwlock_unlock(rwlock);
                }
            }
            operations++;
        }

        // Store results
        usec_t test_time = now_monotonic_high_precision_usec() - start;
        __atomic_store_n(&control->stats[ctx->thread_id].test_time, test_time, __ATOMIC_RELEASE);
        __atomic_store_n(&control->stats[ctx->thread_id].operations, operations, __ATOMIC_RELEASE);
        __atomic_store_n(&control->stats[ctx->thread_id].ready, 1, __ATOMIC_RELEASE);
    }

    return NULL;
}

static void print_summary(const summary_stats_t *summary) {
    fprintf(stderr, "\n=== Performance Summary (Million operations/sec) ===\n\n");
    fprintf(stderr, "%-16s %-8s %-8s %-16s %-16s\n",
            "Lock Type", "Readers", "Writers", "Reader Ops/s", "Writer Ops/s");
    fprintf(stderr, "----------------------------------------------------------------------\n");

    const char *lock_names[] = {"pthread_rwlock", "rw_spinlock"};

    for (int config = 0; config < summary->config_count; config++) {
        for (int lock_type = 0; lock_type < 2; lock_type++) {
            // double total_ops = summary->ops_per_sec[lock_type][config];
            int readers = summary->readers[config];
            int writers = summary->writers[config];

            // Get the actual reader and writer operations
            double reader_ops = readers > 0 ? summary->reader_ops_per_sec[lock_type][config] : 0;
            double writer_ops = writers > 0 ? summary->writer_ops_per_sec[lock_type][config] : 0;

            fprintf(stderr, "%-16s %-8d %-8d %-16.2f %-16.2f\n",
                    lock_names[lock_type],
                    readers,
                    writers,
                    reader_ops / 1000000.0,
                    writer_ops / 1000000.0);
        }
        // Add a separator between configurations
        if (config < summary->config_count - 1)
            fprintf(stderr, "----------------------------------------------------------------------\n");
    }
    fprintf(stderr, "\n");
}

static void print_thread_stats(const char *test_name, int readers, int writers,
                               thread_context_t *contexts, rwlock_control_t *control,
                               summary_stats_t *summary, int config_idx, int lock_type) {
    fprintf(stderr, "\n%-20s (readers: %d, writers: %d)\n", test_name, readers, writers);
    fprintf(stderr, "%4s %8s %12s %12s %12s\n",
            "THR", "TYPE", "OPS", "OPS/SEC", "TIME (ms)");

    uint64_t total_ops = 0;
    double total_ops_per_sec = 0;
    double reader_ops_per_sec = 0;
    double writer_ops_per_sec = 0;

    for(int i = 0; i < readers + writers; i++) {
        uint64_t ops = __atomic_load_n(&control->stats[i].operations, __ATOMIC_RELAXED);
        usec_t time = __atomic_load_n(&control->stats[i].test_time, __ATOMIC_RELAXED);
        double ops_per_sec = (double)ops * USEC_PER_SEC / time;

        fprintf(stderr, "%4d %8s %12"PRIu64" %12.0f %12.2f\n",
                i,
                contexts[i].type == THREAD_READER ? "READER" : "WRITER",
                ops,
                ops_per_sec,
                (double)time / 1000.0);

        total_ops += ops;
        total_ops_per_sec += ops_per_sec;

        if (contexts[i].type == THREAD_READER) {
            reader_ops_per_sec += ops_per_sec;
        } else {
            writer_ops_per_sec += ops_per_sec;
        }
    }

    fprintf(stderr, "%4s %8s %12"PRIu64" %12.0f\n",
            "TOT", "", total_ops, total_ops_per_sec);

    // Store in summary
    summary->ops_per_sec[lock_type][config_idx] = total_ops_per_sec;
    summary->reader_ops_per_sec[lock_type][config_idx] = reader_ops_per_sec;
    summary->writer_ops_per_sec[lock_type][config_idx] = writer_ops_per_sec;
    summary->readers[config_idx] = readers;
    summary->writers[config_idx] = writers;

    verify_no_violations(control);
}


static void run_test(const char *name, int readers, int writers,
                     thread_context_t *contexts, rwlock_control_t *control,
                     summary_stats_t *summary, int config_idx, int lock_type) {
    fprintf(stderr, "\nRunning test: %s with %d readers and %d writers...\n",
            name, readers, writers);

    // Reset all stats and control
    memset(&control->stats, 0, sizeof(control->stats));
    control->counter = 0;
    control->readers = 0;
    control->writers = 0;
    control->violations = 0;

    int total_threads = readers + writers;

    // Signal threads to start
    for(int i = 0; i < total_threads; i++) {
        pthread_mutex_lock(&control->thread_controls[i].cond_mutex);
        control->thread_controls[i].run_flag = 1;
        pthread_cond_signal(&control->thread_controls[i].cond);
        pthread_mutex_unlock(&control->thread_controls[i].cond_mutex);
    }

    // Wait for test duration
    sleep_usec(TEST_DURATION_SEC * USEC_PER_SEC);

    // Signal threads to stop
    for(int i = 0; i < total_threads; i++) {
        __atomic_store_n(&control->thread_controls[i].run_flag, 0, __ATOMIC_RELEASE);
    }

    // Wait for threads to report results
    for(int i = 0; i < total_threads; i++) {
        while(!__atomic_load_n(&control->stats[i].ready, __ATOMIC_ACQUIRE))
            sleep_usec(10);
    }

    print_thread_stats(name, readers, writers, contexts, control, summary, config_idx, lock_type);
}

int rwlocks_stress_test(void) {
    pthread_rwlock_t pthread_rwlock = PTHREAD_RWLOCK_INITIALIZER;
    RW_SPINLOCK rw_spinlock = RW_SPINLOCK_INITIALIZER;
    summary_stats_t summary = {0};

    // Initialize control structures
    rwlock_control_t pthread_control = { 0 };
    rwlock_control_t spinlock_control = { 0 };

    // Initialize per-thread controls for both locks
    for(int i = 0; i < MAX_THREADS; i++) {
        pthread_control.thread_controls[i].cond = (pthread_cond_t)PTHREAD_COND_INITIALIZER;
        pthread_control.thread_controls[i].cond_mutex = (pthread_mutex_t)PTHREAD_MUTEX_INITIALIZER;
        pthread_control.thread_controls[i].run_flag = 0;

        spinlock_control.thread_controls[i].cond = (pthread_cond_t)PTHREAD_COND_INITIALIZER;
        spinlock_control.thread_controls[i].cond_mutex = (pthread_mutex_t)PTHREAD_MUTEX_INITIALIZER;
        spinlock_control.thread_controls[i].run_flag = 0;
    }

    // Create thread contexts
    thread_context_t pthread_contexts[MAX_THREADS];
    thread_context_t spinlock_contexts[MAX_THREADS];

    fprintf(stderr, "\nStarting RW locks benchmark...\n");

    // Test configurations: [readers, writers]
    int configs[][2] = {
        {1, 0},   // Single reader
        {0, 1},   // Single writer
        {1, 1},   // One reader + one writer
        {2, 1},   // Two readers + one writer
        {1, 2},   // One reader + two writers
        {2, 2},   // Two readers + two writers
        {4, 1},   // Four readers + one writer
        {1, 4},   // One reader + four writers
        {4, 4},   // Four readers + four writers
    };

    const int num_configs = sizeof(configs) / sizeof(configs[0]);
    summary.config_count = num_configs;

    // Create all threads
    for(int i = 0; i < MAX_THREADS; i++) {
        char thr_name[32];

        // Initialize pthread contexts
        pthread_contexts[i] = (thread_context_t){
            .thread_id = i,
            .type = i % 2 == 0 ? THREAD_READER :THREAD_WRITER,
            .lock = &pthread_rwlock,
            .is_spinlock = false,
            .control = &pthread_control
        };

        snprintf(thr_name, sizeof(thr_name), "pthread_rw%d", i);
        pthread_contexts[i].thread = nd_thread_create(
            thr_name,
            NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_JOINABLE,
            benchmark_thread,
            &pthread_contexts[i]);

        // Initialize spinlock contexts
        spinlock_contexts[i] = (thread_context_t){
            .thread_id = i,
            .type = i % 2 == 0 ? THREAD_READER : THREAD_WRITER,
            .lock = &rw_spinlock,
            .is_spinlock = true,
            .control = &spinlock_control
        };

        snprintf(thr_name, sizeof(thr_name), "spin_rw%d", i);
        spinlock_contexts[i].thread = nd_thread_create(
            thr_name,
            NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_JOINABLE,
            benchmark_thread,
            &spinlock_contexts[i]);
    }

    // Run all configurations
    for(int i = 0; i < num_configs; i++) {
        int readers = configs[i][0];
        int writers = configs[i][1];

        // Create all threads
        int thread_idx = 0;

        // First assign reader threads
        for(int r = 0; r < readers; r++) {
            pthread_contexts[thread_idx].type = THREAD_READER;
            spinlock_contexts[thread_idx].type = THREAD_READER;
            thread_idx++;
        }

        // Then assign writer threads
        for(int w = 0; w < writers; w++) {
            pthread_contexts[thread_idx].type = THREAD_WRITER;
            spinlock_contexts[thread_idx].type = THREAD_WRITER;
            thread_idx++;
        }

        char test_name[64];
        snprintf(test_name, sizeof(test_name), "pthread_rwlock %dR/%dW", readers, writers);
        run_test(test_name, readers, writers, pthread_contexts, &pthread_control, &summary, i, 0);

        snprintf(test_name, sizeof(test_name), "rw_spinlock %dR/%dW", readers, writers);
        run_test(test_name, readers, writers, spinlock_contexts, &spinlock_control, &summary, i, 1);
    }

    // Print the summary table
    print_summary(&summary);

    // Stop all threads
    fprintf(stderr, "\nStopping threads...\n");
    for(int i = 0; i < MAX_THREADS; i++) {
        // Signal pthread threads
        pthread_mutex_lock(&pthread_control.thread_controls[i].cond_mutex);
        pthread_control.thread_controls[i].run_flag = STOP_SIGNAL;
        pthread_cond_signal(&pthread_control.thread_controls[i].cond);
        pthread_mutex_unlock(&pthread_control.thread_controls[i].cond_mutex);

        // Signal spinlock threads
        pthread_mutex_lock(&spinlock_control.thread_controls[i].cond_mutex);
        spinlock_control.thread_controls[i].run_flag = STOP_SIGNAL;
        pthread_cond_signal(&spinlock_control.thread_controls[i].cond);
        pthread_mutex_unlock(&spinlock_control.thread_controls[i].cond_mutex);
    }

    // Join all threads
    fprintf(stderr, "\nWaiting for threads to exit...\n");
    for(int i = 0; i < MAX_THREADS; i++) {
        nd_thread_join(pthread_contexts[i].thread);
        nd_thread_join(spinlock_contexts[i].thread);
    }

    // Cleanup condition variables and mutexes
    for(int i = 0; i < MAX_THREADS; i++) {
        pthread_cond_destroy(&pthread_control.thread_controls[i].cond);
        pthread_mutex_destroy(&pthread_control.thread_controls[i].cond_mutex);
        pthread_cond_destroy(&spinlock_control.thread_controls[i].cond);
        pthread_mutex_destroy(&spinlock_control.thread_controls[i].cond_mutex);
    }

    pthread_rwlock_destroy(&pthread_rwlock);

    return 0;
}
