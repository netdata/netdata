// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDSET_SLOTS_H
#define NETDATA_RRDSET_SLOTS_H

#include "rrd.h"

void rrdset_stream_send_chart_slot_assign(RRDSET *st);
void rrdset_stream_send_chart_slot_release(RRDSET *st);

// IMPORTANT: The following cleanup functions must only be called when the collector
// is FULLY STOPPED on the chart (not just when collector_tid == 0). The lifecycle
// guarantee (collector stopped before cleanup) provides the real safety - the
// collector_tid check is a secondary safety mechanism.
void rrdset_pluginsd_receive_unslot(RRDSET *st);
void rrdset_pluginsd_receive_unslot_and_cleanup(RRDSET *st);

void rrdset_pluginsd_receive_slots_initialize(RRDSET *st);

// Stress test for PRD_ARRAY lifecycle separation - run with -W prd-array-stress
int prd_array_stress_test(void);

#endif //NETDATA_RRDSET_SLOTS_H
