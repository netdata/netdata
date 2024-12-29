// SPDX-License-Identifier: GPL-3.0-or-later

#include "waiting-queue.h"

typedef struct waiting_thread {
    uv_cond_t cond;          // condition variable for this thread
    usec_t waiting_since_ut;    // when we started waiting
    Word_t order;
    struct waiting_thread *prev, *next;
} WAITING_THREAD;

struct waiting_queue {
    uv_mutex_t mutex;           // protect the queue structure
    Word_t last_seqno;          // incrementing sequence counter
    WAITING_THREAD *list;
    size_t running;             // number of threads currently running/waiting
    SPINLOCK spinlock;
};

// Determine available bits based on system word size
#if SIZEOF_VOID_P == 8
#define PRIORITY_SHIFT 62ULL
#define SEQNO_MASK ((1ULL << PRIORITY_SHIFT) - 1)
#else
#define PRIORITY_SHIFT 30U
#define SEQNO_MASK ((1U << PRIORITY_SHIFT) - 1)
#endif

static inline Word_t make_key(WAITING_QUEUE_PRIORITY priority, Word_t seqno) {
    return ((Word_t)priority << PRIORITY_SHIFT) | (seqno & SEQNO_MASK);
}

static inline WAITING_QUEUE_PRIORITY key_get_priority(Word_t key) {
    return (WAITING_QUEUE_PRIORITY)(key >> PRIORITY_SHIFT);
}

static inline Word_t key_get_seqno(Word_t key) {
    return key & SEQNO_MASK;
}

WAITING_QUEUE *waiting_queue_create(void) {
    WAITING_QUEUE *wq = callocz(1, sizeof(WAITING_QUEUE));

    int ret = uv_mutex_init(&wq->mutex);
    if(ret != 0) {
        freez(wq);
        return NULL;
    }

    spinlock_init(&wq->spinlock);

    wq->running = 0;
    return wq;
}

void waiting_queue_destroy(WAITING_QUEUE *wq) {
    if(!wq) return;

    if(wq->running)
        fatal("WAITING_QUEUE: destroying waiting queue that still has %zu threads running/waiting", wq->running);

    uv_mutex_destroy(&wq->mutex);
    freez(wq);
}

static inline void WAITERS_SET(WAITING_QUEUE *wq, WAITING_THREAD *wt) {
    for(WAITING_THREAD *t = wq->list ; t ;t = t->next) {
        if(wt->order < t->order) {
            DOUBLE_LINKED_LIST_INSERT_ITEM_BEFORE_UNSAFE(wq->list, t, wt, prev, next);
            return;
        }
    }
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(wq->list, wt, prev, next);
}

static inline void WAITERS_DEL(WAITING_QUEUE *wq, WAITING_THREAD *wt) {
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wq->list, wt, prev, next);
}

static inline WAITING_THREAD *WAITERS_FIRST(WAITING_QUEUE *wq) {
    return wq->list;
}

static inline void WAITING_THREAD_init(WAITING_QUEUE *wq, WAITING_THREAD *wt, WAITING_QUEUE_PRIORITY priority) {
    Word_t seqno = __atomic_add_fetch(&wq->last_seqno, 1, __ATOMIC_RELAXED);
    wt->order = make_key(priority, seqno);
    wt->waiting_since_ut = now_monotonic_usec();
    wt->prev = wt->next = NULL;

    int ret = uv_cond_init(&wt->cond);
    if(ret != 0)
        fatal("WAITING_QUEUE: cannot initialize condition variable");
}

static inline void WAITING_THREAD_cleanup(WAITING_QUEUE *wq __maybe_unused, WAITING_THREAD *wt) {
    uv_cond_destroy(&wt->cond);
}

