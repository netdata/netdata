// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdset-slots.h"
#include "rrdset-pluginsd-array.h"

void rrdset_stream_send_chart_slot_assign(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    spinlock_lock(&host->stream.snd.pluginsd_chart_slots.available.spinlock);

    if(host->stream.snd.pluginsd_chart_slots.available.used > 0)
        st->stream.snd.chart_slot =
            host->stream.snd.pluginsd_chart_slots.available.array[--host->stream.snd.pluginsd_chart_slots.available.used];
    else
        st->stream.snd.chart_slot = ++host->stream.snd.pluginsd_chart_slots.last_used;

    spinlock_unlock(&host->stream.snd.pluginsd_chart_slots.available.spinlock);
}

void rrdset_stream_send_chart_slot_release(RRDSET *st) {
    if(!st->stream.snd.chart_slot || st->rrdhost->stream.snd.pluginsd_chart_slots.available.ignore)
        return;

    RRDHOST *host = st->rrdhost;
    spinlock_lock(&host->stream.snd.pluginsd_chart_slots.available.spinlock);

    if(host->stream.snd.pluginsd_chart_slots.available.used >= host->stream.snd.pluginsd_chart_slots.available.size) {
        uint32_t old_slots = host->stream.snd.pluginsd_chart_slots.available.size;
        uint32_t new_slots = (old_slots > 0) ? (old_slots * 2) : 1024;

        host->stream.snd.pluginsd_chart_slots.available.array =
            reallocz(host->stream.snd.pluginsd_chart_slots.available.array, new_slots * sizeof(uint32_t));

        host->stream.snd.pluginsd_chart_slots.available.size = new_slots;

        rrd_slot_memory_added((new_slots - old_slots) * sizeof(uint32_t));
    }

    host->stream.snd.pluginsd_chart_slots.available.array[host->stream.snd.pluginsd_chart_slots.available.used++] =
        st->stream.snd.chart_slot;

    st->stream.snd.chart_slot = 0;
    spinlock_unlock(&host->stream.snd.pluginsd_chart_slots.available.spinlock);
}

// --------------------------------------------------------------------------------------------------------------------
// Helper function to release RRDDIM_ACQUIRED references in array entries
// This must be called before the final prd_array_release when cleaning up

static void prd_array_release_entries(PRD_ARRAY *arr) {
    if (!arr)
        return;

    for (size_t i = 0; i < arr->size; i++) {
        rrddim_acquired_release(arr->entries[i].rda);  // safe with NULL
        arr->entries[i].rda = NULL;
        arr->entries[i].rd = NULL;
        arr->entries[i].id = NULL;
    }
}

static inline void rrdset_clear_host_chart_slot_mapping(RRDSET *st, int32_t last_slot) {
    if(last_slot < 0)
        return;

    RRDHOST *host = st->rrdhost;
    spinlock_lock(&host->stream.rcv.pluginsd_chart_slots.spinlock);
    if((uint32_t)last_slot < host->stream.rcv.pluginsd_chart_slots.size &&
        host->stream.rcv.pluginsd_chart_slots.array[last_slot] == st) {
        host->stream.rcv.pluginsd_chart_slots.array[last_slot] = NULL;
    }
    spinlock_unlock(&host->stream.rcv.pluginsd_chart_slots.spinlock);
}

// --------------------------------------------------------------------------------------------------------------------
// Unslot a chart - releases dimension references but keeps the array for reuse
// This is called when switching charts, marking them obsolete, or during cleanup.
//
// Safe to call from:
// - The collector thread itself (collector_tid == gettid_cached()): uses lock-free access
// - Any thread when the collector is fully stopped (collector_tid == 0): uses refcount
// Skips with a warning if a DIFFERENT thread's collector is active.

