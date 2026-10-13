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

int rw_spinlock_unittest(void) {
    int errors = 0;

    fprintf(stderr, "\nrunning rw-spinlock unittest\n");

    // ----------------------------------------------------------------------
    // initialization: a freshly initialized lock has no writer and no readers,
    // and a static initializer produces the same state.
    {
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

    if (errors)
        fprintf(stderr, "rw-spinlock unittest: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "rw-spinlock unittest: OK\n");

    return errors;
}
