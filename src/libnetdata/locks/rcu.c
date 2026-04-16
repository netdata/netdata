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
//   - Written only by the owning thread, using atomic release stores to
//     publish entry/exit from the read-side critical section.
//   - Read by writers via atomic acquire loads during rcu_synchronize().
//
// This design has the following cache-line properties:
//   - rcu_global_epoch lives on its own line; readers only LOAD it (shared
//     state, no bouncing).
//   - Each RCU_THREAD.epoch is per-thread state; readers do not contend on a
//     shared counter, though allocator placement may still allow false sharing.
//   - The only shared-write operation is the global epoch increment in
//     rcu_synchronize(), which is the slow path.
// ============================================================================

#define RCU_MAX_USEC 512

// Global epoch — readers snapshot, writers bump.
// Starts at 1 so that a per-thread epoch of 0 unambiguously means "idle".
static uint64_t rcu_global_epoch __attribute__((aligned(64))) = 1;

// Protects the thread registry.
static SPINLOCK rcu_registry_spinlock = SPINLOCK_INITIALIZER;

// Serializes rcu_synchronize() calls while allowing register/unregister to
// proceed between snapshot polls.
static SPINLOCK rcu_synchronize_spinlock = SPINLOCK_INITIALIZER;

// Linked list of all registered threads.
static RCU_THREAD *rcu_threads_head = NULL;

// Thread-local pointer to this thread's RCU state.
static __thread RCU_THREAD *rcu_tls = NULL;

// ============================================================================
// Initialization
// ============================================================================

void rcu_init(void) {
    __atomic_store_n(&rcu_global_epoch, 1, __ATOMIC_RELEASE);
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

static void rcu_thread_release(RCU_THREAD *t) {
    if(!t)
        return;

    if(__atomic_sub_fetch(&t->refs, 1, __ATOMIC_ACQ_REL) == 0)
        freez(t);
}

static RCU_THREAD **rcu_snapshot_threads(RCU_THREAD *self, size_t *threads_count) {
    *threads_count = 0;

    spinlock_lock(&rcu_registry_spinlock);

    size_t count = 0;
    for(RCU_THREAD *t = rcu_threads_head; t; t = t->next) {
        if(t != self)
            count++;
    }

    if(!count) {
        spinlock_unlock(&rcu_registry_spinlock);
        return NULL;
    }

    RCU_THREAD **threads = callocz(count, sizeof(*threads));
    size_t idx = 0;
    for(RCU_THREAD *t = rcu_threads_head; t; t = t->next) {
        if(t == self)
            continue;

        __atomic_add_fetch(&t->refs, 1, __ATOMIC_ACQ_REL);
        threads[idx++] = t;
    }

    spinlock_unlock(&rcu_registry_spinlock);

    *threads_count = idx;
    return threads;
}

static void rcu_release_snapshot(RCU_THREAD **threads, size_t threads_count) {
    if(!threads)
        return;

    for(size_t i = 0; i < threads_count; i++)
        rcu_thread_release(threads[i]);

    freez(threads);
}

void rcu_register_thread(void) {
    if(rcu_tls)
        return; // already registered

    RCU_THREAD *t = callocz(1, sizeof(RCU_THREAD));
    // epoch = 0 (idle), nesting = 0
    __atomic_store_n(&t->refs, 1, __ATOMIC_RELAXED);

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
    rcu_thread_release(t);
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

        // Prevent protected reads in the caller's critical section from
        // moving before the published epoch.
        __atomic_thread_fence(__ATOMIC_ACQUIRE);
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
    }
}

// ============================================================================
// Writer-side: synchronize (wait for grace period)
// ============================================================================

#define RCU_SYNCHRONIZE_TIMEOUT_SEC 60

bool rcu_synchronize(void) {
    RCU_THREAD *self = rcu_tls;

    if(unlikely(self && self->nesting > 0)) {
        fatal_assert(self->nesting > 0);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "RCU: rcu_synchronize() called while inside a read-side critical section "
               "(nesting=%u)", self->nesting);
        return false;
    }

    spinlock_lock(&rcu_synchronize_spinlock);

    // Bump the global epoch. After this, any new rcu_read_lock() will
    // snapshot the new epoch, so we only need to wait for threads that
    // hold the old epoch.
    uint64_t old_epoch = __atomic_fetch_add(&rcu_global_epoch, 1, __ATOMIC_ACQ_REL);

    size_t threads_count = 0;
    RCU_THREAD **threads = rcu_snapshot_threads(self, &threads_count);
    if(!threads_count) {
        spinlock_unlock(&rcu_synchronize_spinlock);
        return true;
    }

    usec_t usec = 1;
    usec_t started = now_monotonic_usec();

    bool all_clear;
    do {
        all_clear = true;

        for(size_t i = 0; i < threads_count; i++) {
            uint64_t thread_epoch = __atomic_load_n(&threads[i]->epoch, __ATOMIC_ACQUIRE);
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
                rcu_release_snapshot(threads, threads_count);
                spinlock_unlock(&rcu_synchronize_spinlock);
                return false; // caller MUST NOT free memory
            }

            microsleep(usec);
            usec = usec >= RCU_MAX_USEC ? RCU_MAX_USEC : usec * 2;
        }
    } while(!all_clear);

    rcu_release_snapshot(threads, threads_count);
    spinlock_unlock(&rcu_synchronize_spinlock);
    return true;
}