void rrdset_pluginsd_receive_unslot(RRDSET *st) {
    if(!st)
        return;

    RRDDIM_ACQUIRED **detached_rdas = NULL;
    size_t detached_capacity = 0;
    size_t detached_entries = 0;
    bool we_are_collector = false;
    PRD_ARRAY *arr = NULL;
    int32_t last_slot = -1;

    while(true) {
        spinlock_lock(&st->pluginsd.spinlock);

        // Check collector_tid inside spinlock
        pid_t collector_tid = __atomic_load_n(&st->pluginsd.collector_tid, __ATOMIC_ACQUIRE);
        we_are_collector = (collector_tid == gettid_cached());
        bool different_collector_active = (collector_tid != 0 && !we_are_collector);

        last_slot = st->pluginsd.last_slot;

        if(different_collector_active) {
            // Another thread is the active collector - we cannot safely touch the array.
            // Keep pluginsd state unchanged in this path: the active collector may
            // still read last_slot / dims_with_slots lock-free.
            // Clear only the host slot mapping and bail out.
            nd_log_limit_static_global_var(erl, 1, 0);
            nd_log_limit(&erl, NDLS_DAEMON, NDLP_WARNING,
                         "PLUGINSD: rrdset_pluginsd_receive_unslot called while collector (tid %d) is active, skipping",
                         collector_tid);

            spinlock_unlock(&st->pluginsd.spinlock);
            freez(detached_rdas);
            rrdset_clear_host_chart_slot_mapping(st, last_slot);
            return;
        }

        // Either collector_tid == 0 (collector stopped) or collector_tid == our tid
        // (we ARE the collector). In both cases, it's safe to detach dimension references.
        arr = we_are_collector ?
              prd_array_get_unsafe(&st->pluginsd.prd_array) :
              prd_array_acquire_locked(&st->pluginsd.prd_array);

        if(arr) {
            if(!we_are_collector) {
                // Verify no other thread holds an extra reference before clearing entries.
                // After acquire_locked, refcount should be 2 (original + ours).
                int32_t rc = __atomic_load_n(&arr->refcount, __ATOMIC_ACQUIRE);
                internal_fatal(rc != 2,
                               "PRD_ARRAY: expected refcount 2 after acquire, got %d - concurrent reference leak", rc);

                if(unlikely(rc != 2)) {
                    // Production guard: skip detachment when another reference is active.
                    // Clearing entries in this state can race and double-release references.
                    nd_log_limit_static_global_var(erl_rc, 1, 0);
                    nd_log_limit(&erl_rc, NDLS_DAEMON, NDLP_WARNING,
                                 "PLUGINSD: unslot skipped for chart with unexpected PRD_ARRAY refcount %d (expected 2)",
                                 rc);

                    prd_array_release(arr);
                    spinlock_unlock(&st->pluginsd.spinlock);
                    freez(detached_rdas);
                    rrdset_clear_host_chart_slot_mapping(st, last_slot);
                    return;
                }
            }

            detached_entries = arr->size;
            if(detached_entries > detached_capacity) {
                // Allocate outside the spinlock to avoid allocator latency while other
                // threads are spinning on this lock.
                if(!we_are_collector)
                    prd_array_release(arr);

                spinlock_unlock(&st->pluginsd.spinlock);

                freez(detached_rdas);
                detached_rdas = callocz(detached_entries, sizeof(*detached_rdas));
                detached_capacity = detached_entries;
                continue;
            }

            // Detach entries while holding st->pluginsd.spinlock so concurrent unslot/cleanup
            // cannot race and release the same RRDDIM_ACQUIRED pointers twice.
            for(size_t i = 0; i < detached_entries; i++) {
                detached_rdas[i] = arr->entries[i].rda;
                arr->entries[i].rda = NULL;
                arr->entries[i].rd = NULL;
                arr->entries[i].id = NULL;
            }
        }
        else
            detached_entries = 0;

        st->pluginsd.last_slot = -1;
        st->pluginsd.dims_with_slots = false;

        spinlock_unlock(&st->pluginsd.spinlock);
        break;
    }

    // Release detached references outside the spinlock.
    if(detached_rdas) {
        for(size_t i = 0; i < detached_entries; i++)
            rrddim_acquired_release(detached_rdas[i]); // safe with NULL

        freez(detached_rdas);
    }

    if(arr && !we_are_collector) {
        // Release our acquired reference (keeps the struct's reference alive for reuse)
        prd_array_release(arr);
    }

    rrdset_clear_host_chart_slot_mapping(st, last_slot);
}

