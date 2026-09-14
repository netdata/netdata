// SPDX-License-Identifier: GPL-3.0-or-later

#include "benchmark-rw.h"

#define MAX_THREADS 64
#define TEST_DURATION_SEC 1
#define STOP_SIGNAL UINT64_MAX
#define MAX_CONFIGS 20

// ----------------------------------------------------------------------------
// Runtime knobs, read from the environment so the benchmark stays reachable
// through the plain `netdata -W rwlockstest` entrypoint (no CLI plumbing).
//
//   RWBENCH_SECS            seconds per configuration          (default 1)
//   RWBENCH_CS_WORK         work units inside the critical section (default 0)
//   RWBENCH_PROBE_SECS      seconds per writer-latency probe    (default 5)
//   RWBENCH_WRITER_GAP_US   probe writer idle gap between acquisitions (default 1000)
//   RWBENCH_SKIP_MATRIX     set to 1 to run only the latency probe
//   RWBENCH_SKIP_PROBE      set to 1 to run only the throughput/latency matrix

static unsigned long env_ulong(const char *name, unsigned long def) {
    const char *v = getenv(name);
    if(!v || !*v) return def;

    char *end = NULL;
    errno_clear();
    unsigned long r = strtoul(v, &end, 10);
    if(errno || !end || *end) {
        fprintf(stderr, "RWBENCH: ignoring invalid %s='%s', using %lu\n", name, v, def);
        return def;
    }

    return r;
}

// ----------------------------------------------------------------------------
// Nanosecond clock.
//
// libnetdata's clocks API exposes microseconds only (clocks.h), which cannot
// resolve an uncontended read-lock acquisition (tens of nanoseconds). This is
// kept file-local instead of added to libnetdata/clocks because the benchmark is
// its only consumer; promote it if a second one appears.

static ALWAYS_INLINE nsec_t bench_now_nsec(void) {
    struct timespec ts;
    if(unlikely(clock_gettime(CLOCK_MONOTONIC, &ts) == -1))
        return 0;

    return (nsec_t)ts.tv_sec * NSEC_PER_SEC + (nsec_t)ts.tv_nsec;
}

// ----------------------------------------------------------------------------
// Latency histogram.
//
// Power-of-two buckets over nanoseconds: bucket i counts samples in
// [2^i, 2^(i+1)) ns. The question this benchmark answers is whether a lock
// acquisition costs tens of nanoseconds, ~50us (one nanosleep of timer slack) or
// ~512us (the top of the back-off ramp) - those are orders of magnitude apart,
// so power-of-two resolution is enough and needs no per-sample storage.
// Exact max and sum are kept alongside.

#define LAT_BUCKETS 48  // up to 2^48 ns ~ 78 hours

typedef struct {
    uint64_t count;
    uint64_t sum_ns;
    uint64_t max_ns;
    uint64_t buckets[LAT_BUCKETS];
} latency_hist_t;

static ALWAYS_INLINE void lat_record(latency_hist_t *h, nsec_t ns) {
    h->count++;
    h->sum_ns += ns;
    if(ns > h->max_ns) h->max_ns = ns;

    // bucket = floor(log2(ns)); 0 and 1 both land in bucket 0
    size_t b = 0;
    if(ns > 1) {
        b = (size_t)(63 - __builtin_clzll((unsigned long long)ns));
        if(b >= LAT_BUCKETS) b = LAT_BUCKETS - 1;
    }
    h->buckets[b]++;
}

static void lat_merge(latency_hist_t *dst, const latency_hist_t *src) {
    dst->count += src->count;
    dst->sum_ns += src->sum_ns;
    if(src->max_ns > dst->max_ns) dst->max_ns = src->max_ns;
    for(size_t i = 0; i < LAT_BUCKETS; i++)
        dst->buckets[i] += src->buckets[i];
}

