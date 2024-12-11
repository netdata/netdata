// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_CONTROL_H
#define NETDATA_STREAM_CONTROL_H

#include "libnetdata/libnetdata.h"

#define STREAM_CONTROL_SLEEP_UT (10 * USEC_PER_MS + os_random(10 * USEC_PER_MS))

#define stream_control_throttle() microsleep(STREAM_CONTROL_SLEEP_UT)

void stream_control_backfill_query_started(void);
void stream_control_backfill_query_finished(void);

void stream_control_replication_query_started(void);
void stream_control_replication_query_finished(void);

void stream_control_user_weights_query_started(void);
void stream_control_user_weights_query_finished(void);

void stream_control_user_data_query_started(void);
void stream_control_user_data_query_finished(void);

bool stream_control_ml_should_be_running(void);
bool stream_control_children_should_be_accepted(void);
bool stream_control_replication_should_be_running(void);
bool stream_control_health_should_be_running(void);

#endif //NETDATA_STREAM_CONTROL_H