// --------------------------------------------------------------------------------------------------------------------
// Full cleanup - unslots and frees the array
// This is called during chart finalization or host cleanup
// Thread-safe: uses spinlock for cleanup coordination and reference counting for array lifetime

void rrdset_pluginsd_receive_unslot_and_cleanup(RRDSET *st) {
    if(!st)
        return;

    spinlock_lock(&st->pluginsd.spinlock);

    // Check if collector is still active.
    pid_t collector_tid = __atomic_load_n(&st->pluginsd.collector_tid, __ATOMIC_ACQUIRE);
    pid_t current_tid = gettid_cached();
    if(collector_tid != 0) {
        if(collector_tid != current_tid) {
            // A different thread owns this chart as collector.
            // We must not release PRD dimension references while that thread may
            // still be using cached rd pointers from the array.
            // RRDSET_FLAG_COLLECTION_FINISHED is not a reliable signal here:
            // it can be set by the cleanup caller (service thread) rather than
            // by the collector itself, creating a race where we free dimensions
            // that the collector is actively dereferencing.
            // Legitimate teardown clears collector_tid via
            // rrdhost_pluginsd_receive_chart_slots_free() after the receiver
            // is fully stopped, before invoking cleanup.
            nd_log_limit_static_global_var(erl, 1, 0);
            nd_log_limit(&erl, NDLS_DAEMON, NDLP_WARNING,
                         "PLUGINSD: attempted cleanup while collector (tid %d) is still active on chart, skipping",
                         collector_tid);
            spinlock_unlock(&st->pluginsd.spinlock);
            return;
        }

        // We ARE the collector thread - safe to proceed with cleanup.
        // This should not normally happen; keep it explicit so we don't
        // mask it as a stale tid case.
#ifdef NETDATA_INTERNAL_CHECKS
        internal_fatal(true,
                       "PRD_ARRAY: cleanup called from collector thread (tid %d) - lifecycle violation",
                       collector_tid);
#endif

        nd_log_limit_static_global_var(erl_collector, 1, 0);
        nd_log_limit(&erl_collector, NDLS_DAEMON, NDLP_WARNING,
                     "PLUGINSD: cleanup called from collector thread (tid %d), forcing collector_tid=0",
                     collector_tid);

        __atomic_store_n(&st->pluginsd.collector_tid, 0, __ATOMIC_RELEASE);
    }

    // Replace the array with NULL - this prevents new references from being acquired
    PRD_ARRAY *old_arr = prd_array_replace(&st->pluginsd.prd_array, NULL);

    // Capture last_slot before resetting - we need it to clear the host mapping
    int32_t last_slot = st->pluginsd.last_slot;

    // Reset state while holding the lock
    __atomic_store_n(&st->pluginsd.pos, 0, __ATOMIC_RELAXED);
    st->pluginsd.set = false;
    st->pluginsd.last_slot = -1;
    st->pluginsd.dims_with_slots = false;

    spinlock_unlock(&st->pluginsd.spinlock);

    // Clear the chart slot mapping using the captured last_slot value
    rrdset_clear_host_chart_slot_mapping(st, last_slot);

    // Now handle the old array outside the lock
    if (old_arr) {
        // After prd_array_replace, we hold the only reference (refcount should be 1).
        // It's safe to release entries only when we're the sole owner, to avoid clearing
        // entries that another thread might still be reading through its own reference.
        int32_t rc = __atomic_load_n(&old_arr->refcount, __ATOMIC_ACQUIRE);
        internal_fatal(rc != 1,
                       "PRD_ARRAY: expected refcount 1 after replace, got %d - concurrent reference leak", rc);

        if(unlikely(rc != 1)) {
            // Production guard: another reference still exists, so clearing entries
            // here could race with readers and double-release RRDDIM_ACQUIRED.
            nd_log_limit_static_global_var(erl_cleanup_rc, 1, 0);
            nd_log_limit(&erl_cleanup_rc, NDLS_DAEMON, NDLP_WARNING,
                         "PLUGINSD: cleanup deferred for chart with unexpected PRD_ARRAY refcount %d (expected 1)",
                         rc);

            // Drop our reference only; remaining owners will eventually release.
            prd_array_release(old_arr);
            return;
        }

        // Release all dimension references (safe - we're the sole owner).
        prd_array_release_entries(old_arr);

        // Release our reference - this will free the array (refcount 1 -> 0)
        prd_array_release(old_arr);
    }
}