// Upper bound (in ns) of the bucket holding the requested percentile.
static uint64_t lat_percentile_ns(const latency_hist_t *h, double p) {
    if(!h->count) return 0;

    uint64_t target = (uint64_t)((double)h->count * p);
    if(target < 1) target = 1;

    uint64_t seen = 0;
    for(size_t i = 0; i < LAT_BUCKETS; i++) {
        seen += h->buckets[i];
        if(seen >= target)
            return 2ULL << i;   // upper bound of [2^i, 2^(i+1))
    }

    return h->max_ns;
}

static double lat_mean_us(const latency_hist_t *h) {
    if(!h->count) return 0.0;
    return (double)h->sum_ns / (double)h->count / 1000.0;
}

static void lat_print_line(const char *label, const latency_hist_t *h) {
    fprintf(stderr, "%-26s %10"PRIu64" %10.3f %10.3f %10.3f %10.3f %12.3f\n",
            label,
            h->count,
            lat_mean_us(h),
            (double)lat_percentile_ns(h, 0.50) / 1000.0,
            (double)lat_percentile_ns(h, 0.99) / 1000.0,
            (double)lat_percentile_ns(h, 0.999) / 1000.0,
            (double)h->max_ns / 1000.0);
}

static void lat_print_header(const char *what) {
    fprintf(stderr, "\n%s (microseconds; percentiles are power-of-two bucket upper bounds)\n", what);
    fprintf(stderr, "%-26s %10s %10s %10s %10s %10s %12s\n",
            "SERIES", "SAMPLES", "MEAN", "P50<=", "P99<=", "P99.9<=", "MAX");
    fprintf(stderr, "-------------------------------------------------------------------------------------------------\n");
}

