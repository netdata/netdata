// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDSET_SLOTS_H
#define NETDATA_RRDSET_SLOTS_H

#include "rrd.h"

void rrdset_stream_send_chart_slot_assign(RRDSET *st);
void rrdset_stream_send_chart_slot_release(RRDSET *st);

// rrdset_pluginsd_receive_unslot: Releases dimension references but keeps the array.
// Safe to call from the collector thread itself (detected via collector_tid == gettid_cached())
// or when the collector is fully stopped.
void rrdset_pluginsd_receive_unslot(RRDSET *st);

// rrdset_pluginsd_receive_unslot_and_cleanup: Full cleanup - releases dimension references
// AND frees the array. Must only be called when the collector is FULLY STOPPED on the chart.
// The collector_tid check is a safety mechanism (fires internal_fatal in debug builds).
void rrdset_pluginsd_receive_unslot_and_cleanup(RRDSET *st);

void rrdset_pluginsd_receive_slots_initialize(RRDSET *st);

// Stress test for PRD_ARRAY lifecycle separation - run with -W prd-array-stress
int prd_array_stress_test(void);

#endif //NETDATA_RRDSET_SLOTS_H
