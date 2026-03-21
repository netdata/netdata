// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

// ============================================================================
// Epoch-based RCU implementation
//
// Global epoch: monotonically increasing uint64_t.
//   - Readers snapshot it into their per-thread state on rcu_read_lock().
//   - Writers bump it, then wait for all readers that hold an old epoch.
//
// Per-thread epoch:
//   - 0 means "not in a read-side critical section"
//   - Non-zero means "in a read-side CS, entered at this epoch"
//   - Written ONLY by the owning thread (no atomics needed for the write).
//   - Read by writers via atomic load during rcu_synchronize().
//
// This design has the following cache-line properties:
//   - rcu_global_epoch lives on its own line; readers only LOAD it (shared
//     state, no bouncing).
//   - Each RCU_THREAD.epoch lives on the owning thread's own cache line;
//     the owner STOREs, the synchronizer LOADs (no bouncing between readers).
//   - The only shared-write operation is the global epoch increment in
//     rcu_synchronize(), which is the slow path.
// ============================================================================

#define RCU_MAX_USEC 512

// Global epoch — readers snapshot, writers bump.
// Starts at 1 so that a per-thread epoch of 0 unambiguously means "idle".
static uint64_t rcu_global_epoch __attribute__((aligned(64))) = 1;

// Global count of threads currently in a read-side critical section.
// Used by rcu_synchronize() for fast-path: if 0 (or only self), return immediately.
// This IS a shared-write atomic (incremented/decremented on every rcu_read_lock/unlock),
// but it's only read by writers in rcu_synchronize() — readers don't read it.
// The cost is one atomic add/sub per outermost rcu_read_lock/unlock, which is
// acceptable since the fast-path avoids the much more expensive thread-list scan.
static int32_t rcu_active_readers __attribute__((aligned(64))) = 0;

// Serializes rcu_synchronize() calls and protects the thread registry.
static SPINLOCK rcu_registry_spinlock = SPINLOCK_INITIALIZER;

// Linked list of all registered threads.
static RCU_THREAD *rcu_threads_head = NULL;

// Thread-local pointer to this thread's RCU state.
static __thread RCU_THREAD *rcu_tls = NULL;

// ============================================================================
// Initialization
// ============================================================================

void rcu_init(void) {
    __atomic_store_n(&rcu_global_epoch, 1, __ATOMIC_RELEASE);
    __atomic_store_n(&rcu_active_readers, 0, __ATOMIC_RELEASE);
    rcu_threads_head = NULL;
}

static __attribute__((constructor)) void rcu_constructor_init(void) {
    rcu_init();
}

void rcu_destroy(void) {
    // All threads must have unregistered before this is called.
    // If any remain, it's a bug — but we don't fatal() here to allow
    // clean shutdown even if some threads leaked.
    rcu_threads_head = NULL;
}

// ============================================================================
// Thread registration
// ============================================================================

void rcu_register_thread(void) {
    if(rcu_tls)
        return; // already registered

    RCU_THREAD *t = callocz(1, sizeof(RCU_THREAD));
    // epoch = 0 (idle), nesting = 0

    spinlock_lock(&rcu_registry_spinlock);

    t->next = rcu_threads_head;
    t->prev = NULL;
    if(rcu_threads_head)
        rcu_threads_head->prev = t;
    rcu_threads_head = t;

    spinlock_unlock(&rcu_registry_spinlock);

    rcu_tls = t;
}

void rcu_unregister_thread(void) {
    RCU_THREAD *t = rcu_tls;
    if(!t)
        return;

    // Ensure we're not inside a read-side critical section.
    if(t->nesting > 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "RCU: unregistering thread while inside read-side critical section (nesting=%u)",
               t->nesting);
        // Force-clear so rcu_synchronize() won't hang.
        __atomic_store_n(&t->epoch, 0, __ATOMIC_RELEASE);
        __atomic_sub_fetch(&rcu_active_readers, 1, __ATOMIC_RELEASE);
        t->nesting = 0;
    }

    spinlock_lock(&rcu_registry_spinlock);

    if(t->prev)
        t->prev->next = t->next;
    else
        rcu_threads_head = t->next;

    if(t->next)
        t->next->prev = t->prev;

    spinlock_unlock(&rcu_registry_spinlock);

    rcu_tls = NULL;
    freez(t);
}

bool rcu_thread_is_registered(void) {
    return rcu_tls != NULL;
}

