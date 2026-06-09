// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

// Deterministic unit tests for the baseline rw-spinlock API semantics that
// already hold on master. These tests intentionally avoid timing/sleep-based
// concurrency assertions (kept in the rwlockstest benchmark) so they stay
// reproducible. They also intentionally do NOT assert writer-priority,
// writer-starvation prevention, or writer-pending reader blocking; those
// behaviors belong with the PR that introduces them.

#define RW_TEST(condition, msg) do {                                            \
        if (!(condition)) {                                                     \
            fprintf(stderr, "rw-spinlock unittest FAILED: %s (%s:%d)\n",        \
                    (msg), __FUNCTION__, __LINE__);                             \
            errors++;                                                           \
        }                                                                       \
    } while(0)

// Writer-priority thread: blocks on the write lock (parking behind a reader the
// caller holds), then immediately releases once it acquires.
static void rw_spinlock_wp_writer(void *arg) {
    RW_SPINLOCK *lock = (RW_SPINLOCK *)arg;
    rw_spinlock_write_lock(lock);
    rw_spinlock_write_unlock(lock);
}

// Writer-priority invariant (multi-threaded, deterministic): while a writer is
// parked waiting for an existing reader to drain, new readers must back off and
// must not be admitted. This is the no-writer-starvation guarantee. It cannot be
// observed single-threaded, and it does NOT hold on the pre-writer-priority
// implementation (which cleared the writer bit and retried, admitting readers).
static int rw_spinlock_writer_priority_test(void) {
    int errors = 0;
    RW_SPINLOCK lock = RW_SPINLOCK_INITIALIZER;

    fprintf(stderr, "  - writer-priority invariant: new readers back off while a writer is pending\n");

    // hold a reader so an incoming writer must wait
    rw_spinlock_read_lock(&lock);

    ND_THREAD *writer = nd_thread_create("rwsp_wp", NETDATA_THREAD_OPTION_DONT_LOG,
                                         rw_spinlock_wp_writer, &lock);

    // Wait until the writer has parked. Detected purely through the public API:
    // once the writer marks itself pending, tryread starts failing. Bounded
    // deadline is only a hang-guard, not a correctness signal.
    bool writer_parked = false;
    usec_t deadline = now_monotonic_usec() + 5 * USEC_PER_SEC;
    while (now_monotonic_usec() < deadline) {
        if (rw_spinlock_tryread_lock(&lock)) {
            // still admitted => writer not pending yet; release and retry
            rw_spinlock_read_unlock(&lock);
        }
        else {
            writer_parked = true;
            break;
        }
    }
    RW_TEST(writer_parked, "a writer parks (pending) while a reader is held");

    // While the writer is pending and our reader is still held, no new reader may
    // be admitted. The held reader keeps the writer parked, so the pending state
    // is stable for the whole burst.
    int admitted = 0;
    for (int i = 0; i < 100000; i++) {
        if (rw_spinlock_tryread_lock(&lock)) {
            admitted++;
            rw_spinlock_read_unlock(&lock);
        }
    }
    RW_TEST(admitted == 0, "no reader admitted while a writer is pending (writer-priority)");

    // release our reader so the parked writer can finally acquire and exit
    rw_spinlock_read_unlock(&lock);
    nd_thread_join(writer);

    RW_TEST(lock.counter == 0, "lock is free after the writer released");
    RW_TEST(lock.writer == 0, "writer tid cleared after release");

    return errors;
}

// ----------------------------------------------------------------------------
// Multiple writers serialize: each writer increments a plain (non-atomic) shared
// counter under the write lock. The final value can only equal the expected
// total if the lock provides true mutual exclusion among concurrent writers; a
// lost update means writers overlapped.

#define RW_SPINLOCK_TEST_WRITERS 8
#define RW_SPINLOCK_TEST_INCREMENTS 10000

typedef struct {
    RW_SPINLOCK *lock;
    int *shared;      // intentionally non-atomic; protected only by the lock
    int increments;
} rw_spinlock_mutex_ctx_t;

static void rw_spinlock_mutex_writer(void *arg) {
    rw_spinlock_mutex_ctx_t *ctx = (rw_spinlock_mutex_ctx_t *)arg;
    for (int i = 0; i < ctx->increments; i++) {
        rw_spinlock_write_lock(ctx->lock);
        (*ctx->shared)++;
        rw_spinlock_write_unlock(ctx->lock);
    }
}

static int rw_spinlock_writer_mutex_test(void) {
    int errors = 0;
    RW_SPINLOCK lock = RW_SPINLOCK_INITIALIZER;
    int shared = 0;

    fprintf(stderr, "  - mutual exclusion: %d concurrent writers must produce no lost updates\n",
            RW_SPINLOCK_TEST_WRITERS);

    rw_spinlock_mutex_ctx_t ctx = {
        .lock = &lock,
        .shared = &shared,
        .increments = RW_SPINLOCK_TEST_INCREMENTS,
    };

    ND_THREAD *threads[RW_SPINLOCK_TEST_WRITERS];
    for (int i = 0; i < RW_SPINLOCK_TEST_WRITERS; i++)
        threads[i] = nd_thread_create("rwsp_mx", NETDATA_THREAD_OPTION_DONT_LOG,
                                      rw_spinlock_mutex_writer, &ctx);
    for (int i = 0; i < RW_SPINLOCK_TEST_WRITERS; i++)
        nd_thread_join(threads[i]);

    RW_TEST(shared == RW_SPINLOCK_TEST_WRITERS * RW_SPINLOCK_TEST_INCREMENTS,
            "concurrent writers serialize with no lost updates");
    RW_TEST(lock.counter == 0, "lock is free after concurrent writers");
    RW_TEST(lock.writer == 0, "writer tid cleared after concurrent writers");

    return errors;
}

