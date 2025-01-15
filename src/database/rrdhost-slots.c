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
        for (size_t s = 0; s < host->stream.rcv.pluginsd_chart_slots.size; s++)
            rrdset_pluginsd_receive_unslot_and_cleanup(host->stream.rcv.pluginsd_chart_slots.array[s]);

        freez(host->stream.rcv.pluginsd_chart_slots.array);
        host->stream.rcv.pluginsd_chart_slots.array = NULL;
        host->stream.rcv.pluginsd_chart_slots.size = 0;
    }

    spinlock_unlock(&host->stream.rcv.pluginsd_chart_slots.spinlock);
}