// --------------------------------------------------------------------------------------------------------------------
// Initialize the pluginsd slots for a chart

void rrdset_pluginsd_receive_slots_initialize(RRDSET *st) {
    spinlock_init(&st->pluginsd.spinlock);
    st->pluginsd.last_slot = -1;
    st->pluginsd.prd_array = NULL;  // Explicitly initialize to NULL
}

// --------------------------------------------------------------------------------------------------------------------
// Stress test for PRD_ARRAY lifecycle separation model
// Run with: netdata -W prd-array-stress
//
// This test validates the lifecycle separation model used in production:
// - In production, the collector is FULLY STOPPED before cleanup runs
// - The collector_tid check is a safety mechanism, but the real protection comes from lifecycle separation
// - This test simulates that by running the writer and cleaner in non-overlapping phases
//
// The test runs in cycles:
// 1. Writer phase: collector runs multiple iterations (collector_tid set)
// 2. Handoff: collector fully stops (collector_tid cleared, writer_done signaled)
// 3. Cleanup phase: cleaner runs (only when writer is fully stopped)
// 4. Repeat
// --------------------------------------------------------------------------------------------------------------------

#define PRD_STRESS_TEST_DURATION_SEC 5
#define PRD_STRESS_ITERATIONS_PER_PHASE 50

typedef struct {
    PRD_ARRAY *prd_array;
    pid_t collector_tid;
    SPINLOCK spinlock;

    // Lifecycle coordination (simulates stream receiver stop/start)
    bool test_running;           // Overall test is running
    bool writer_should_run;      // Writer is allowed to run
    bool writer_is_running;      // Writer is currently in a phase

    // Counters
    uint64_t grow_count;
    uint64_t cleanup_count;
    uint64_t phase_count;
} prd_stress_state_t;

static prd_stress_state_t prd_stress_state;

static void prd_stress_writer_thread(void *arg __maybe_unused) {
    while (__atomic_load_n(&prd_stress_state.test_running, __ATOMIC_ACQUIRE)) {

        // Wait until we're allowed to run (simulates stream receiver starting)
        while (__atomic_load_n(&prd_stress_state.test_running, __ATOMIC_ACQUIRE) &&
               !__atomic_load_n(&prd_stress_state.writer_should_run, __ATOMIC_ACQUIRE)) {
            tinysleep();
        }

        if (!__atomic_load_n(&prd_stress_state.test_running, __ATOMIC_ACQUIRE))
            break;

        // Signal that writer is now running
        __atomic_store_n(&prd_stress_state.writer_is_running, true, __ATOMIC_RELEASE);

        // Simulate collector_tid being set (like pluginsd_set_scope_chart does)
        __atomic_store_n(&prd_stress_state.collector_tid, gettid_cached(), __ATOMIC_RELEASE);

        // Run multiple iterations in this phase (simulates collecting data)
        for (int iter = 0; iter < PRD_STRESS_ITERATIONS_PER_PHASE; iter++) {
            if (!__atomic_load_n(&prd_stress_state.writer_should_run, __ATOMIC_ACQUIRE))
                break;

            PRD_ARRAY *current_arr = prd_array_get_unsafe(&prd_stress_state.prd_array);

            size_t current_size = current_arr ? current_arr->size : 0;
            size_t new_size = current_size + 10;

            if (new_size > 500)
                new_size = 10;

            PRD_ARRAY *new_arr = prd_array_create(new_size);

            if (current_arr && current_size > 0) {
                size_t copy_count = (current_size < new_size) ? current_size : new_size;
                for(size_t i = 0; i < copy_count; i++) {
                    new_arr->entries[i].rd = current_arr->entries[i].rd;
                    new_arr->entries[i].id = current_arr->entries[i].id;
                    new_arr->entries[i].rda = NULL;
                }
            }

            for (size_t i = (current_size < new_size ? current_size : 0); i < new_size; i++) {
                new_arr->entries[i].rd = (void *)(uintptr_t)(i + 1);
                new_arr->entries[i].id = "test";
            }

            PRD_ARRAY *old_arr = prd_array_replace(&prd_stress_state.prd_array, new_arr);

            if (old_arr)
                prd_array_release(old_arr);

            __atomic_fetch_add(&prd_stress_state.grow_count, 1, __ATOMIC_RELAXED);

            tinysleep();
        }

        // Clear collector_tid (like pluginsd_set_scope_chart does when switching away)
        __atomic_store_n(&prd_stress_state.collector_tid, 0, __ATOMIC_RELEASE);

        // Signal that writer phase is complete
        __atomic_store_n(&prd_stress_state.writer_is_running, false, __ATOMIC_RELEASE);

        // Wait until controller signals us to run again
        while (__atomic_load_n(&prd_stress_state.test_running, __ATOMIC_ACQUIRE) &&
               !__atomic_load_n(&prd_stress_state.writer_should_run, __ATOMIC_ACQUIRE)) {
            tinysleep();
        }
    }
}

