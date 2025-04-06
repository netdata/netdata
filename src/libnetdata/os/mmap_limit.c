// SPDX-License-Identifier: GPL-3.0-or-later

#include "mmap_limit.h"
#include "libnetdata/libnetdata.h"

unsigned long long os_mmap_limit(void) {
    static unsigned long long cached_limit = 0;

    if (cached_limit)
        return cached_limit;

#if defined(OS_LINUX)
    if(read_single_number_file("/proc/sys/vm/max_map_count", &cached_limit) != 0)
        cached_limit = 65536;
#else
    // For other operating systems, assume no limit.
    cached_limit = UINT32_MAX;
#endif

    return cached_limit;
}
