// SPDX-License-Identifier: GPL-3.0-or-later

#include "benchmark.h"

#define MAX_THREADS 64
#define TEST_DURATION_SEC 1
#define STOP_SIGNAL UINT64_MAX
#define NUM_LOCK_TYPES 5

// Structure to store summary stats
typedef struct {
    double locks_per_sec[NUM_LOCK_TYPES][7];  // [lock_type][thread_count_index]
} summary_stats_t;

typedef struct {
    uint64_t locks;
    usec_t test_time;
    volatile int ready;
} thread_stats_t;

typedef struct {
    pthread_cond_t cond;              // Individual condition for each thread
    netdata_mutex_t cond_mutex;       // Individual mutex for each thread
    uint64_t run_flag;               // Individual run flag for each thread
} thread_control_t;

typedef struct {
    uint64_t protected_counter;
    thread_stats_t stats[MAX_THREADS];
    thread_control_t thread_controls[MAX_THREADS];  // Array of per-thread controls
} lock_control_t;

typedef enum {
    LOCK_MUTEX,
    LOCK_RWLOCK,
    LOCK_SPINLOCK,
    LOCK_RW_SPINLOCK,
    LOCK_WAITQ
} lock_type_t;

typedef struct {
    int thread_id;
    lock_type_t type;
    WAITQ_PRIORITY priority;    // For waitq only
    lock_control_t *control;
    void *lock;                 // Points to the actual lock
    ND_THREAD *thread;
} thread_context_t;

static const char *lock_names[] = {
    "Mutex",
    "RWLock",
    "Spinlock",
    "RW Spinlock",
    "WaitQ"
};

static const char *priority_to_string(WAITQ_PRIORITY p) {
    switch(p) {
        case WAITQ_PRIO_URGENT: return "URGENT";
        case WAITQ_PRIO_HIGH:   return "HIGH";
        case WAITQ_PRIO_NORMAL: return "NORMAL";
        case WAITQ_PRIO_LOW:    return "LOW";
        default:                return "UNKNOWN";
    }
}

static void print_summary(const summary_stats_t *summary) {
    fprintf(stderr, "\n=== Performance Summary (Million locks/sec) ===\n\n");
    fprintf(stderr, "%-12s %8s %8s %8s %8s %8s %8s %8s\n",
                                "Lock Type", "1", "2", "4", "8", "16", "32", "64");
    fprintf(stderr, "------------------------------------------------------------------------------\n");

    for(int type = 0; type < NUM_LOCK_TYPES; type++) {
        fprintf(stderr, "%-12s", lock_names[type]);
        for(int i = 0; i < 7; i++) {  // 6 different thread counts
            fprintf(stderr, " %8.2f", summary->locks_per_sec[type][i] / 1000000.0);
        }
        fprintf(stderr, "\n");
    }
    fprintf(stderr, "\n");
}

static void wait_for_signal(pthread_cond_t *cond, pthread_mutex_t *mutex, uint64_t *flag) {
    pthread_mutex_lock(mutex);
    while (*flag == 0)
        pthread_cond_wait(cond, mutex);
    pthread_mutex_unlock(mutex);
}

