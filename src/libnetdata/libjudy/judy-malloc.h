// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JUDY_MALLOC_H
#define NETDATA_JUDY_MALLOC_H

#include "../libnetdata.h"

size_t judy_aral_free_bytes(void);
size_t judy_aral_structures(void);
struct aral_statistics *judy_aral_statistics(void);

void JudyAllocThreadPulseReset(void);
int64_t JudyAllocThreadPulseGetAndReset(void);

#endif //NETDATA_JUDY_MALLOC_H