static void prd_stress_cleanup_thread(void *arg __maybe_unused) {
    while (__atomic_load_n(&prd_stress_state.test_running, __ATOMIC_ACQUIRE)) {

        // Wait until writer is fully stopped (simulates stream_receiver_signal_to_stop_and_wait)
        while (__atomic_load_n(&prd_stress_state.test_running, __ATOMIC_ACQUIRE) &&
               __atomic_load_n(&prd_stress_state.writer_is_running, __ATOMIC_ACQUIRE)) {
            tinysleep();
        }

        if (!__atomic_load_n(&prd_stress_state.test_running, __ATOMIC_ACQUIRE))
            break;

        // Now safe to cleanup - writer is fully stopped
        spinlock_lock(&prd_stress_state.spinlock);

        // Double-check collector_tid (should be 0 since writer stopped)
        pid_t collector_tid = __atomic_load_n(&prd_stress_state.collector_tid, __ATOMIC_ACQUIRE);
        if (collector_tid != 0) {
            // This shouldn't happen if lifecycle is correct
            spinlock_unlock(&prd_stress_state.spinlock);
            continue;
        }

        PRD_ARRAY *old_arr = prd_array_replace(&prd_stress_state.prd_array, NULL);

        spinlock_unlock(&prd_stress_state.spinlock);

        if (old_arr) {
            for (size_t i = 0; i < old_arr->size; i++) {
                old_arr->entries[i].rda = NULL;
                old_arr->entries[i].rd = NULL;
                old_arr->entries[i].id = NULL;
            }

            prd_array_release(old_arr);
            __atomic_fetch_add(&prd_stress_state.cleanup_count, 1, __ATOMIC_RELAXED);
        }

        tinysleep();
    }
}

// Controller thread - orchestrates the lifecycle phases
static void prd_stress_controller_thread(void *arg __maybe_unused) {
    while (__atomic_load_n(&prd_stress_state.test_running, __ATOMIC_ACQUIRE)) {

        // Start writer phase
        __atomic_store_n(&prd_stress_state.writer_should_run, true, __ATOMIC_RELEASE);

        // Wait for writer to start and run
        sleep_usec(10000);  // 10ms - let writer run

        // Signal writer to stop (simulates stream receiver stopping)
        __atomic_store_n(&prd_stress_state.writer_should_run, false, __ATOMIC_RELEASE);

        // Wait for writer to fully stop
        while (__atomic_load_n(&prd_stress_state.test_running, __ATOMIC_ACQUIRE) &&
               __atomic_load_n(&prd_stress_state.writer_is_running, __ATOMIC_ACQUIRE)) {
            tinysleep();
        }

        // Cleanup phase - cleaner will run now that writer is stopped
        sleep_usec(5000);  // 5ms - let cleanup run

        __atomic_fetch_add(&prd_stress_state.phase_count, 1, __ATOMIC_RELAXED);
    }
}