static void benchmark_thread(void *arg) {
    thread_context_t *ctx = (thread_context_t *)arg;
    thread_stats_t *stats = &ctx->control->stats[ctx->thread_id];
    thread_control_t *thread_control = &ctx->control->thread_controls[ctx->thread_id];

    while(1) {
        wait_for_signal(&thread_control->cond, &thread_control->cond_mutex, &thread_control->run_flag);

        if (thread_control->run_flag == STOP_SIGNAL)
            break;

        usec_t start = now_monotonic_high_precision_usec();
        uint64_t local_counter = 0;

        switch(ctx->type) {
            case LOCK_MUTEX: {
                pthread_mutex_t *mutex = ctx->lock;
                while (thread_control->run_flag) {
                    pthread_mutex_lock(mutex);
                    ctx->control->protected_counter++;
                    pthread_mutex_unlock(mutex);
                    local_counter++;
                }
                break;
            }

            case LOCK_RWLOCK: {
                pthread_rwlock_t *rwlock = ctx->lock;
                while (thread_control->run_flag) {
                    pthread_rwlock_wrlock(rwlock);
                    ctx->control->protected_counter++;
                    pthread_rwlock_unlock(rwlock);
                    local_counter++;
                }
                break;
            }

            case LOCK_SPINLOCK: {
                SPINLOCK *spinlock = ctx->lock;
                while (thread_control->run_flag) {
                    spinlock_lock(spinlock);
                    ctx->control->protected_counter++;
                    spinlock_unlock(spinlock);
                    local_counter++;
                }
                break;
            }

            case LOCK_RW_SPINLOCK: {
                RW_SPINLOCK *rw_spinlock = ctx->lock;
                while (thread_control->run_flag) {
                    rw_spinlock_write_lock(rw_spinlock);
                    ctx->control->protected_counter++;
                    rw_spinlock_write_unlock(rw_spinlock);
                    local_counter++;
                }
                break;
            }

            case LOCK_WAITQ: {
                WAITQ *waitq = ctx->lock;
                WAITQ_PRIORITY priority = ctx->priority;
                while (thread_control->run_flag) {
                    waitq_acquire(waitq, priority);
                    ctx->control->protected_counter++;
                    waitq_release(waitq);
                    local_counter++;
                }
                break;
            }
        }

        // Store results atomically
        usec_t test_time = now_monotonic_high_precision_usec() - start;
        __atomic_store_n(&stats->test_time, test_time, __ATOMIC_RELEASE);
        __atomic_store_n(&stats->locks, local_counter, __ATOMIC_RELEASE);
        __atomic_store_n(&stats->ready, 1, __ATOMIC_RELEASE);
    }
}

static void print_thread_stats(const char *test_name, int threads, thread_context_t *contexts,
                               thread_stats_t *stats, uint64_t protected_counter,
                               summary_stats_t *summary, int thread_count_idx) {
    fprintf(stderr, "\n%-20s (threads: %d)\n", test_name, threads);
    if (strcmp(test_name, "WaitQ") == 0) {
        fprintf(stderr, "%4s %8s %12s %12s %12s\n",
                "THR", "PRIO", "LOCKS", "LOCKS/SEC", "TIME (ms)");
    }
    else {
        fprintf(stderr, "%4s %12s %12s %12s\n",
                "THR", "LOCKS", "LOCKS/SEC", "TIME (ms)");
    }

    uint64_t total_locks = 0;
    double total_locks_per_sec = 0;

    for(int i = 0; i < threads; i++) {
        uint64_t locks = __atomic_load_n(&stats[i].locks, __ATOMIC_ACQUIRE);
        usec_t time = __atomic_load_n(&stats[i].test_time, __ATOMIC_ACQUIRE);
        double locks_per_sec = (double)locks * USEC_PER_SEC / time;
        total_locks_per_sec += locks_per_sec;

        if (strcmp(test_name, "WaitQ") == 0) {
            fprintf(stderr, "%4d %8s %12"PRIu64" %12.0f %12.2f\n",
                    i,
                    priority_to_string(contexts[i].priority),
                    locks,
                    locks_per_sec,
                    (double)time / 1000.0);
        }
        else {
            fprintf(stderr, "%4d %12"PRIu64" %12.0f %12.2f\n",
                    i, locks, locks_per_sec,
                    (double)time / 1000.0);
        }

        total_locks += locks;
    }

    if(total_locks != protected_counter) {
        fprintf(stderr, "\nERROR: Counter mismatch!\n");
        fprintf(stderr, "Sum of thread counters: %"PRIu64"\n", total_locks);
        fprintf(stderr, "Protected counter: %"PRIu64"\n", protected_counter);
        fprintf(stderr, "Difference: %"PRIu64"\n",
                total_locks > protected_counter ?
                    total_locks - protected_counter :
                    protected_counter - total_locks);

        fflush(stderr);
        _exit(1);
    }

    fprintf(stderr, "%4s %12"PRIu64"\n", "TOT", total_locks);

    // Store in summary for the final table
    summary->locks_per_sec[contexts[0].type][thread_count_idx] = total_locks_per_sec;
}

