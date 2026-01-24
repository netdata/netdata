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
// Thread-safe: uses reference counting to protect array access

void rrdset_pluginsd_receive_unslot(RRDSET *st) {
    // Acquire a reference to safely access the array
    PRD_ARRAY *arr = prd_array_acquire(&st->pluginsd.prd_array);
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

    spinlock_lock(&st->pluginsd.spinlock);

    // Check if collector is still active - if so, we cannot safely cleanup
    // The collector will clear collector_tid after it's done accessing the array
    pid_t collector_tid = __atomic_load_n(&st->pluginsd.collector_tid, __ATOMIC_ACQUIRE);
    if(collector_tid != 0) {
        // Collector is still active, cannot cleanup now
        // This shouldn't happen during normal operation - log a warning
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_WARNING,
                     "PLUGINSD: attempted cleanup while collector (tid %d) is still active on chart, skipping",
                     collector_tid);
        spinlock_unlock(&st->pluginsd.spinlock);
        return;
    }

    // Replace the array with NULL - this prevents new references from being acquired
    PRD_ARRAY *old_arr = prd_array_replace(&st->pluginsd.prd_array, NULL);

    // Reset other state while holding the lock
    __atomic_store_n(&st->pluginsd.pos, 0, __ATOMIC_RELAXED);
    st->pluginsd.set = false;
    st->pluginsd.last_slot = -1;
    st->pluginsd.dims_with_slots = false;

    spinlock_unlock(&st->pluginsd.spinlock);

    // Now handle the old array outside the lock
    if (old_arr) {
        // Track memory being freed
        rrd_slot_memory_removed(sizeof(PRD_ARRAY) + old_arr->size * sizeof(struct pluginsd_rrddim));

        // Release all dimension references
        prd_array_release_entries(old_arr);

        // Clear the chart slot mapping
        RRDHOST *host = st->rrdhost;
        if(st->pluginsd.last_slot >= 0 &&
            (uint32_t)st->pluginsd.last_slot < host->stream.rcv.pluginsd_chart_slots.size &&
            host->stream.rcv.pluginsd_chart_slots.array[st->pluginsd.last_slot] == st) {
            host->stream.rcv.pluginsd_chart_slots.array[st->pluginsd.last_slot] = NULL;
        }

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
