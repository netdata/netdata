// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDCOLLECTOR_H
#define NETDATA_RRDCOLLECTOR_H

#include "rrd.h"

// ----------------------------------------------------------------------------
// public API

void rrd_collector_started(void);
void rrd_collector_finished(void);

#endif //NETDATA_RRDCOLLECTOR_H