static void run_test(const char *name, int threads, thread_context_t *contexts,
                     lock_control_t *control, summary_stats_t *summary) {
    fprintf(stderr, "\nRunning test: %s with %d threads...\n", name, threads);

    // Reset stats and counter
    for(int i = 0; i < threads; i++) {
        __atomic_store_n(&control->stats[i].locks, 0, __ATOMIC_RELEASE);
        __atomic_store_n(&control->stats[i].test_time, 0, __ATOMIC_RELEASE);
        __atomic_store_n(&control->stats[i].ready, 0, __ATOMIC_RELEASE);
    }
    control->protected_counter = 0;

    // Signal only the threads we need for this test
    for(int i = 0; i < threads; i++) {
        thread_control_t *thread_control = &control->thread_controls[i];
        pthread_mutex_lock(&thread_control->cond_mutex);
        thread_control->run_flag = 1;
        pthread_cond_signal(&thread_control->cond);
        pthread_mutex_unlock(&thread_control->cond_mutex);
    }

    // Wait for test duration
    sleep_usec(TEST_DURATION_SEC * USEC_PER_SEC);

    // Signal threads to stop
    for(int i = 0; i < threads; i++) {
        thread_control_t *thread_control = &control->thread_controls[i];
        __atomic_store_n(&thread_control->run_flag, 0, __ATOMIC_RELEASE);
    }

    // Wait for threads to report results
    for(int i = 0; i < threads; i++) {
        while(!__atomic_load_n(&control->stats[i].ready, __ATOMIC_ACQUIRE))
            sleep_usec(10);
    }

    // Get thread count index for summary
    int thread_count_idx;
    switch(threads) {
        case 1: thread_count_idx = 0; break;
        case 2: thread_count_idx = 1; break;
        case 4: thread_count_idx = 2; break;
        case 8: thread_count_idx = 3; break;
        case 16: thread_count_idx = 4; break;
        case 32: thread_count_idx = 5; break;
        case 64: thread_count_idx = 6; break;
        default: thread_count_idx = 0; break;
    }

    print_thread_stats(name, threads, contexts, control->stats, control->protected_counter,
                       summary, thread_count_idx);
}

static void set_waitq_priorities(int thread_count, thread_context_t *contexts) {
    switch(thread_count) {
        case 1:
            contexts[0].priority = WAITQ_PRIO_URGENT;
            break;

        case 2:
            contexts[0].priority = WAITQ_PRIO_URGENT;
            contexts[1].priority = WAITQ_PRIO_HIGH;
            break;

        case 4:
            contexts[0].priority = WAITQ_PRIO_URGENT;
            contexts[1].priority = WAITQ_PRIO_HIGH;
            contexts[2].priority = WAITQ_PRIO_NORMAL;
            contexts[3].priority = WAITQ_PRIO_LOW;
            break;

        default: { // 8, 16, 32
            int threads_per_priority = thread_count / 4;
            int remainder = thread_count % 4;
            int thread_idx = 0;

            for (int prio = WAITQ_PRIO_URGENT; prio <= WAITQ_PRIO_LOW; prio++) {
                int count = threads_per_priority + (remainder > 0 ? 1 : 0);
                remainder--;

                for (int i = 0; i < count && thread_idx < thread_count; i++) {
                    contexts[thread_idx++].priority = prio;
                }
            }
            break;
        }
    }
}

