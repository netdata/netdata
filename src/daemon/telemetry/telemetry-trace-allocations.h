// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TELEMETRY_TRACE_ALLOCATIONS_H
#define NETDATA_TELEMETRY_TRACE_ALLOCATIONS_H

#include "daemon/common.h"

#if defined(TELEMETRY_INTERNALS)
#ifdef NETDATA_TRACE_ALLOCATIONS
void telemetry_trace_allocations_do(bool extended);
#endif
#endif

#endif //NETDATA_TELEMETRY_TRACE_ALLOCATIONS_H