// Distribution dump - the bimodality of writer acquisition is the point, and a
// mean hides it completely.
static void lat_print_distribution(const latency_hist_t *h) {
    if(!h->count) return;

    fprintf(stderr, "    distribution: ");
    for(size_t i = 0; i < LAT_BUCKETS; i++) {
        if(!h->buckets[i]) continue;
        fprintf(stderr, "[<%.3gus]=%.1f%% ",
                (double)(2ULL << i) / 1000.0,
                (double)h->buckets[i] * 100.0 / (double)h->count);
    }
    fprintf(stderr, "\n");
}

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
        latency_hist_t lat;         // Lock-acquisition latency (when measured)
        uint64_t sink;              // Per-thread accumulator for critical-section work
    } stats[MAX_THREADS];

    // Per-run knobs, set by run_test() before the threads are released
    uint32_t cs_work;               // work units executed inside the critical section
    bool measure_latency;           // time every lock acquisition

    // Per-thread control
    struct {
        netdata_cond_t cond;              // Thread start condition
        netdata_mutex_t cond_mutex;       // Mutex for condition
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
    void *lock;                  // Points to either netdata_rwlock_t or RW_SPINLOCK
    bool is_spinlock;            // true for RW_SPINLOCK, false for netdata_rwlock_t
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

static void wait_for_start(netdata_cond_t *cond, netdata_mutex_t *mutex, uint64_t *flag) {
    netdata_mutex_lock(mutex);
    while (*flag == 0)
        netdata_cond_wait(cond, mutex);
    netdata_mutex_unlock(mutex);
}

// Work performed inside the critical section.
//
// Writers do the shared counter increment the original benchmark did - it is
// exclusive, so it is a legitimate write. Readers only read: the original code
// had every reader do control->counter++ under a READ lock, which is a genuine
// data race between concurrent readers and made the shared cacheline bounce for
// reasons unrelated to the lock being measured.
static ALWAYS_INLINE void do_critical_section_work(rwlock_control_t *control, thread_context_t *ctx) {
    if(ctx->type == THREAD_WRITER)
        control->counter++;

    for(uint32_t i = 0; i < control->cs_work; i++)
        control->stats[ctx->thread_id].sink += __atomic_load_n(&control->counter, __ATOMIC_RELAXED);
}

static void benchmark_thread(void *arg) {
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

        // The latency pass reads the clock twice per acquisition, which costs
        // more than an uncontended acquisition itself. That is why it is a
        // separate pass: the throughput pass below stays clock-free so its
        // numbers remain comparable.
        const bool measure = control->measure_latency;
        latency_hist_t *lat = &control->stats[ctx->thread_id].lat;

        while (control->thread_controls[ctx->thread_id].run_flag) {
            nsec_t t0 = measure ? bench_now_nsec() : 0;

            if(ctx->is_spinlock) {
                RW_SPINLOCK *spinlock = ctx->lock;
                if(ctx->type == THREAD_READER) {
                    rw_spinlock_read_lock(spinlock);
                    if(measure) lat_record(lat, bench_now_nsec() - t0);
                    check_access_safety(control, THREAD_READER);
                    do_critical_section_work(control, ctx);
                    release_access(control, THREAD_READER);
                    rw_spinlock_read_unlock(spinlock);
                }
                else {
                    rw_spinlock_write_lock(spinlock);
                    if(measure) lat_record(lat, bench_now_nsec() - t0);
                    check_access_safety(control, THREAD_WRITER);
                    do_critical_section_work(control, ctx);
                    release_access(control, THREAD_WRITER);
                    rw_spinlock_write_unlock(spinlock);
                }
            }
            else {
                netdata_rwlock_t *rwlock = ctx->lock;
                if(ctx->type == THREAD_READER) {
                    netdata_rwlock_rdlock(rwlock);
                    if(measure) lat_record(lat, bench_now_nsec() - t0);
                    check_access_safety(control, THREAD_READER);
                    do_critical_section_work(control, ctx);
                    release_access(control, THREAD_READER);
                    netdata_rwlock_rdunlock(rwlock);
                }
                else {
                    netdata_rwlock_wrlock(rwlock);
                    if(measure) lat_record(lat, bench_now_nsec() - t0);
                    check_access_safety(control, THREAD_WRITER);
                    do_critical_section_work(control, ctx);
                    release_access(control, THREAD_WRITER);
                    // was rdunlock(): a write lock must be released with
                    // wrunlock(), they are different libuv calls, so the
                    // netdata_rwlock writer column was invalid before this.
                    netdata_rwlock_wrunlock(rwlock);
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
}

static void print_summary(const summary_stats_t *summary) {
    fprintf(stderr, "\n=== Performance Summary (Million operations/sec) ===\n\n");
    fprintf(stderr, "%-16s %-8s %-8s %-16s %-16s\n",
            "Lock Type", "Readers", "Writers", "Reader Ops/s", "Writer Ops/s");
    fprintf(stderr, "----------------------------------------------------------------------\n");

    const char *lock_names[] = {"netdata_rwlock", "rw_spinlock"};

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

    if(control->measure_latency) {
        latency_hist_t rd = {0}, wr = {0};
        for(int i = 0; i < readers + writers; i++) {
            if(contexts[i].type == THREAD_READER)
                lat_merge(&rd, &control->stats[i].lat);
            else
                lat_merge(&wr, &control->stats[i].lat);
        }

        lat_print_header("  acquisition latency");
        if(rd.count) lat_print_line("  readers", &rd);
        if(wr.count) {
            lat_print_line("  writers", &wr);
            lat_print_distribution(&wr);
        }
    }

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

    // Reset all stats and control (clears the per-thread latency histograms too)
    memset(&control->stats, 0, sizeof(control->stats));
    control->counter = 0;
    control->readers = 0;
    control->writers = 0;
    control->violations = 0;

    int total_threads = readers + writers;

    // Signal threads to start
    for(int i = 0; i < total_threads; i++) {
        netdata_mutex_lock(&control->thread_controls[i].cond_mutex);
        control->thread_controls[i].run_flag = 1;
        netdata_cond_signal(&control->thread_controls[i].cond);
        netdata_mutex_unlock(&control->thread_controls[i].cond_mutex);
    }

    // Wait for test duration
    sleep_usec(env_ulong("RWBENCH_SECS", TEST_DURATION_SEC) * USEC_PER_SEC);

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


// ====================================================================================================================
// Layer 2 probe: writer acquisition latency under continuous reader churn.
//
// This is the scenario the writer-priority change targets: one writer arrives
// periodically into a stream of readers that never stops. It answers two
// questions the throughput matrix cannot:
//
//   1. how long does a writer wait to acquire, at the tail, not the mean; and
//   2. what does the blocking reader path cost while a writer is pending.
//
// The readers use the BLOCKING rw_spinlock_read_lock() deliberately. The
// rw-spinlock unittest churns with rw_spinlock_tryread_lock(), which returns
// immediately and therefore never exercises the reader back-off ramp
// (microsleep 1us -> 512us) that a pending writer forces readers onto.

#define PROBE_MAX_READERS 32

typedef struct {
    void *lock;
    bool is_spinlock;
    uint32_t stop;
    uint32_t gap_us;        // writer only: idle time between acquisitions
    uint32_t finished;      // writer only: set when the thread has exited its loop
    uint64_t ops;
    latency_hist_t lat;
} probe_ctx_t;

static void probe_reader_thread(void *arg) {
    probe_ctx_t *ctx = (probe_ctx_t *)arg;

    while(!__atomic_load_n(&ctx->stop, __ATOMIC_ACQUIRE)) {
        nsec_t t0 = bench_now_nsec();

        if(ctx->is_spinlock) {
            rw_spinlock_read_lock((RW_SPINLOCK *)ctx->lock);
            lat_record(&ctx->lat, bench_now_nsec() - t0);
            rw_spinlock_read_unlock((RW_SPINLOCK *)ctx->lock);
        }
        else {
            netdata_rwlock_rdlock((netdata_rwlock_t *)ctx->lock);
            lat_record(&ctx->lat, bench_now_nsec() - t0);
            netdata_rwlock_rdunlock((netdata_rwlock_t *)ctx->lock);
        }

        ctx->ops++;
    }
}

static void probe_writer_thread(void *arg) {
    probe_ctx_t *ctx = (probe_ctx_t *)arg;

    while(!__atomic_load_n(&ctx->stop, __ATOMIC_ACQUIRE)) {
        // Stay out of the lock between acquisitions, so every sample measures a
        // writer ARRIVING into an established reader stream. Back-to-back
        // acquisitions would keep the lock writer-held and measure nothing.
        microsleep(ctx->gap_us);

        nsec_t t0 = bench_now_nsec();

        if(ctx->is_spinlock) {
            rw_spinlock_write_lock((RW_SPINLOCK *)ctx->lock);
            lat_record(&ctx->lat, bench_now_nsec() - t0);
            rw_spinlock_write_unlock((RW_SPINLOCK *)ctx->lock);
        }
        else {
            netdata_rwlock_wrlock((netdata_rwlock_t *)ctx->lock);
            lat_record(&ctx->lat, bench_now_nsec() - t0);
            netdata_rwlock_wrunlock((netdata_rwlock_t *)ctx->lock);
        }

        ctx->ops++;
    }

    __atomic_store_n(&ctx->finished, 1, __ATOMIC_RELEASE);
}

static void run_writer_latency_probe(const char *lock_name, void *lock, bool is_spinlock,
                                     int nreaders, unsigned long secs, unsigned long gap_us) {
    probe_ctx_t readers[PROBE_MAX_READERS];
    probe_ctx_t writer = { .lock = lock, .is_spinlock = is_spinlock, .gap_us = (uint32_t)gap_us };
    ND_THREAD *reader_threads[PROBE_MAX_READERS];
    char thr_name[32];

    fprintf(stderr, "\n%s: 1 writer (every %luus) vs %d churning readers, %lus\n",
            lock_name, gap_us, nreaders, secs);

    usec_t readers_started_ut = now_monotonic_usec();
    for(int i = 0; i < nreaders; i++) {
        readers[i] = (probe_ctx_t){ .lock = lock, .is_spinlock = is_spinlock };
        snprintf(thr_name, sizeof(thr_name), "probe_rd%d", i);
        reader_threads[i] = nd_thread_create(thr_name, NETDATA_THREAD_OPTION_DONT_LOG,
                                             probe_reader_thread, &readers[i]);
    }

    // Let the reader stream establish itself before the writer arrives, so the
    // first samples are not measuring an empty lock.
    sleep_usec(100 * USEC_PER_MS);

    ND_THREAD *writer_thread = nd_thread_create("probe_wr", NETDATA_THREAD_OPTION_DONT_LOG,
                                                probe_writer_thread, &writer);

    sleep_usec(secs * USEC_PER_SEC);

    // Stop the writer first: readers must outlive it, otherwise the last writer
    // acquisition is measured against a draining reader set instead of a full one.
    __atomic_store_n(&writer.stop, 1, __ATOMIC_RELEASE);

    // The writer can be parked inside write_lock. On an implementation that
    // starves writers it never returns while the readers churn, so joining it
    // here would hang the benchmark forever - which IS the finding, but it has
    // to be reported rather than hung on. Give it a bounded grace period, then
    // release the readers so it can always complete.
    usec_t grace_deadline = now_monotonic_usec() + 5 * USEC_PER_SEC;
    while(!__atomic_load_n(&writer.finished, __ATOMIC_ACQUIRE) &&
          now_monotonic_usec() < grace_deadline)
        yield_the_processor();

    bool writer_starved = !__atomic_load_n(&writer.finished, __ATOMIC_ACQUIRE);

    for(int i = 0; i < nreaders; i++)
        __atomic_store_n(&readers[i].stop, 1, __ATOMIC_RELEASE);

    nd_thread_join(writer_thread);
    for(int i = 0; i < nreaders; i++)
        nd_thread_join(reader_threads[i]);

    // Measured, not nominal: when the writer starves, the readers keep running
    // through the whole grace period, and dividing by `secs` would understate
    // their throughput by exactly that much - in the one case we care about.
    usec_t readers_ran_ut = now_monotonic_usec() - readers_started_ut;

    latency_hist_t rd = {0};
    uint64_t reader_ops = 0;
    for(int i = 0; i < nreaders; i++) {
        lat_merge(&rd, &readers[i].lat);
        reader_ops += readers[i].ops;
    }

    if(writer_starved)
        fprintf(stderr, "    *** WRITER STARVED: still parked in the lock 5s after stop;"
                        " its last sample only completed once the readers were stopped ***\n");

    lat_print_header("  latency");
    lat_print_line("  writer acquire", &writer.lat);
    lat_print_distribution(&writer.lat);
    lat_print_line("  reader acquire", &rd);
    fprintf(stderr, "    reader throughput: %.3f M ops/sec across %d readers (over %.2fs measured)\n",
            (double)reader_ops * USEC_PER_SEC / (double)readers_ran_ut / 1000000.0,
            nreaders,
            (double)readers_ran_ut / (double)USEC_PER_SEC);

    if(is_spinlock) {
        RW_SPINLOCK *sp = (RW_SPINLOCK *)lock;
        if(sp->counter != 0 || sp->writer != 0)
            fprintf(stderr, "    FATAL: lock not free after probe (counter=%u writer=%d)\n",
                    sp->counter, sp->writer);
    }
}

static void writer_latency_probe(void) {
    unsigned long secs = env_ulong("RWBENCH_PROBE_SECS", 5);
    unsigned long gap_us = env_ulong("RWBENCH_WRITER_GAP_US", 1000);
    int reader_counts[] = { 1, 4, 8, 16, 32 };

    fprintf(stderr, "\n\n====================================================================\n");
    fprintf(stderr, "WRITER ACQUISITION LATENCY UNDER READER CHURN\n");
    fprintf(stderr, "====================================================================\n");

    for(size_t i = 0; i < sizeof(reader_counts) / sizeof(reader_counts[0]); i++) {
        int nreaders = reader_counts[i];

        RW_SPINLOCK rw_spinlock = RW_SPINLOCK_INITIALIZER;
        run_writer_latency_probe("rw_spinlock", &rw_spinlock, true, nreaders, secs, gap_us);

        netdata_rwlock_t rwlock;
        netdata_rwlock_init(&rwlock);
        run_writer_latency_probe("netdata_rwlock", &rwlock, false, nreaders, secs, gap_us);
        netdata_rwlock_destroy(&rwlock);
    }
}

int rwlocks_stress_test(void) {
    netdata_rwlock_t netdata_rwlock;
    netdata_rwlock_init(&netdata_rwlock);

    RW_SPINLOCK rw_spinlock = RW_SPINLOCK_INITIALIZER;
    summary_stats_t summary = {0};

    // Initialize control structures
    rwlock_control_t netdata_control = { 0 };
    rwlock_control_t spinlock_control = { 0 };

    // Initialize per-thread controls for both locks
    for(int i = 0; i < MAX_THREADS; i++) {
        netdata_cond_init(&netdata_control.thread_controls[i].cond);
        netdata_mutex_init(&netdata_control.thread_controls[i].cond_mutex);
        netdata_control.thread_controls[i].run_flag = 0;

        netdata_cond_init(&spinlock_control.thread_controls[i].cond);
        netdata_mutex_init(&spinlock_control.thread_controls[i].cond_mutex);

        spinlock_control.thread_controls[i].run_flag = 0;
    }

    // Create thread contexts
    thread_context_t netdata_contexts[MAX_THREADS];
    thread_context_t spinlock_contexts[MAX_THREADS];

    fprintf(stderr, "\nStarting RW locks benchmark...\n");

    // Test configurations: [readers, writers]
    int configs[][2] = {
        {1, 0},   // Single reader                  - uncontended reader guard
        {0, 1},   // Single writer                  - uncontended writer guard
        {1, 1},   // One reader + one writer
        {2, 1},   // Two readers + one writer
        {1, 2},   // One reader + two writers
        {2, 2},   // Two readers + two writers
        {4, 1},   // Four readers + one writer
        {1, 4},   // One reader + four writers      - writer->writer handoff
        {4, 4},   // Four readers + four writers
        {8, 1},   // Eight readers + one writer     - reader pressure on one writer
        {16, 1},  // 16 readers + one writer
        {32, 1},  // 32 readers + one writer        - oversubscribed below 33 cores
        {16, 4},  // 16 readers + four writers      - reader pressure + handoff
    };

    const int num_configs = sizeof(configs) / sizeof(configs[0]);
    summary.config_count = num_configs;
    const bool skip_matrix = env_ulong("RWBENCH_SKIP_MATRIX", 0) != 0;
    const bool skip_probe  = env_ulong("RWBENCH_SKIP_PROBE", 0) != 0;

    // Create all threads
    for(int i = 0; i < MAX_THREADS; i++) {
        char thr_name[32];

        // Initialize pthread contexts
        netdata_contexts[i] = (thread_context_t){
            .thread_id = i,
            .type = i % 2 == 0 ? THREAD_READER :THREAD_WRITER,
            .lock = &netdata_rwlock,
            .is_spinlock = false,
            .control = &netdata_control
        };

        snprintf(thr_name, sizeof(thr_name), "netdata_rw%d", i);
        netdata_contexts[i].thread =
            nd_thread_create(thr_name, NETDATA_THREAD_OPTION_DONT_LOG, benchmark_thread, &netdata_contexts[i]);

        // Initialize spinlock contexts
        spinlock_contexts[i] = (thread_context_t){
            .thread_id = i,
            .type = i % 2 == 0 ? THREAD_READER : THREAD_WRITER,
            .lock = &rw_spinlock,
            .is_spinlock = true,
            .control = &spinlock_control
        };

        snprintf(thr_name, sizeof(thr_name), "spin_rw%d", i);
        spinlock_contexts[i].thread =
            nd_thread_create(thr_name, NETDATA_THREAD_OPTION_DONT_LOG, benchmark_thread, &spinlock_contexts[i]);
    }

    // Two passes over the matrix. Pass 0 is clock-free and measures throughput.
    // Pass 1 times every acquisition: two clock reads per operation cost more
    // than an uncontended acquisition, so its throughput is NOT comparable with
    // pass 0 - only its latency distribution is meaningful.
    for(int pass = 0; pass < 2 && !skip_matrix; pass++) {
    bool measure_latency = (pass == 1);
    uint32_t cs_work = (uint32_t)env_ulong("RWBENCH_CS_WORK", 0);

    netdata_control.measure_latency = spinlock_control.measure_latency = measure_latency;
    netdata_control.cs_work = spinlock_control.cs_work = cs_work;
    memset(&summary, 0, sizeof(summary));
    summary.config_count = num_configs;

    fprintf(stderr, "\n\n====================================================================\n");
    fprintf(stderr, "PASS %d: %s (cs_work=%u)\n", pass,
            measure_latency ? "LATENCY (clock-instrumented, throughput not comparable)"
                            : "THROUGHPUT (clock-free)", cs_work);
    fprintf(stderr, "====================================================================\n");

    // Run all configurations
    for(int i = 0; i < num_configs; i++) {
        int readers = configs[i][0];
        int writers = configs[i][1];

        // Create all threads
        int thread_idx = 0;

        // First assign reader threads
        for(int r = 0; r < readers; r++) {
            netdata_contexts[thread_idx].type = THREAD_READER;
            spinlock_contexts[thread_idx].type = THREAD_READER;
            thread_idx++;
        }

        // Then assign writer threads
        for(int w = 0; w < writers; w++) {
            netdata_contexts[thread_idx].type = THREAD_WRITER;
            spinlock_contexts[thread_idx].type = THREAD_WRITER;
            thread_idx++;
        }

        char test_name[64];
        snprintf(test_name, sizeof(test_name), "netdata_rwlock %dR/%dW", readers, writers);
        run_test(test_name, readers, writers, netdata_contexts, &netdata_control, &summary, i, 0);

        snprintf(test_name, sizeof(test_name), "rw_spinlock %dR/%dW", readers, writers);
        run_test(test_name, readers, writers, spinlock_contexts, &spinlock_control, &summary, i, 1);
    }

    // Print the summary table
    print_summary(&summary);
    } // pass

    // Stop all threads
    fprintf(stderr, "\nStopping threads...\n");
    for(int i = 0; i < MAX_THREADS; i++) {
        // Signal pthread threads
        netdata_mutex_lock(&netdata_control.thread_controls[i].cond_mutex);
        netdata_control.thread_controls[i].run_flag = STOP_SIGNAL;
        netdata_cond_signal(&netdata_control.thread_controls[i].cond);
        netdata_mutex_unlock(&netdata_control.thread_controls[i].cond_mutex);

        // Signal spinlock threads
        netdata_mutex_lock(&spinlock_control.thread_controls[i].cond_mutex);
        spinlock_control.thread_controls[i].run_flag = STOP_SIGNAL;
        netdata_cond_signal(&spinlock_control.thread_controls[i].cond);
        netdata_mutex_unlock(&spinlock_control.thread_controls[i].cond_mutex);
    }

    // Join all threads
    fprintf(stderr, "\nWaiting for threads to exit...\n");
    for(int i = 0; i < MAX_THREADS; i++) {
        nd_thread_join(netdata_contexts[i].thread);
        nd_thread_join(spinlock_contexts[i].thread);
    }

    // Cleanup condition variables and mutexes
    for(int i = 0; i < MAX_THREADS; i++) {
        netdata_cond_destroy(&netdata_control.thread_controls[i].cond);
        netdata_mutex_destroy(&netdata_control.thread_controls[i].cond_mutex);
        netdata_cond_destroy(&spinlock_control.thread_controls[i].cond);
        netdata_mutex_destroy(&spinlock_control.thread_controls[i].cond_mutex);
    }

    netdata_rwlock_destroy(&netdata_rwlock);

    if(!skip_probe)
        writer_latency_probe();

    return 0;
}
