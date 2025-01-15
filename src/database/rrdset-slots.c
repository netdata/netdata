// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdset-slots.h"

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

void rrdset_pluginsd_receive_unslot(RRDSET *st) {
    for(size_t i = 0; i < st->pluginsd.size ;i++) {
        rrddim_acquired_release(st->pluginsd.prd_array[i].rda); // can be NULL
        st->pluginsd.prd_array[i].rda = NULL;
        st->pluginsd.prd_array[i].rd = NULL;
        st->pluginsd.prd_array[i].id = NULL;
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

void rrdset_pluginsd_receive_unslot_and_cleanup(RRDSET *st) {
    if(!st)
        return;

    spinlock_lock(&st->pluginsd.spinlock);

    rrdset_pluginsd_receive_unslot(st);

    rrd_slot_memory_removed(st->pluginsd.size * sizeof(struct pluginsd_rrddim));
    freez(st->pluginsd.prd_array);
    st->pluginsd.prd_array = NULL;
    st->pluginsd.size = 0;
    st->pluginsd.pos = 0;
    st->pluginsd.set = false;
    st->pluginsd.last_slot = -1;
    st->pluginsd.dims_with_slots = false;
    st->pluginsd.collector_tid = 0;

    spinlock_unlock(&st->pluginsd.spinlock);
}

void rrdset_pluginsd_receive_slots_initialize(RRDSET *st) {
    spinlock_init(&st->pluginsd.spinlock);
    st->pluginsd.last_slot = -1;
}
