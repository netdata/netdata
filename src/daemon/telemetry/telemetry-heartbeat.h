// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TELEMETRY_HEARTBEAT_H
#define NETDATA_TELEMETRY_HEARTBEAT_H

#include "daemon/common.h"

#if defined(TELEMETRY_INTERNALS)
void telemetry_heartbeat_do(bool extended);
#endif

#endif //NETDATA_TELEMETRY_HEARTBEAT_H