int locks_stress_test(void) {
    summary_stats_t summary = {0};

    // Initialize actual locks
    netdata_mutex_t mutex;
    netdata_rwlock_t rwlock;
    netdata_mutex_init(&mutex);
    netdata_rwlock_init(&rwlock);

    SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    RW_SPINLOCK rw_spinlock = RW_SPINLOCK_INITIALIZER;
    WAITQ waitq = WAITQ_INITIALIZER;

    void *locks[] = {
        &mutex,
        &rwlock,
        &spinlock,
        &rw_spinlock,
        &waitq
    };

    // Initialize control structures
    lock_control_t controls[NUM_LOCK_TYPES] = { 0 };
    for(int i = 0; i < NUM_LOCK_TYPES; i++) {
        // Initialize per-thread condition variables and mutexes
        for(int j = 0; j < MAX_THREADS; j++) {
            pthread_cond_init(&controls[i].thread_controls[j].cond, NULL);
            pthread_mutex_init(&controls[i].thread_controls[j].cond_mutex, NULL);
            controls[i].thread_controls[j].run_flag = 0;
        }
    }

    // Initialize thread arrays
    thread_context_t *threads[NUM_LOCK_TYPES];
    for(int i = 0; i < NUM_LOCK_TYPES; i++) {
        threads[i] = calloc(MAX_THREADS, sizeof(thread_context_t));
        if(!threads[i]) {
            fprintf(stderr, "Failed to allocate memory for threads\n");
            return 1;
        }

        // Initialize thread contexts
        for(int j = 0; j < MAX_THREADS; j++) {
            threads[i][j] = (thread_context_t){
                .thread_id = j,
                .type = i,
                .control = &controls[i],
                .lock = locks[i]
            };
        }
    }

    // Create all threads
    fprintf(stderr, "Creating threads...\n");
    for(int type = 0; type < NUM_LOCK_TYPES; type++) {
        for(int i = 0; i < MAX_THREADS; i++) {
            char thr_name[32];
            snprintf(thr_name, sizeof(thr_name), "%s%d", lock_names[type], i);
            threads[type][i].thread = nd_thread_create(
                thr_name,
                NETDATA_THREAD_OPTION_DONT_LOG,
                benchmark_thread,
                &threads[type][i]);
        }
    }

    // Run tests with different thread counts
    int thread_counts[] = {1, 2, 4, 8, 16, 32, 64};

    // Warm up the CPU
    sleep_usec(100000);

    for(size_t i = 0; i < sizeof(thread_counts)/sizeof(thread_counts[0]); i++) {
        int count = thread_counts[i];

        // Set waitq priorities for this test
        set_waitq_priorities(count, threads[LOCK_WAITQ]);

        // Run test for each lock type
        for(int type = 0; type < NUM_LOCK_TYPES; type++) {
            run_test(lock_names[type], count, threads[type], &controls[type], &summary);
        }
    }

    // Print the summary table
    print_summary(&summary);

    // Signal all threads to exit
    fprintf(stderr, "\nStopping threads...\n");
    for(int type = 0; type < NUM_LOCK_TYPES; type++) {
        for(int i = 0; i < MAX_THREADS; i++) {
            thread_control_t *thread_control = &controls[type].thread_controls[i];
            netdata_mutex_lock(&thread_control->cond_mutex);
            thread_control->run_flag = STOP_SIGNAL;
            pthread_cond_signal(&thread_control->cond);
            netdata_mutex_unlock(&thread_control->cond_mutex);
        }
    }

    // Join all threads
    fprintf(stderr, "\nWaiting for threads to exit...\n");
    for(int type = 0; type < NUM_LOCK_TYPES; type++) {
        for(int i = 0; i < MAX_THREADS; i++) {
            nd_thread_join(threads[type][i].thread);
        }
    }

    // Cleanup condition variables and mutexes
    for(int type = 0; type < NUM_LOCK_TYPES; type++) {
        for(int i = 0; i < MAX_THREADS; i++) {
            pthread_cond_destroy(&controls[type].thread_controls[i].cond);
            netdata_mutex_destroy(&controls[type].thread_controls[i].cond_mutex);
        }
        free(threads[type]);
    }

    return 0;
}