usec_t waiting_queue_wait(WAITING_QUEUE *wq, WAITING_QUEUE_PRIORITY priority) {
    // Try fast path first - if we're the only one, just go
    if(__atomic_add_fetch(&wq->running, 1, __ATOMIC_RELAXED) == 1) {
        if(spinlock_trylock(&wq->spinlock))
            return 0;
    }

    // Slow path - need to wait

    WAITING_THREAD wt;
    WAITING_THREAD_init(wq, &wt, priority);

    uv_mutex_lock(&wq->mutex);
    WAITERS_SET(wq, &wt);

    // Wait for our turn
    do {
        if (WAITERS_FIRST(wq) == &wt && spinlock_trylock(&wq->spinlock))
            break;
        else
            uv_cond_wait(&wt.cond, &wq->mutex);
    } while(true);

    WAITERS_DEL(wq, &wt);
    uv_mutex_unlock(&wq->mutex);
    WAITING_THREAD_cleanup(wq, &wt);

    return now_monotonic_usec() - wt.waiting_since_ut;
}

void waiting_queue_done(WAITING_QUEUE *wq) {
    spinlock_unlock(&wq->spinlock);

    // Fast path if we're alone
    if(__atomic_sub_fetch(&wq->running, 1, __ATOMIC_RELAXED) == 0)
        return;

    // Slow path - need to signal next in line
    uv_mutex_lock(&wq->mutex);

    // Wake up next in line if any
    if(wq->list)
        uv_cond_signal(&wq->list->cond);

    uv_mutex_unlock(&wq->mutex);
}

size_t waiting_queue_waiting(WAITING_QUEUE *wq) {
    return __atomic_load_n(&wq->running, __ATOMIC_RELAXED);
}


// --------------------------------------------------------------------------------------------------------------------

// For stress test statistics
typedef struct thread_stats {
    WAITING_QUEUE_PRIORITY priority;
    size_t executions;           // how many times we got through
    usec_t total_wait_time;      // total time spent waiting
    usec_t max_wait_time;        // maximum time spent waiting
} THREAD_STATS;

struct thread_args {
    THREAD_STATS *stats;
    WAITING_QUEUE *wq;
    bool with_sleep;
    bool *stop_flag;
};

static const char *priority_to_string(WAITING_QUEUE_PRIORITY p) {
    switch(p) {
        case WAITING_QUEUE_PRIO_URGENT: return "URGENT";
        case WAITING_QUEUE_PRIO_HIGH:   return "HIGH";
        case WAITING_QUEUE_PRIO_NORMAL: return "NORMAL";
        case WAITING_QUEUE_PRIO_LOW:    return "LOW";
        default:                        return "UNKNOWN";
    }
}

