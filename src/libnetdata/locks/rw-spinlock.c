// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#define MAX_USEC 512 // Maximum backoff limit in microseconds

#define WRITER_BIT (1U << 31)
#define READER_MASK (~WRITER_BIT)

// Embedded in every DICTIONARY (twice) and every RRDSET: it must not grow.
_Static_assert(sizeof(RW_SPINLOCK) == 8, "RW_SPINLOCK must stay 8 bytes");

// ----------------------------------------------------------------------------
// Holder-aware deadlock detection for rw-spinlocks.
//
// The plain spinlock_deadlock_detect() can only name the WAITER, because it never sees the lock.
// That is why the fleet's 3600s rw-spinlock aborts are un-triagable: they identify the victim and
// say nothing about who held the lock.
//
// RW_SPINLOCK already stores the write holder's tid, so naming it costs nothing. Readers, however,
// are anonymous (the lock keeps only an aggregate count), and growing RW_SPINLOCK is not an option:
// it is embedded in every DICTIONARY (twice) and every RRDSET. So everything else we need is
// accumulated on the WAITER'S OWN STACK, exactly like the pre-existing deadlock_timestamp local:
//
//   - the reader count, with its min/max over the deadlock checks;
//   - the writer tid at the first check, to compare with the one at timeout.
//
// These are samples taken once per deadlock check (about a minute apart once backed off), so the
// message reports them and leaves the conclusion to the reader:
//   - the reader count includes readers caught mid-attempt (they increment, see a writer, undo);
//   - writer is 0 while a waiting writer has only flicked WRITER_BIT on, and briefly during unlock;
//   - the same tid at both checks does not prove one uninterrupted hold.
//
// Zero bytes added to the lock, no change to the uncontended fast path.
typedef struct rw_spinlock_wait {
    usec_t timestamp;       // when we first noticed we were spinning (0 = not seeded yet)
    pid_t first_writer;     // rw_spinlock->writer as sampled at that moment
    uint32_t readers_min;
    uint32_t readers_max;
} RW_SPINLOCK_WAIT;

#define RW_SPINLOCK_WAIT_INIT (RW_SPINLOCK_WAIT){ 0 }

void rw_spinlock_deadlock_holder_format(char *dst, size_t dst_size, pid_t self, pid_t writer, pid_t first_writer,
                                        uint32_t readers, uint32_t readers_min, uint32_t readers_max) {
    char now_txt[40], first_txt[48];

    if (writer)
        snprintfz(now_txt, sizeof(now_txt), "writer tid=%d%s", writer, writer == self ? " (this thread)" : "");
    else
        snprintfz(now_txt, sizeof(now_txt), "no writer");

    if (writer == first_writer)
        snprintfz(first_txt, sizeof(first_txt), "same at the first check");
    else if (first_writer)
        snprintfz(first_txt, sizeof(first_txt), "was tid=%d at the first check", first_writer);
    else
        snprintfz(first_txt, sizeof(first_txt), "no writer at the first check");

    snprintfz(dst, dst_size, "[holder: %s, %s, reader count=%u (min=%u max=%u over the checks)]",
              now_txt, first_txt, readers, readers_min, readers_max);
}

// Out of line: the inlined lock functions keep only the call, not the message buffers.
static NEVER_INLINE void rw_spinlock_deadlock_detect(RW_SPINLOCK_WAIT *w, const char *type, const char *func,
                                                     RW_SPINLOCK *rw_spinlock, uint32_t readers_now) {
    // Can be stale by the time we print it; it is only ever printed, never dereferenced.
    pid_t writer_now = __atomic_load_n(&rw_spinlock->writer, __ATOMIC_RELAXED);

    if (!w->timestamp) {
        w->timestamp = now_monotonic_usec();
        w->first_writer = writer_now;
        w->readers_min = readers_now;
        w->readers_max = readers_now;
        return;
    }

    if (readers_now < w->readers_min) w->readers_min = readers_now;
    if (readers_now > w->readers_max) w->readers_max = readers_now;

    usec_t now = now_monotonic_usec();
    if (now - w->timestamp < SPINLOCK_DEADLOCK_TIMEOUT_SEC * USEC_PER_SEC)
        return;

    int64_t waited_s = (int64_t)((now - w->timestamp) / USEC_PER_SEC);

    char holder[256];
    rw_spinlock_deadlock_holder_format(holder, sizeof(holder), gettid_cached(), writer_now, w->first_writer,
                                       readers_now, w->readers_min, w->readers_max);

    fatal("DEADLOCK DETECTED: %s in function '%s' could not be acquired for %"PRIi64" seconds %s",
          type, func, waited_s, holder);
}

// ----------------------------------------------------------------------------
// rw_spinlock implementation

void rw_spinlock_init_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
    rw_spinlock->writer = 0;
    rw_spinlock->counter = 0;
}

