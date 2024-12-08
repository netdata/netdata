// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_WORKERS_H
#define NETDATA_PULSE_WORKERS_H

#include "daemon/common.h"

#if defined(PULSE_INTERNALS)
void pulse_workers_do(bool extended);
void pulse_workers_cleanup(void);
#endif

#endif //NETDATA_PULSE_WORKERS_H