// ----------------------------------------------------------------------------
// Writer liveness under reader churn: a pool of readers continuously acquires
// and releases the read lock while a single writer blocks on the write lock.
// Under writer-priority the writer must acquire promptly despite the churn (the
// robust assertion). On the pre-writer-priority implementation a writer can be
// starved by the reader stream; this test would then fail at the deadline, but
// because starvation is timing-dependent this fail-on-old-master signal is
// best-effort, not deterministic (the deterministic guards are the back-off
// invariant and mutual-exclusion tests).

#define RW_SPINLOCK_TEST_READERS 4

typedef struct {
    RW_SPINLOCK *lock;
    uint32_t stop;
} rw_spinlock_churn_ctx_t;

typedef struct {
    RW_SPINLOCK *lock;
    uint32_t acquired;
} rw_spinlock_liveness_writer_ctx_t;

static void rw_spinlock_reader_churn(void *arg) {
    rw_spinlock_churn_ctx_t *ctx = (rw_spinlock_churn_ctx_t *)arg;
    while (!__atomic_load_n(&ctx->stop, __ATOMIC_ACQUIRE)) {
        if (rw_spinlock_tryread_lock(ctx->lock))
            rw_spinlock_read_unlock(ctx->lock);
    }
}

static void rw_spinlock_liveness_writer(void *arg) {
    rw_spinlock_liveness_writer_ctx_t *ctx = (rw_spinlock_liveness_writer_ctx_t *)arg;
    rw_spinlock_write_lock(ctx->lock);
    __atomic_store_n(&ctx->acquired, 1, __ATOMIC_RELEASE);
    rw_spinlock_write_unlock(ctx->lock);
}

static int rw_spinlock_writer_liveness_test(void) {
    int errors = 0;
    RW_SPINLOCK lock = RW_SPINLOCK_INITIALIZER;
    rw_spinlock_churn_ctx_t rctx = { .lock = &lock, .stop = 0 };
    rw_spinlock_liveness_writer_ctx_t wctx = { .lock = &lock, .acquired = 0 };

    fprintf(stderr, "  - writer liveness: a writer must acquire despite %d churning readers\n",
            RW_SPINLOCK_TEST_READERS);

    ND_THREAD *readers[RW_SPINLOCK_TEST_READERS];
    for (int i = 0; i < RW_SPINLOCK_TEST_READERS; i++)
        readers[i] = nd_thread_create("rwsp_rd", NETDATA_THREAD_OPTION_DONT_LOG,
                                      rw_spinlock_reader_churn, &rctx);

    ND_THREAD *writer = nd_thread_create("rwsp_wr", NETDATA_THREAD_OPTION_DONT_LOG,
                                         rw_spinlock_liveness_writer, &wctx);

    // Bounded wait for the writer to acquire. Generous deadline so this never
    // false-fails on writer-priority (acquisition takes microseconds there).
    bool acquired = false;
    usec_t deadline = now_monotonic_usec() + 5 * USEC_PER_SEC;
    while (now_monotonic_usec() < deadline) {
        if (__atomic_load_n(&wctx.acquired, __ATOMIC_ACQUIRE)) {
            acquired = true;
            break;
        }
    }
    RW_TEST(acquired, "writer acquires despite continuous reader churn (no starvation)");

    // Stop the readers so the writer is guaranteed to finish even if it was
    // starved past the deadline (the failure case), preventing any hang.
    __atomic_store_n(&rctx.stop, 1, __ATOMIC_RELEASE);
    nd_thread_join(writer);
    for (int i = 0; i < RW_SPINLOCK_TEST_READERS; i++)
        nd_thread_join(readers[i]);

    RW_TEST(lock.counter == 0, "lock is free after liveness test");
    RW_TEST(lock.writer == 0, "writer tid cleared after liveness test");

    return errors;
}

