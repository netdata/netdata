// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_GORILLA_H
#define NETDATA_PULSE_GORILLA_H

#include "daemon/common.h"

void pulse_gorilla_hot_buffer_added();
void pulse_gorilla_tier0_page_flush(uint32_t actual, uint32_t optimal, uint32_t original);

#if defined(PULSE_INTERNALS)
void pulse_gorilla_do(bool extended);
#endif

#endif //NETDATA_PULSE_GORILLA_H