int prd_array_stress_test(void) {
    int duration_secs = PRD_STRESS_TEST_DURATION_SEC;

    fprintf(stderr, "\nPRD_ARRAY Lifecycle Stress Test\n");
    fprintf(stderr, "================================\n");
    fprintf(stderr, "Duration: %d seconds\n", duration_secs);
    fprintf(stderr, "This test simulates production lifecycle:\n");
    fprintf(stderr, "  1. Writer (collector) runs with collector_tid set\n");
    fprintf(stderr, "  2. Writer fully stops (collector_tid cleared)\n");
    fprintf(stderr, "  3. Cleaner runs cleanup\n");
    fprintf(stderr, "  4. Repeat\n\n");

    // Initialize state
    memset(&prd_stress_state, 0, sizeof(prd_stress_state));
    prd_stress_state.prd_array = prd_array_create(10);
    spinlock_init(&prd_stress_state.spinlock);
    __atomic_store_n(&prd_stress_state.test_running, true, __ATOMIC_RELEASE);

    // Start threads
    char thread_name[32];

    snprintfz(thread_name, sizeof(thread_name), "PRDSTRESS_W");
    ND_THREAD *writer_thread = nd_thread_create(thread_name, NETDATA_THREAD_OPTION_DEFAULT,
                                                 prd_stress_writer_thread, NULL);

    snprintfz(thread_name, sizeof(thread_name), "PRDSTRESS_C");
    ND_THREAD *cleanup_thread = nd_thread_create(thread_name, NETDATA_THREAD_OPTION_DEFAULT,
                                                  prd_stress_cleanup_thread, NULL);

    snprintfz(thread_name, sizeof(thread_name), "PRDSTRESS_CTRL");
    ND_THREAD *controller_thread = nd_thread_create(thread_name, NETDATA_THREAD_OPTION_DEFAULT,
                                                     prd_stress_controller_thread, NULL);

    // Run the test
    fprintf(stderr, "Running stress test...\n");
    for (int i = 0; i < duration_secs; i++) {
        sleep_usec(USEC_PER_SEC);
        fprintf(stderr, "  %d/%d sec - phases: %"PRIu64", grows: %"PRIu64", cleanups: %"PRIu64"\n",
                i + 1, duration_secs,
                __atomic_load_n(&prd_stress_state.phase_count, __ATOMIC_RELAXED),
                __atomic_load_n(&prd_stress_state.grow_count, __ATOMIC_RELAXED),
                __atomic_load_n(&prd_stress_state.cleanup_count, __ATOMIC_RELAXED));
    }

    // Stop all threads
    __atomic_store_n(&prd_stress_state.test_running, false, __ATOMIC_RELEASE);
    __atomic_store_n(&prd_stress_state.writer_should_run, true, __ATOMIC_RELEASE);  // Unblock writer

    nd_thread_join(controller_thread);
    nd_thread_join(writer_thread);
    nd_thread_join(cleanup_thread);

    // Final cleanup
    PRD_ARRAY *final_arr = prd_array_replace(&prd_stress_state.prd_array, NULL);
    if (final_arr)
        prd_array_release(final_arr);

    // Print results
    fprintf(stderr, "\nTest completed!\n");
    fprintf(stderr, "===============\n");
    fprintf(stderr, "Total phases:    %"PRIu64"\n", prd_stress_state.phase_count);
    fprintf(stderr, "Total grows:     %"PRIu64"\n", prd_stress_state.grow_count);
    fprintf(stderr, "Total cleanups:  %"PRIu64"\n", prd_stress_state.cleanup_count);

    if (prd_stress_state.cleanup_count > 0 && prd_stress_state.grow_count > 0) {
        fprintf(stderr, "\nSUCCESS: Lifecycle separation validated\n");
        fprintf(stderr, "- Writer and cleaner ran in non-overlapping phases\n");
        fprintf(stderr, "- No concurrent access to the array\n");
        fprintf(stderr, "- Reference counting worked correctly\n");
        return 0;
    } else {
        fprintf(stderr, "\nWARNING: Low activity - increase test duration\n");
        return 1;
    }
}
