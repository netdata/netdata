// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDHOST_SLOTS_H
#define NETDATA_RRDHOST_SLOTS_H

#include "rrdhost.h"

void rrdhost_pluginsd_send_chart_slots_free(RRDHOST *host);
void rrdhost_pluginsd_receive_chart_slots_free(RRDHOST *host);


#endif //NETDATA_RRDHOST_SLOTS_H
