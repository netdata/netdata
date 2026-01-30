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
// Thread-safe: uses spinlock + reference counting to protect array access

void rrdset_pluginsd_receive_unslot(RRDSET *st) {
    // Acquire a reference to safely access the array
    // The spinlock prevents races with concurrent replace+release operations
    PRD_ARRAY *arr = prd_array_acquire(&st->pluginsd.prd_array, &st->pluginsd.spinlock);
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
// Stress test for PRD_ARRAY reference counting
// Run with: netdata -W prd-array-stress
// --------------------------------------------------------------------------------------------------------------------

#define PRD_STRESS_TEST_DURATION_SEC 5
#define PRD_STRESS_NUM_READERS 4
#define PRD_STRESS_NUM_WRITERS 1   // Must be 1 - only one collector per chart in real code
#define PRD_STRESS_NUM_CLEANERS 1

typedef struct {
    PRD_ARRAY *prd_array;
    int32_t collector_tid;
    SPINLOCK spinlock;
    bool running;
    uint64_t acquire_count;
    uint64_t release_count;
    uint64_t grow_count;
    uint64_t cleanup_count;
    uint64_t cleanup_skipped;
} prd_stress_state_t;

static prd_stress_state_t prd_stress_state;

static void prd_stress_reader_thread(void *arg __maybe_unused) {
    while (__atomic_load_n(&prd_stress_state.running, __ATOMIC_ACQUIRE)) {
        PRD_ARRAY *arr = prd_array_acquire(&prd_stress_state.prd_array, &prd_stress_state.spinlock);
        if (arr) {
            // Simulate work with the array - read entries
            for (size_t i = 0; i < arr->size && i < 10; i++) {
                volatile void *dummy = arr->entries[i].rd;
                (void)dummy;
            }
            __atomic_fetch_add(&prd_stress_state.acquire_count, 1, __ATOMIC_RELAXED);

            // Small delay to increase chance of races
            tinysleep();

            prd_array_release(arr);
            __atomic_fetch_add(&prd_stress_state.release_count, 1, __ATOMIC_RELAXED);
        }
        tinysleep();
    }
}

static void prd_stress_writer_thread(void *arg __maybe_unused) {
    while (__atomic_load_n(&prd_stress_state.running, __ATOMIC_ACQUIRE)) {
        // Simulate collector_tid being set (like pluginsd_set_scope_chart does)
        __atomic_store_n(&prd_stress_state.collector_tid, 1, __ATOMIC_RELEASE);

        PRD_ARRAY *current_arr = prd_array_get_unsafe(&prd_stress_state.prd_array);

        // Determine new size based on current array (or start fresh if NULL)
        size_t current_size = current_arr ? current_arr->size : 0;
        size_t new_size = current_size + 10;

        // Wrap around to avoid unbounded growth
        if (new_size > 1000)
            new_size = 10;

        // Create new array
        PRD_ARRAY *new_arr = prd_array_create(new_size);

        // Copy existing entries (only copy what fits in the new array)
        if (current_arr && current_size > 0) {
            size_t copy_count = (current_size < new_size) ? current_size : new_size;
            memcpy(new_arr->entries, current_arr->entries,
                   copy_count * sizeof(struct pluginsd_rrddim));
        }

        // Initialize new entries with some data
        for (size_t i = current_size; i < new_size; i++) {
            new_arr->entries[i].rd = (void *)(uintptr_t)(i + 1);
            new_arr->entries[i].id = "test";
        }

        // Atomically replace
        PRD_ARRAY *old_arr = prd_array_replace(&prd_stress_state.prd_array, new_arr);

        if (old_arr)
            prd_array_release(old_arr);

        __atomic_fetch_add(&prd_stress_state.grow_count, 1, __ATOMIC_RELAXED);

        // Clear collector_tid
        __atomic_store_n(&prd_stress_state.collector_tid, 0, __ATOMIC_RELEASE);

        usleep(100);
    }
}

static void prd_stress_cleanup_thread(void *arg __maybe_unused) {
    while (__atomic_load_n(&prd_stress_state.running, __ATOMIC_ACQUIRE)) {
        usleep(500);

        spinlock_lock(&prd_stress_state.spinlock);

        // Check if collector is active (like rrdset_pluginsd_receive_unslot_and_cleanup does)
        int32_t collector_tid = __atomic_load_n(&prd_stress_state.collector_tid, __ATOMIC_ACQUIRE);
        if (collector_tid != 0) {
            __atomic_fetch_add(&prd_stress_state.cleanup_skipped, 1, __ATOMIC_RELAXED);
            spinlock_unlock(&prd_stress_state.spinlock);
            continue;
        }

        // Replace array with NULL
        PRD_ARRAY *old_arr = prd_array_replace(&prd_stress_state.prd_array, NULL);

        spinlock_unlock(&prd_stress_state.spinlock);

        if (old_arr) {
            // Simulate clearing entries
            for (size_t i = 0; i < old_arr->size; i++) {
                old_arr->entries[i].rda = NULL;
                old_arr->entries[i].rd = NULL;
                old_arr->entries[i].id = NULL;
            }

            prd_array_release(old_arr);
            __atomic_fetch_add(&prd_stress_state.cleanup_count, 1, __ATOMIC_RELAXED);
        }
    }
}

int prd_array_stress_test(void) {
    int num_readers = PRD_STRESS_NUM_READERS;
    int num_writers = PRD_STRESS_NUM_WRITERS;
    int num_cleaners = PRD_STRESS_NUM_CLEANERS;
    int duration_secs = PRD_STRESS_TEST_DURATION_SEC;
    int total_threads = num_readers + num_writers + num_cleaners;

    fprintf(stderr, "\nPRD_ARRAY Reference Counting Stress Test\n");
    fprintf(stderr, "=========================================\n");
    fprintf(stderr, "Duration: %d seconds\n", duration_secs);
    fprintf(stderr, "Readers: %d, Writers: %d, Cleaners: %d\n\n", num_readers, num_writers, num_cleaners);

    // Initialize state
    memset(&prd_stress_state, 0, sizeof(prd_stress_state));
    prd_stress_state.prd_array = prd_array_create(10);
    spinlock_init(&prd_stress_state.spinlock);
    __atomic_store_n(&prd_stress_state.running, true, __ATOMIC_RELEASE);

    // Start threads
    ND_THREAD **threads = callocz(total_threads, sizeof(ND_THREAD *));
    char thread_name[32];

    int t = 0;
    for (int i = 0; i < num_readers; i++) {
        snprintfz(thread_name, sizeof(thread_name), "PRDSTRESS_R%d", i);
        threads[t++] = nd_thread_create(thread_name, NETDATA_THREAD_OPTION_DEFAULT,
                                        prd_stress_reader_thread, NULL);
    }
    for (int i = 0; i < num_writers; i++) {
        snprintfz(thread_name, sizeof(thread_name), "PRDSTRESS_W%d", i);
        threads[t++] = nd_thread_create(thread_name, NETDATA_THREAD_OPTION_DEFAULT,
                                        prd_stress_writer_thread, NULL);
    }
    for (int i = 0; i < num_cleaners; i++) {
        snprintfz(thread_name, sizeof(thread_name), "PRDSTRESS_C%d", i);
        threads[t++] = nd_thread_create(thread_name, NETDATA_THREAD_OPTION_DEFAULT,
                                        prd_stress_cleanup_thread, NULL);
    }

    // Run the test
    fprintf(stderr, "Running stress test...\n");
    for (int i = 0; i < duration_secs; i++) {
        sleep_usec(USEC_PER_SEC);
        fprintf(stderr, "  %d/%d sec - acquires: %"PRIu64", releases: %"PRIu64", grows: %"PRIu64", cleanups: %"PRIu64", skipped: %"PRIu64"\n",
                i + 1, duration_secs,
                __atomic_load_n(&prd_stress_state.acquire_count, __ATOMIC_RELAXED),
                __atomic_load_n(&prd_stress_state.release_count, __ATOMIC_RELAXED),
                __atomic_load_n(&prd_stress_state.grow_count, __ATOMIC_RELAXED),
                __atomic_load_n(&prd_stress_state.cleanup_count, __ATOMIC_RELAXED),
                __atomic_load_n(&prd_stress_state.cleanup_skipped, __ATOMIC_RELAXED));
    }

    // Stop threads
    __atomic_store_n(&prd_stress_state.running, false, __ATOMIC_RELEASE);

    for (int i = 0; i < total_threads; i++) {
        nd_thread_join(threads[i]);
    }

    // Final cleanup
    PRD_ARRAY *final_arr = prd_array_replace(&prd_stress_state.prd_array, NULL);
    if (final_arr)
        prd_array_release(final_arr);

    freez(threads);

    // Print results
    uint64_t acquires = __atomic_load_n(&prd_stress_state.acquire_count, __ATOMIC_RELAXED);
    uint64_t releases = __atomic_load_n(&prd_stress_state.release_count, __ATOMIC_RELAXED);

    fprintf(stderr, "\nTest completed!\n");
    fprintf(stderr, "===============\n");
    fprintf(stderr, "Total acquires:        %"PRIu64"\n", acquires);
    fprintf(stderr, "Total releases:        %"PRIu64"\n", releases);
    fprintf(stderr, "Total grows:           %"PRIu64"\n", prd_stress_state.grow_count);
    fprintf(stderr, "Total cleanups:        %"PRIu64"\n", prd_stress_state.cleanup_count);
    fprintf(stderr, "Total cleanup skipped: %"PRIu64"\n", prd_stress_state.cleanup_skipped);

    int rc = 0;
    if (acquires == releases) {
        fprintf(stderr, "\nSUCCESS: All acquires have matching releases (no leaks)\n");
    } else {
        fprintf(stderr, "\nFAILED: Acquire/release mismatch - possible leak\n");
        rc = 1;
    }

    return rc;
}
