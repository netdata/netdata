// SPDX-License-Identifier: GPL-3.0-or-later

#include "waitq.h"

#define MAX_USEC 512 // Maximum backoff limit in microseconds

#define PRIORITY_SHIFT 32
#define NO_PRIORITY 0

static ALWAYS_INLINE uint64_t make_order(WAITQ_PRIORITY priority, uint64_t seqno) {
    return ((uint64_t)priority << PRIORITY_SHIFT) + seqno;
}

static ALWAYS_INLINE uint64_t get_our_order(WAITQ *waitq, WAITQ_PRIORITY priority) {
    uint64_t seqno = __atomic_add_fetch(&waitq->last_seqno, 1, __ATOMIC_RELAXED);
    return make_order(priority, seqno);
}

ALWAYS_INLINE void waitq_init(WAITQ *waitq) {
    spinlock_init(&waitq->spinlock);
    waitq->current_priority = 0;
    waitq->last_seqno = 0;
}

ALWAYS_INLINE void waitq_destroy(WAITQ *wq __maybe_unused) { ; }

static ALWAYS_INLINE bool write_our_priority(WAITQ *waitq, uint64_t our_order) {
    uint64_t current = __atomic_load_n(&waitq->current_priority, __ATOMIC_RELAXED);
    if(current == our_order) return true;

    do {

        if(current != NO_PRIORITY && current < our_order)
            return false;

    } while(!__atomic_compare_exchange_n(
                &waitq->current_priority,
                &current,
                our_order,
                false,
                __ATOMIC_RELAXED,
                __ATOMIC_RELAXED));

    return true;
}

static ALWAYS_INLINE bool clear_our_priority(WAITQ *waitq, uint64_t our_order) {
    uint64_t expected = our_order;

    return
        __atomic_compare_exchange_n(
            &waitq->current_priority,
            &expected,
            NO_PRIORITY,
            false,
            __ATOMIC_RELAXED,
            __ATOMIC_RELAXED);
}

ALWAYS_INLINE bool waitq_try_acquire_with_trace(WAITQ *waitq, WAITQ_PRIORITY priority, const char *func __maybe_unused) {
    // Fast path for no contention - try to get the lock immediately without a sequence number
    if (__atomic_load_n(&waitq->current_priority, __ATOMIC_RELAXED) == NO_PRIORITY && 
        spinlock_trylock(&waitq->spinlock)) {
        waitq->writer = gettid_cached();
        return true;
    }

    // Normal path with queuing if contention exists
    uint64_t our_order = get_our_order(waitq, priority);

    bool rc = write_our_priority(waitq, our_order) && spinlock_trylock(&waitq->spinlock);
    if(rc)
        waitq->writer = gettid_cached();

    clear_our_priority(waitq, our_order);

    return rc;
}

ALWAYS_INLINE void waitq_acquire_with_trace(WAITQ *waitq, WAITQ_PRIORITY priority, const char *func) {
    // Fast path for no contention - try to get the lock immediately without a sequence number
    if (__atomic_load_n(&waitq->current_priority, __ATOMIC_RELAXED) == NO_PRIORITY && 
        spinlock_trylock(&waitq->spinlock)) {
        waitq->writer = gettid_cached();
        return;
    }

    // Normal path with queuing if contention exists
    uint64_t our_order = get_our_order(waitq, priority);

    size_t spins = 0;
    usec_t usec = 1;
    usec_t deadlock_timestamp = 0;

    while(true) {
        while (write_our_priority(waitq, our_order)) {
            if(spinlock_trylock(&waitq->spinlock)) {
                waitq->writer = gettid_cached();
                clear_our_priority(waitq, our_order);
                worker_spinlock_contention(func, spins);
                return;
            }
            yield_the_processor();
        }

        // Back off
        spins++;
        
        // Check for deadlock every SPINS_BEFORE_DEADLOCK_CHECK iterations
        if ((spins % SPINS_BEFORE_DEADLOCK_CHECK) == 0) {
            spinlock_deadlock_detect(&deadlock_timestamp, "waitq", func);
        }
        
        microsleep(usec);
        usec = usec >= MAX_USEC ? MAX_USEC : usec * 2;
    }
}

ALWAYS_INLINE void waitq_release(WAITQ *waitq) {
    spinlock_unlock(&waitq->spinlock);
}

// --------------------------------------------------------------------------------------------------------------------

#define THREADS_PER_PRIORITY 2
#define TEST_DURATION_SEC 2

// For stress test statistics
typedef struct thread_stats {
    WAITQ_PRIORITY priority;
    size_t executions;           // how many times we got through
    usec_t total_wait_time;      // total time spent waiting
    usec_t max_wait_time;        // maximum time spent waiting
} THREAD_STATS;

struct thread_args {
    THREAD_STATS *stats;
    WAITQ *wq;
    bool with_sleep;
    bool *stop_flag;
};

