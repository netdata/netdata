// SPDX-License-Identifier: GPL-3.0-or-later

#include "mmap_limit.h"
#include "libnetdata/libnetdata.h"

static unsigned long long cached_limit = 0;
static bool cached_limit_available = false;
static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

unsigned long long os_mmap_limit(void) {
    if (__atomic_load_n(&cached_limit_available, __ATOMIC_ACQUIRE))
        return cached_limit;

    spinlock_lock(&spinlock);

    unsigned long long limit = cached_limit;
    if (!limit) {
#if defined(OS_LINUX)
        if(read_single_number_file("/proc/sys/vm/max_map_count", &limit) != 0)
            limit = 65536;
#else
        // For other operating systems, assume no limit.
        limit = UINT32_MAX;
#endif
        cached_limit = limit;

        bool limit_available = (bool)limit;
        __atomic_store_n(&cached_limit_available, limit_available, __ATOMIC_RELEASE);
    }

    spinlock_unlock(&spinlock);

    return limit;
}
