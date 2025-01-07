// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_MEM_AVAILABLE_H
#define NETDATA_OS_MEM_AVAILABLE_H

#include "libnetdata/libnetdata.h"

typedef struct {
    // ram_total_bytes is the total memory available in the system
    // and it includes all the physical RAM the system may have.
    // It does not include non-RAM memory (i.e. SWAP in not included).
    // ram_total_bytes may be cached between calls to these functions
    // (i.e. it may not be as up to date as ram_available_bytes).
    uint64_t ram_total_bytes;

    // ram_available_bytes is the total RAM memory available for
    // applications, before the system and its applications run
    // out-of-memory. This is always provided live by querying the system.
    // It does not include non-RAM memory (i.e. SWAP in not included).
    uint64_t ram_available_bytes;
} OS_SYSTEM_MEMORY;

// The function to get current system memory:
OS_SYSTEM_MEMORY os_system_memory(bool query_total_ram);

// Returns the last successfully reported os_system_memory() value.
OS_SYSTEM_MEMORY os_last_reported_system_memory(void);

#endif //NETDATA_OS_MEM_AVAILABLE_H
