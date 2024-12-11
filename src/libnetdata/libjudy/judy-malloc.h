// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JUDY_MALLOC_H
#define NETDATA_JUDY_MALLOC_H

#include "../libnetdata.h"

size_t judy_aral_overhead(void);
size_t judy_aral_structures(void);

void JudyAllocThreadTelemetryReset(void);
int64_t JudyAllocThreadTelemetryGetAndReset(void);

#endif //NETDATA_JUDY_MALLOC_H
