// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RCU_H
#define NETDATA_RCU_H

#include "libnetdata/common.h"
#include "spinlock.h"

// ============================================================================
// Epoch-based Read-Copy-Update (RCU)
//
// Provides extremely fast read-side critical sections
// at the cost of slower write-side synchronization.
//
// Use this when:
//   - Reads vastly outnumber writes
//   - Multiple reader threads contend on the same data
//   - Current RW_SPINLOCK reader cache-line bouncing is a bottleneck
//
// Read-side cost:  1 atomic load (global epoch, shared-read)
//                + 1 store to per-thread epoch
//                + 1 shared atomic add/sub on the outermost lock/unlock pair
//                  for the active-reader fast path in rcu_synchronize()
//
// Write-side cost: 1 atomic increment (global epoch)
//                + scan all registered threads (may spin-wait on slow readers)
//
// Nesting: read-side critical sections may nest; only the outermost pair
//          has any cost.
//
// ============================================================================

// Per-thread RCU state.
// Allocated per thread; callers should not assume cache-line alignment.
// The epoch field is written ONLY by the owning thread (via atomic release
// stores) and read by writers during rcu_synchronize() (via atomic acquire
// loads). The nesting field is private to the owning thread.
typedef struct rcu_thread {
    // --- written by owning thread, read by writers ---
    uint64_t epoch;                     // snapshot of global epoch while in read-side CS; 0 = idle
    uint32_t nesting;                   // nesting depth (private, never read by others)

    // --- registry linkage (protected by rcu_registry_spinlock) ---
    struct rcu_thread *prev;
    struct rcu_thread *next;
} RCU_THREAD;

// ============================================================================
// API
// ============================================================================

// Initialize the RCU subsystem. This is also invoked by a constructor.
void rcu_init(void);

// Destroy the RCU subsystem — call once at shutdown.
void rcu_destroy(void);

// Per-thread lifecycle — every thread that will enter rcu_read_lock() must
// first call rcu_register_thread(), and must call rcu_unregister_thread()
// before it exits. Failing to unregister will make rcu_synchronize() wait
// forever for that thread.
void rcu_register_thread(void);
void rcu_unregister_thread(void);

// Returns true if the calling thread is registered with RCU.
bool rcu_thread_is_registered(void);

// Returns true if the calling thread is inside an RCU read-side critical section.
// Writers should check this before calling rcu_synchronize() to avoid livelock
// when deleting items from dictionary B while traversing dictionary A.
bool rcu_thread_in_read_cs(void);

// Read-side critical section.
// Between rcu_read_lock() and rcu_read_unlock(), the thread is guaranteed
// that no writer will complete rcu_synchronize() — i.e. old data will not
// be freed while the reader holds the lock.
//
// These are meant to be very fast, with only one shared atomic add/sub on
// the outermost lock/unlock pair.
void rcu_read_lock_with_trace(const char *func);
void rcu_read_unlock_with_trace(const char *func);

#define rcu_read_lock() rcu_read_lock_with_trace(__FUNCTION__)
#define rcu_read_unlock() rcu_read_unlock_with_trace(__FUNCTION__)

// Writer-side: wait until all threads that were in a read-side critical
// section at the time of this call have left it.
//
// Returns true if the grace period completed successfully — the caller
// can safely free old data. Returns false if the wait timed out — the
// caller MUST NOT free the data (readers may still hold pointers).
//
// Multiple concurrent rcu_synchronize() calls are serialized internally.
bool rcu_synchronize(void);

// ----------------------------------------------------------------------------
// Pointer publication helpers
//
// rcu_dereference(p)       — load a pointer that may be updated by writers
// rcu_assign_pointer(p, v) — publish a new pointer so readers see it
// ----------------------------------------------------------------------------

#define rcu_dereference(p) \
    __atomic_load_n(&(p), __ATOMIC_ACQUIRE)

#define rcu_assign_pointer(p, v) \
    __atomic_store_n(&(p), (v), __ATOMIC_RELEASE)

// ----------------------------------------------------------------------------
// Pointer-swap RCU pattern for non-DICTIONARY structures
//
// Use this when a data structure is rarely modified but frequently read.
// The writer creates a new version, atomically publishes it, waits for
// readers to finish with the old version, then frees the old version.
//
// Example usage:
//
//   // Reader (called frequently):
//   rcu_read_lock();
//   struct config *cfg = rcu_dereference(global_config);
//   use(cfg->field);
//   rcu_read_unlock();
//
//   // Writer (called rarely):
//   struct config *new_cfg = mallocz(sizeof(*new_cfg));
//   *new_cfg = *global_config;   // copy
//   new_cfg->field = new_value;  // modify
//   struct config *old = global_config;
//   rcu_assign_pointer(global_config, new_cfg);
//   rcu_synchronize();           // wait for readers of old
//   freez(old);                  // safe to free now
//
// For singly-linked list element removal:
//
//   // Writer removes 'item' from list:
//   rcu_assign_pointer(prev->next, item->next);  // unlink
//   rcu_synchronize();                            // wait for readers
//   freez(item);                                  // safe
//
// ----------------------------------------------------------------------------

#endif // NETDATA_RCU_H
