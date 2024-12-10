// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_CONTROL_H
#define NETDATA_STREAM_CONTROL_H

#include "libnetdata/libnetdata.h"

#define STREAM_CONTROL_SLEEP_UT (10 * USEC_PER_MS + os_random(10 * USEC_PER_MS))

#define stream_control_throttle() microsleep(STREAM_CONTROL_SLEEP_UT)

bool stream_control_ml_should_be_running(void);
bool stream_control_children_should_be_accepted(void);
bool stream_control_replication_should_be_running(void);
bool stream_control_health_should_be_running(void);

#endif //NETDATA_STREAM_CONTROL_H