int rw_spinlock_unittest(void) {
    int errors = 0;

    fprintf(stderr, "\nrunning rw-spinlock unittest\n");

    // ----------------------------------------------------------------------
    // initialization: a freshly initialized lock has no writer and no readers,
    // and a static initializer produces the same state.
    {
        fprintf(stderr, "  - initialization: init() and the static initializer clear writer and counter\n");
        RW_SPINLOCK lock;
        rw_spinlock_init(&lock);
        RW_TEST(lock.writer == 0, "init clears writer");
        RW_TEST(lock.counter == 0, "init clears counter");

        RW_SPINLOCK lock_static = RW_SPINLOCK_INITIALIZER;
        RW_TEST(lock_static.writer == 0, "static initializer clears writer");
        RW_TEST(lock_static.counter == 0, "static initializer clears counter");
    }

    // ----------------------------------------------------------------------
    // tryread on a free lock succeeds, and read_unlock restores the free state.
    {
        fprintf(stderr, "  - tryread: succeeds on a free lock and read_unlock restores free state\n");
        RW_SPINLOCK lock = RW_SPINLOCK_INITIALIZER;
        RW_TEST(rw_spinlock_tryread_lock(&lock) == true, "tryread succeeds on free lock");
        rw_spinlock_read_unlock(&lock);
        RW_TEST(lock.counter == 0, "read_unlock restores free state");
    }

    // ----------------------------------------------------------------------
    // concurrent (and recursive) readers: a shared lock admits multiple
    // simultaneous read holders. With no writer held or pending, the same
    // thread can take the read lock repeatedly; each acquisition succeeds and
    // each unlock is balanced.
    {
        fprintf(stderr, "  - readers: multiple/recursive read holders allowed; writer excluded while readers held\n");
        RW_SPINLOCK lock = RW_SPINLOCK_INITIALIZER;
        const int readers = 5;

        for (int i = 0; i < readers; i++)
            RW_TEST(rw_spinlock_tryread_lock(&lock) == true, "additional reader admitted");

        // a writer must not be able to enter while readers are present
        RW_TEST(rw_spinlock_trywrite_lock(&lock) == false, "trywrite fails while readers held");

        for (int i = 0; i < readers; i++)
            rw_spinlock_read_unlock(&lock);

        RW_TEST(lock.counter == 0, "all readers released");

        // once readers are gone, a writer can enter
        RW_TEST(rw_spinlock_trywrite_lock(&lock) == true, "trywrite succeeds after readers drain");
        rw_spinlock_write_unlock(&lock);
        RW_TEST(lock.counter == 0, "write_unlock restores free state");
    }

    // ----------------------------------------------------------------------
    // blocking read_lock path (non-contended): read_lock/read_unlock balance
    // and admit multiple holders just like tryread.
    {
        fprintf(stderr, "  - read_lock: blocking read path balances and admits multiple holders\n");
        RW_SPINLOCK lock = RW_SPINLOCK_INITIALIZER;
        rw_spinlock_read_lock(&lock);
        rw_spinlock_read_lock(&lock);
        RW_TEST(rw_spinlock_trywrite_lock(&lock) == false, "trywrite fails while blocking readers held");
        rw_spinlock_read_unlock(&lock);
        rw_spinlock_read_unlock(&lock);
        RW_TEST(lock.counter == 0, "blocking readers released");
    }

    // ----------------------------------------------------------------------
    // trywrite on a free lock succeeds and records the writer; write_unlock
    // clears the writer and the writer bit.
    {
        fprintf(stderr, "  - trywrite: succeeds on a free lock, records writer tid, write_unlock clears it\n");
        RW_SPINLOCK lock = RW_SPINLOCK_INITIALIZER;
        RW_TEST(rw_spinlock_trywrite_lock(&lock) == true, "trywrite succeeds on free lock");
        RW_TEST(lock.writer == gettid_cached(), "trywrite records the writer tid");
        rw_spinlock_write_unlock(&lock);
        RW_TEST(lock.writer == 0, "write_unlock clears writer");
        RW_TEST(lock.counter == 0, "write_unlock restores free state");
    }

    // ----------------------------------------------------------------------
    // writer exclusivity: while a writer holds the lock, neither a reader nor
    // another writer may enter.
    {
        fprintf(stderr, "  - writer exclusivity: readers and other writers are excluded while a writer holds\n");
        RW_SPINLOCK lock = RW_SPINLOCK_INITIALIZER;
        rw_spinlock_write_lock(&lock);
        RW_TEST(rw_spinlock_tryread_lock(&lock) == false, "tryread fails while writer held");
        RW_TEST(rw_spinlock_trywrite_lock(&lock) == false, "second trywrite fails while writer held");
        rw_spinlock_write_unlock(&lock);

        // after release both a reader and a writer can enter again
        RW_TEST(rw_spinlock_tryread_lock(&lock) == true, "tryread succeeds after writer released");
        rw_spinlock_read_unlock(&lock);
        RW_TEST(rw_spinlock_trywrite_lock(&lock) == true, "trywrite succeeds after writer released");
        rw_spinlock_write_unlock(&lock);
        RW_TEST(lock.counter == 0, "lock free after exclusivity test");
    }

    // ----------------------------------------------------------------------
    // writer-priority invariant (multi-threaded): new readers back off while a
    // writer is pending, preventing writer starvation.
    errors += rw_spinlock_writer_priority_test();

    // concurrent writers serialize with no lost updates.
    errors += rw_spinlock_writer_mutex_test();

    // a blocked writer makes progress under continuous reader churn.
    errors += rw_spinlock_writer_liveness_test();

    if (errors)
        fprintf(stderr, "rw-spinlock unittest: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "rw-spinlock unittest: OK\n");

    return errors;
}
