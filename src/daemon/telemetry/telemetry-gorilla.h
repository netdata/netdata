// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TELEMETRY_GORILLA_H
#define NETDATA_TELEMETRY_GORILLA_H

#include "daemon/common.h"

void telemetry_gorilla_hot_buffer_added();
void telemetry_gorilla_tier0_page_flush(uint32_t actual, uint32_t optimal, uint32_t original);

#if defined(TELEMETRY_INTERNALS)
void telemetry_gorilla_do(bool extended);
#endif

#endif //NETDATA_TELEMETRY_GORILLA_H
