// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_CONTROL_H
#define NETDATA_STREAM_CONTROL_H

#include "libnetdata/libnetdata.h"

bool stream_ml_should_be_running(void);
bool stream_children_should_be_accepted(void);
bool stream_replication_should_be_running(void);
bool stream_health_should_be_running(void);

#endif //NETDATA_STREAM_CONTROL_H
