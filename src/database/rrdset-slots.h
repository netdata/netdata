// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDSET_SLOTS_H
#define NETDATA_RRDSET_SLOTS_H

#include "rrd.h"

void rrdset_stream_send_chart_slot_assign(RRDSET *st);
void rrdset_stream_send_chart_slot_release(RRDSET *st);

void rrdhost_pluginsd_send_chart_slots_free(RRDHOST *host);
void rrdset_pluginsd_receive_unslot(RRDSET *st);
void rrdset_pluginsd_receive_unslot_and_cleanup(RRDSET *st);
void rrdset_pluginsd_receive_slots_initialize(RRDSET *st);
void rrdhost_pluginsd_receive_chart_slots_free(RRDHOST *host);

#endif //NETDATA_RRDSET_SLOTS_H
