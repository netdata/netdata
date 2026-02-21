// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdhost-slots.h"
#include "rrdset.h"

void rrdhost_pluginsd_send_chart_slots_free(RRDHOST *host) {
    rrd_slot_memory_removed(host->stream.snd.pluginsd_chart_slots.available.size * sizeof(uint32_t));

    spinlock_lock(&host->stream.snd.pluginsd_chart_slots.available.spinlock);
    host->stream.snd.pluginsd_chart_slots.available.ignore = true;
    freez(host->stream.snd.pluginsd_chart_slots.available.array);
    host->stream.snd.pluginsd_chart_slots.available.array = NULL;
    host->stream.snd.pluginsd_chart_slots.available.used = 0;
    host->stream.snd.pluginsd_chart_slots.available.size = 0;
    spinlock_unlock(&host->stream.snd.pluginsd_chart_slots.available.spinlock);

    // zero all the slots on all charts, so that they will not attempt to access the array
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        st->stream.snd.chart_slot = 0;
    }
    rrdset_foreach_done(st);
}

void rrdhost_pluginsd_receive_chart_slots_free(RRDHOST *host) {
    rrd_slot_memory_removed(host->stream.rcv.pluginsd_chart_slots.size * sizeof(uint32_t));

    spinlock_lock(&host->stream.rcv.pluginsd_chart_slots.spinlock);

    if(host->stream.rcv.pluginsd_chart_slots.array) {
        for (size_t s = 0; s < host->stream.rcv.pluginsd_chart_slots.size; s++) {
            RRDSET *st = host->stream.rcv.pluginsd_chart_slots.array[s];
            if(st) {
                // Clear collector_tid - the collector is already stopped
                // (stream_receiver_signal_to_stop_and_wait was called before this)
                // so it's safe to cleanup regardless of the previous collector_tid value
                __atomic_store_n(&st->pluginsd.collector_tid, 0, __ATOMIC_RELEASE);

                // Pre-clear last_slot so that rrdset_pluginsd_receive_unslot_and_cleanup
                // won't try to re-acquire the host spinlock we already hold.
                // We're freeing the entire host slots array below, so clearing individual
                // slot entries is unnecessary.
                st->pluginsd.last_slot = -1;

                rrdset_pluginsd_receive_unslot_and_cleanup(st);
            }
        }

        freez(host->stream.rcv.pluginsd_chart_slots.array);
        host->stream.rcv.pluginsd_chart_slots.array = NULL;
        host->stream.rcv.pluginsd_chart_slots.size = 0;
    }

    spinlock_unlock(&host->stream.rcv.pluginsd_chart_slots.spinlock);
}