static const char *priority_to_string(WAITQ_PRIORITY p) {
    switch(p) {
        case WAITQ_PRIO_URGENT: return "URGENT";
        case WAITQ_PRIO_HIGH:   return "HIGH";
        case WAITQ_PRIO_NORMAL: return "NORMAL";
        case WAITQ_PRIO_LOW:    return "LOW";
        default:                        return "UNKNOWN";
    }
}

static void stress_thread(void *arg) {
    struct thread_args *args = arg;

    THREAD_STATS *stats = args->stats;
    WAITQ *wq = args->wq;
    bool with_sleep = args->with_sleep;
    bool *stop_flag = args->stop_flag;

    while(!__atomic_load_n(stop_flag, __ATOMIC_ACQUIRE)) {
        usec_t waiting_since_ut = now_monotonic_usec();
        waitq_acquire(wq, stats->priority);
        usec_t wait_time = now_monotonic_usec() - waiting_since_ut;
        stats->executions++;
        stats->total_wait_time += wait_time;
        if(wait_time > stats->max_wait_time)
            stats->max_wait_time = wait_time;

        if(with_sleep)
            tinysleep();

        waitq_release(wq);
    }
}

static void print_thread_stats(THREAD_STATS *stats, size_t count, usec_t duration) {
    fprintf(stderr, "\n%-8s %12s %12s %12s %12s %12s\n",
            "PRIORITY", "EXECUTIONS", "EXEC/SEC", "AVG WAIT", "MAX WAIT", "% WAITING");

    size_t total_execs = 0;
    for(size_t i = 0; i < count; i++)
        total_execs += stats[i].executions;

    double total_time_sec = duration / (double)USEC_PER_SEC;

    for(size_t i = 0; i < count; i++) {
        double execs_per_sec = stats[i].executions / total_time_sec;
        double avg_wait = stats[i].executions ? (double)stats[i].total_wait_time / stats[i].executions : 0;
        double percent_waiting = stats[i].total_wait_time * 100.0 / duration;

        fprintf(stderr, "%-8s %12zu %12.1f %12.1f %12"PRIu64" %12.1f%%\n",
                priority_to_string(stats[i].priority),
                stats[i].executions,
                execs_per_sec,
                avg_wait,
                stats[i].max_wait_time,
                percent_waiting);
    }
}

static int unittest_stress(void) {
    int errors = 0;
    fprintf(stderr, "\nStress testing waiting queue...\n");

    WAITQ wq = WAITQ_INITIALIZER;
    const size_t num_priorities = 4;
    const size_t total_threads = num_priorities * THREADS_PER_PRIORITY;

    // Test both with and without sleep
    for(int test = 0; test < 2; test++) {
        bool with_sleep = (test == 1);
        bool stop_flag = false;

        fprintf(stderr, "\nRunning %ds stress test %s sleep:\n",
                TEST_DURATION_SEC, with_sleep ? "with" : "without");

        // Prepare thread stats and args
        THREAD_STATS stats[total_threads];
        struct thread_args thread_args[total_threads];
        ND_THREAD *threads[total_threads];

        fprintf(stderr, "Starting %zu threads for %ds test %s sleep...\n",
                total_threads,
                TEST_DURATION_SEC,
                with_sleep ? "with" : "without");

        // Initialize stats and create threads
        size_t thread_idx = 0;
        for(int prio = WAITQ_PRIO_URGENT; prio <= WAITQ_PRIO_LOW; prio++) {
            for(int t = 0; t < THREADS_PER_PRIORITY; t++) {
                stats[thread_idx] = (THREAD_STATS){
                    .priority = prio,
                    .executions = 0,
                    .total_wait_time = 0,
                    .max_wait_time = 0
                };
                thread_args[thread_idx] = (struct thread_args){
                    .stats = &stats[thread_idx],  // Pass pointer to stats
                    .wq = &wq,
                    .with_sleep = with_sleep,
                    .stop_flag = &stop_flag
                };

                char thread_name[32];
                snprintf(thread_name, sizeof(thread_name), "STRESS%d-%d", prio, t);
                threads[thread_idx] = nd_thread_create(
                    thread_name, NETDATA_THREAD_OPTION_DONT_LOG, stress_thread, &thread_args[thread_idx]);
                thread_idx++;
            }
        }

        // Let it run
        time_t start = now_monotonic_sec();
        fprintf(stderr, "Running...");
        while(now_monotonic_sec() - start < TEST_DURATION_SEC) {
            fprintf(stderr, ".");
            sleep_usec(500000); // Print a dot every 0.5 seconds
        }
        fprintf(stderr, "\n");


        fprintf(stderr, "Stopping threads...\n");
        __atomic_store_n(&stop_flag, true, __ATOMIC_RELEASE);

        // Wait for threads and collect stats
        fprintf(stderr, "Waiting for %zu threads to finish...\n", total_threads);
        for(size_t i = 0; i < total_threads; i++)
            nd_thread_join(threads[i]);

        // Print stats
        print_thread_stats(stats, total_threads, TEST_DURATION_SEC * USEC_PER_SEC);
    }

    waitq_destroy(&wq);
    return errors;
}

int unittest_waiting_queue(void) {
   int errors = unittest_stress();
   return errors;
}
