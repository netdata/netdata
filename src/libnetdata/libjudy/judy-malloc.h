// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JUDY_MALLOC_H
#define NETDATA_JUDY_MALLOC_H

#include "../libnetdata.h"

size_t judy_aral_free_bytes(void);
size_t judy_aral_structures(void);
struct aral_statistics *judy_aral_statistics(void);

void JudyAllocThreadPulseReset(void);
int64_t JudyAllocThreadPulseGetAndReset(void);

typedef void (*JUDY_ALLOCATOR_SCOPED_CALLBACK)(void *data);
// The callback scope may nest: every invocation restores the complete prior thread-local state.
// Judy arrays allocated from owa must not outlive that arena.
void JudyAllocThreadScopedOWA(ONEWAYALLOC *owa, JUDY_ALLOCATOR_SCOPED_CALLBACK callback, void *data);

void libjudy_malloc_init(void);

#endif //NETDATA_JUDY_MALLOC_H
