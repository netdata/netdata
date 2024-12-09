// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_TRACE_ALLOCATIONS_H
#define NETDATA_PULSE_TRACE_ALLOCATIONS_H

#include "daemon/common.h"

#if defined(PULSE_INTERNALS)
#ifdef NETDATA_TRACE_ALLOCATIONS
void pulse_trace_allocations_do(bool extended);
#endif
#endif

#endif //NETDATA_PULSE_TRACE_ALLOCATIONS_H
