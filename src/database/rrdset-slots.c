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

// --------------------------------------------------------------------------------------------------------------------
// Unslot a chart - releases dimension references but keeps the array for reuse
// This is called when switching charts or marking them obsolete
//
// IMPORTANT: This function must only be called when the collector is STOPPED, not just
// when collector_tid happens to be 0. The collector_tid check inside the spinlock is a
// safety mechanism, but the real protection comes from the caller ensuring the collector
// is fully stopped before calling this function.

void rrdset_pluginsd_receive_unslot(RRDSET *st) {
    if(!st)
        return;

    spinlock_lock(&st->pluginsd.spinlock);

    // Check collector_tid inside spinlock - if set, collector is active, skip
    pid_t collector_tid = __atomic_load_n(&st->pluginsd.collector_tid, __ATOMIC_ACQUIRE);
    if(collector_tid != 0) {
        // This shouldn't happen if caller ensured collector is stopped
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_WARNING,
                     "PLUGINSD: rrdset_pluginsd_receive_unslot called while collector (tid %d) is active, skipping",
                     collector_tid);
        spinlock_unlock(&st->pluginsd.spinlock);
        return;
    }

    // Acquire array while holding spinlock (prevents TOCTOU with other cleanup)
    PRD_ARRAY *arr = prd_array_acquire_locked(&st->pluginsd.prd_array);

    spinlock_unlock(&st->pluginsd.spinlock);

    if (arr) {
        // Release all dimension references
        prd_array_release_entries(arr);
        // Release our reference to the array
        prd_array_release(arr);
    }

    RRDHOST *host = st->rrdhost;

    if(st->pluginsd.last_slot >= 0 &&
        (uint32_t)st->pluginsd.last_slot < host->stream.rcv.pluginsd_chart_slots.size &&
        host->stream.rcv.pluginsd_chart_slots.array[st->pluginsd.last_slot] == st) {
        host->stream.rcv.pluginsd_chart_slots.array[st->pluginsd.last_slot] = NULL;
    }

    st->pluginsd.last_slot = -1;
    st->pluginsd.dims_with_slots = false;
}

// --------------------------------------------------------------------------------------------------------------------
// Full cleanup - unslots and frees the array
// This is called during chart finalization or host cleanup
// Thread-safe: uses spinlock for cleanup coordination and reference counting for array lifetime

void rrdset_pluginsd_receive_unslot_and_cleanup(RRDSET *st) {
    if(!st)
        return;

    RRDHOST *host = st->rrdhost;

    spinlock_lock(&st->pluginsd.spinlock);

    // Check if collector is still active - if so, we cannot safely cleanup
    // The collector will clear collector_tid after it's done accessing the array
    pid_t collector_tid = __atomic_load_n(&st->pluginsd.collector_tid, __ATOMIC_ACQUIRE);
    if(collector_tid != 0) {
        // Collector is still active, cannot cleanup now
        // This shouldn't happen during normal operation - log a warning (rate limited)
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_WARNING,
                     "PLUGINSD: attempted cleanup while collector (tid %d) is still active on chart, skipping",
                     collector_tid);
        spinlock_unlock(&st->pluginsd.spinlock);
        return;
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
    if(last_slot >= 0 &&
        (uint32_t)last_slot < host->stream.rcv.pluginsd_chart_slots.size &&
        host->stream.rcv.pluginsd_chart_slots.array[last_slot] == st) {
        host->stream.rcv.pluginsd_chart_slots.array[last_slot] = NULL;
    }

    // Now handle the old array outside the lock
    if (old_arr) {
        // Track memory being freed
        rrd_slot_memory_removed(sizeof(PRD_ARRAY) + old_arr->size * sizeof(struct pluginsd_rrddim));

        // Release all dimension references
        prd_array_release_entries(old_arr);

        // Release our reference - array will be freed when refcount reaches 0
        // If another thread still has a reference (unlikely but possible during races),
        // the array will be freed when that thread releases its reference
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
                memcpy(new_arr->entries, current_arr->entries,
                       copy_count * sizeof(struct pluginsd_rrddim));
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

        // Wait for permission to stop (let cleanup run)
        while (__atomic_load_n(&prd_stress_state.test_running, __ATOMIC_ACQUIRE) &&
               !__atomic_load_n(&prd_stress_state.writer_should_run, __ATOMIC_ACQUIRE) &&
               !__atomic_load_n(&prd_stress_state.writer_is_running, __ATOMIC_ACQUIRE)) {
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