bool rcu_thread_in_read_cs(void) {
    RCU_THREAD *t = rcu_tls;
    return t && t->nesting > 0;
}

// ============================================================================
// Read-side critical section
// ============================================================================

ALWAYS_INLINE void rcu_read_lock_with_trace(const char *func __maybe_unused) {
    RCU_THREAD *t = rcu_tls;

    // Gracefully handle unregistered threads: auto-register.
    // This is a slow path — only the first call per thread pays this cost.
    if(unlikely(!t)) {
        rcu_register_thread();
        t = rcu_tls;
    }

    if(t->nesting++ == 0) {
        // Outermost lock: snapshot the global epoch.
        uint64_t e = __atomic_load_n(&rcu_global_epoch, __ATOMIC_ACQUIRE);

        // Store our epoch so writers can see we're active.
        __atomic_store_n(&t->epoch, e, __ATOMIC_RELEASE);

        // Increment global active reader count for rcu_synchronize() fast-path.
        __atomic_add_fetch(&rcu_active_readers, 1, __ATOMIC_RELEASE);
    }
}

ALWAYS_INLINE void rcu_read_unlock_with_trace(const char *func __maybe_unused) {
    RCU_THREAD *t = rcu_tls;

    if(unlikely(!t)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "RCU: rcu_read_unlock() called on unregistered thread in %s", func);
        return;
    }

    if(unlikely(t->nesting == 0)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "RCU: rcu_read_unlock() called without matching rcu_read_lock() in %s", func);
        return;
    }

    if(--t->nesting == 0) {
        // Outermost unlock: clear our epoch so writers know we're done.
        __atomic_store_n(&t->epoch, 0, __ATOMIC_RELEASE);

        // Decrement global active reader count.
        __atomic_sub_fetch(&rcu_active_readers, 1, __ATOMIC_RELEASE);
    }
}

// ============================================================================
// Writer-side: synchronize (wait for grace period)
// ============================================================================

#define RCU_SYNCHRONIZE_TIMEOUT_SEC 60

bool rcu_synchronize(void) {
    // Fast path: if no thread (other than ourselves) is in an RCU read-side
    // CS, there's nothing to wait for. Check the global active reader count.
    RCU_THREAD *self = rcu_tls;
    int32_t self_contribution = (self && self->nesting > 0) ? 1 : 0;
    int32_t active = __atomic_load_n(&rcu_active_readers, __ATOMIC_ACQUIRE);
    if(active <= self_contribution)
        return true; // no other readers active — grace period trivially complete

    // Bump the global epoch. After this, any new rcu_read_lock() will
    // snapshot the new epoch, so we only need to wait for threads that
    // hold the old epoch.
    uint64_t old_epoch = __atomic_fetch_add(&rcu_global_epoch, 1, __ATOMIC_ACQ_REL);

    usec_t usec = 1;
    usec_t started = now_monotonic_usec();

    spinlock_lock(&rcu_registry_spinlock);

    bool all_clear;
    do {
        all_clear = true;

        for(RCU_THREAD *t = rcu_threads_head; t; t = t->next) {
            if(t == self)
                continue; // skip the calling thread

            uint64_t thread_epoch = __atomic_load_n(&t->epoch, __ATOMIC_ACQUIRE);
            if(thread_epoch != 0 && thread_epoch <= old_epoch) {
                all_clear = false;
                break;
            }
        }

        if(!all_clear) {
            // Check for timeout — a stalled reader should not block writers forever
            usec_t elapsed = now_monotonic_usec() - started;
            if(elapsed >= RCU_SYNCHRONIZE_TIMEOUT_SEC * USEC_PER_SEC) {
                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "RCU: rcu_synchronize() timed out after %d seconds waiting for readers "
                       "(epoch %"PRIu64"). Returning false — caller will defer or retry.",
                       RCU_SYNCHRONIZE_TIMEOUT_SEC, old_epoch);
                spinlock_unlock(&rcu_registry_spinlock);
                return false; // caller MUST NOT free memory
            }

            // Release the spinlock while we sleep to allow threads to
            // unregister (e.g. if they're exiting).
            spinlock_unlock(&rcu_registry_spinlock);

            microsleep(usec);
            usec = usec >= RCU_MAX_USEC ? RCU_MAX_USEC : usec * 2;

            spinlock_lock(&rcu_registry_spinlock);
        }
    } while(!all_clear);

    spinlock_unlock(&rcu_registry_spinlock);
    return true;
}