static int unittest_functional(void) {
    int errors = 0;
    fprintf(stderr, "\nTesting waiting queue...\n");

    WAITING_QUEUE *wq = waiting_queue_create();

    // Test 1: Fast path should work with no contention
    fprintf(stderr, "  Test 1: Fast path - no contention: ");
    usec_t wait_time = waiting_queue_wait(wq, WAITING_QUEUE_PRIO_NORMAL);
    waiting_queue_done(wq);
    if(wait_time != 0) {
        fprintf(stderr, "FAILED (waited %"PRIu64" usec)\n", wait_time);
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    // Test 2: Priorities should be respected
    fprintf(stderr, "  Test 2: Priority ordering: ");
    WAITING_THREAD threads[100];
    for(size_t t = 0; t < _countof(threads); t++) {
        __atomic_add_fetch(&wq->running, 1, __ATOMIC_RELAXED);
        WAITING_THREAD_init(wq, &threads[t], os_random(WAITING_QUEUE_PRIO_MAX));
        WAITERS_SET(wq, &threads[t]);
    }

    bool failed = false;
    size_t prio_counts[WAITING_QUEUE_PRIO_MAX] = { 0 };
    WAITING_QUEUE_PRIORITY last_prio = WAITING_QUEUE_PRIO_URGENT;
    Word_t last_seqno = 0;
    for(size_t t = 0; t < _countof(threads); t++) {
        WAITING_THREAD *wt = WAITERS_FIRST(wq);
        WAITERS_DEL(wq, wt);
        __atomic_sub_fetch(&wq->running, 1, __ATOMIC_RELAXED);

        WAITING_QUEUE_PRIORITY prio = key_get_priority(wt->order);
        Word_t seqno = key_get_seqno(wt->order);

        prio_counts[prio]++;
        if(prio < last_prio) {
            if(!failed)
                fprintf(stderr, "FAILED\n");

            fprintf(stderr, " > ERROR: prio %u is before prio %u\n", prio, last_prio);
            errors++;
            failed = true;
        }
        else if(prio == last_prio && seqno < last_seqno) {
            if(!failed)
                fprintf(stderr, "FAILED\n");

            fprintf(stderr, " > ERROR: seqno %lu is before seqno %lu\n", seqno, last_seqno);
            errors++;
            failed = true;
        }

        last_seqno = seqno;
        last_prio = prio;
        WAITING_THREAD_cleanup(wq, wt);
    }

    if(!failed)
        fprintf(stderr, "OK\n");

    for(size_t p = 0; p < WAITING_QUEUE_PRIO_MAX ;p++)
        fprintf(stderr, "     > prio %zu got %zu waiters\n", p, prio_counts[p]);

    // Test 3: Queue stats should be accurate
    fprintf(stderr, "  Test 3: Queue statistics: ");
    size_t waiting = waiting_queue_waiting(wq);
    if(waiting != 0) {
        fprintf(stderr, "FAILED (queue shows %zu waiting)\n", waiting);
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    waiting_queue_destroy(wq);
    return errors;
}

static void *stress_thread(void *arg) {
    struct thread_args *args = arg;

    THREAD_STATS *stats = args->stats;
    WAITING_QUEUE *wq = args->wq;
    bool with_sleep = args->with_sleep;
    bool *stop_flag = args->stop_flag;

    while(!__atomic_load_n(stop_flag, __ATOMIC_ACQUIRE)) {
        usec_t wait_time = waiting_queue_wait(wq, stats->priority);
        stats->executions++;
        stats->total_wait_time += wait_time;
        if(wait_time > stats->max_wait_time)
            stats->max_wait_time = wait_time;

        if(with_sleep)
            tinysleep();

        waiting_queue_done(wq);
    }

    return NULL;
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

#define THREADS_PER_PRIORITY 2
#define TEST_DURATION_SEC 5

static int unittest_stress(void) {
    int errors = 0;
    fprintf(stderr, "\nStress testing waiting queue...\n");

    WAITING_QUEUE *wq = waiting_queue_create();
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
        for(int prio = WAITING_QUEUE_PRIO_URGENT; prio <= WAITING_QUEUE_PRIO_LOW; prio++) {
            for(int t = 0; t < THREADS_PER_PRIORITY; t++) {
                stats[thread_idx] = (THREAD_STATS){
                    .priority = prio,
                    .executions = 0,
                    .total_wait_time = 0,
                    .max_wait_time = 0
                };
                thread_args[thread_idx] = (struct thread_args){
                    .stats = &stats[thread_idx],  // Pass pointer to stats
                    .wq = wq,
                    .with_sleep = with_sleep,
                    .stop_flag = &stop_flag
                };

                char thread_name[32];
                snprintf(thread_name, sizeof(thread_name), "STRESS%d-%d", prio, t);
                threads[thread_idx] = nd_thread_create(
                    thread_name,
                    NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_JOINABLE,
                    stress_thread,
                    &thread_args[thread_idx]);
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

//        // Basic validation
//        for(size_t i = 0; i < total_threads - THREADS_PER_PRIORITY; i++) {
//            if(stats[i].executions < stats[i + THREADS_PER_PRIORITY].executions) {
//                fprintf(stderr, "ERROR: Higher priority thread got fewer executions!\n");
//                errors++;
//            }
//        }
//
//        // Check fairness within same priority
//        for(size_t i = 0; i < total_threads; i += THREADS_PER_PRIORITY) {
//            for(size_t j = i + 1; j < i + THREADS_PER_PRIORITY; j++) {
//                double diff = (double)(stats[i].executions - stats[j].executions) /
//                              (double)(stats[i].executions + stats[j].executions);
//                if(fabs(diff) > 0.1) {  // allow 10% difference
//                    fprintf(stderr, "ERROR: Unfair distribution within same priority!\n");
//                    errors++;
//                }
//            }
//        }
    }

    waiting_queue_destroy(wq);
    return errors;
}

int unittest_waiting_queue(void) {
   int errors = unittest_functional();
   errors += unittest_stress();

   return errors;
}