ALWAYS_INLINE bool rw_spinlock_tryread_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;

    uint32_t val = __atomic_add_fetch(&rw_spinlock->counter, 1, __ATOMIC_ACQUIRE);

    // Check if a writer holds the lock
    if (val & WRITER_BIT) {
        // Undo our increment and fail
        __atomic_sub_fetch(&rw_spinlock->counter, 1, __ATOMIC_RELEASE);
        return false;
    }

    worker_spinlock_contention(func, spins);
    nd_thread_rwspinlock_read_locked();
    return true;
}

ALWAYS_INLINE void rw_spinlock_read_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;
    usec_t usec = 1;
    RW_SPINLOCK_WAIT wait = RW_SPINLOCK_WAIT_INIT;

    while (true) {
        // Optimistically increment reader count
        uint32_t val = __atomic_add_fetch(&rw_spinlock->counter, 1, __ATOMIC_ACQUIRE);

        // Check if a writer holds the lock
        if (!(val & WRITER_BIT)) {
            // no writer, we are in
            worker_spinlock_contention(func, spins);
            nd_thread_rwspinlock_read_locked();
            return;
        }

        // Undo our increment and retry
        __atomic_sub_fetch(&rw_spinlock->counter, 1, __ATOMIC_RELEASE);

        spins++;
        
        // Check for deadlock every SPINS_BEFORE_DEADLOCK_CHECK iterations
        if ((spins % SPINS_BEFORE_DEADLOCK_CHECK) == 0) {
            // val still carries the reader count we observed before undoing our own increment;
            // subtract ourselves so the number describes the other threads counted in the lock.
            uint32_t readers = (val & READER_MASK);
            rw_spinlock_deadlock_detect(&wait, "rw-spinlock read lock", func, rw_spinlock,
                                        readers ? readers - 1 : 0);
        }

        microsleep(usec);
        usec = usec >= MAX_USEC ? MAX_USEC : usec * 2;
    }
}

ALWAYS_INLINE void rw_spinlock_read_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
    __atomic_sub_fetch(&rw_spinlock->counter, 1, __ATOMIC_RELEASE);
    nd_thread_rwspinlock_read_unlocked();
}

ALWAYS_INLINE bool rw_spinlock_trywrite_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    // Optimistically set writer bit
    uint32_t old = __atomic_fetch_or(&rw_spinlock->counter, WRITER_BIT, __ATOMIC_ACQUIRE);

    if(old == 0) {
        __atomic_store_n(&rw_spinlock->writer, gettid_cached(), __ATOMIC_RELAXED);
        worker_spinlock_contention(func, 0);
        nd_thread_rwspinlock_write_locked();
        return true;
    }

    // Check if we were the only one
    if (old & WRITER_BIT) {
        // there is a writer inside (keep the writer bit there)
    }
    else /* if ((old & READER_MASK) != 0) */ {
        // there are readers inside, remove the writer bit we added
        __atomic_and_fetch(&rw_spinlock->counter, ~WRITER_BIT, __ATOMIC_RELEASE);
    }

    return false;
}

ALWAYS_INLINE void rw_spinlock_write_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;
    usec_t usec = 1;
    RW_SPINLOCK_WAIT wait = RW_SPINLOCK_WAIT_INIT;
    uint32_t readers_seen = 0;

    while (1) {
        // Optimistically set writer bit
        uint32_t old = __atomic_fetch_or(&rw_spinlock->counter, WRITER_BIT, __ATOMIC_ACQUIRE);

        // Check if we were the only one
        if (old == 0) {
            __atomic_store_n(&rw_spinlock->writer, gettid_cached(), __ATOMIC_RELAXED);
            worker_spinlock_contention(func, spins);
            nd_thread_rwspinlock_write_locked();
            return;
        }

        // Check if we were the only one
        if (old & WRITER_BIT) {
            // there is a writer inside (keep the writer bit there)
        }
        else /* if ((old & READER_MASK) != 0) */ {
            // there are readers inside, remove the writer bit we added
            __atomic_and_fetch(&rw_spinlock->counter, ~WRITER_BIT, __ATOMIC_RELEASE);
        }

        readers_seen = (old & READER_MASK);
        spins++;

        // Check for deadlock every SPINS_BEFORE_DEADLOCK_CHECK iterations
        if ((spins % SPINS_BEFORE_DEADLOCK_CHECK) == 0) {
            rw_spinlock_deadlock_detect(&wait, "rw-spinlock write lock", func, rw_spinlock, readers_seen);
        }
        
        microsleep(usec);
        usec = usec >= MAX_USEC ? MAX_USEC : usec * 2;
    }
}

ALWAYS_INLINE void rw_spinlock_write_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
    __atomic_store_n(&rw_spinlock->writer, 0, __ATOMIC_RELAXED);
    __atomic_and_fetch(&rw_spinlock->counter, ~WRITER_BIT, __ATOMIC_RELEASE);
    nd_thread_rwspinlock_write_unlocked();
}
