// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDSET_SLOTS_H
#define NETDATA_RRDSET_SLOTS_H

#include "rrd.h"

void rrdset_stream_send_chart_slot_assign(RRDSET *st);
void rrdset_stream_send_chart_slot_release(RRDSET *st);

void rrdset_pluginsd_receive_unslot(RRDSET *st);
void rrdset_pluginsd_receive_unslot_and_cleanup(RRDSET *st);
void rrdset_pluginsd_receive_slots_initialize(RRDSET *st);

#endif //NETDATA_RRDSET_SLOTS_H
