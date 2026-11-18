// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_MEM_AVAILABLE_H
#define NETDATA_OS_MEM_AVAILABLE_H

#include "libnetdata/libnetdata.h"

typedef struct {
    // ram_total_bytes is the total memory available in the system
    // and it includes all the physical RAM the system may have.
    // It does not include non-RAM memory (i.e. SWAP in not included).
    // ram_total_bytes is refreshed less often than ram_available_bytes.
    uint64_t ram_total_bytes;

    // ram_available_bytes is the total RAM memory available for
    // applications, before the system and its applications run
    // out-of-memory.
    // It does not include non-RAM memory (i.e. SWAP in not included).
    uint64_t ram_available_bytes;
} OS_SYSTEM_MEMORY;

#define OS_SYSTEM_MEMORY_OK(mem) ((mem).ram_total_bytes > 0)
#define OS_SYSTEM_MEMORY_EMPTY (OS_SYSTEM_MEMORY){ 0 }

// System memory is read by one sampler thread, so that a read that stalls (e.g. /proc/meminfo served by a hung
// lxcfs) blocks only that thread. Everybody else gets the last published sample, without any I/O.

// A sample older than this is stale: decisions that need current memory (page cache sizing, low-memory triggers)
// must not act on it.
#define OS_SYSTEM_MEMORY_MAX_AGE_UT (10 * USEC_PER_SEC)

// The last published sample, of any age. If nothing has been read yet, the first caller reads the system memory
// once (concurrent callers wait for it) - this may block at startup. After that first read, it never does I/O.
OS_SYSTEM_MEMORY os_system_memory(void);

// The last published sample if it is younger than max_age_ut (0 = any age), otherwise OS_SYSTEM_MEMORY_EMPTY.
// Never does I/O. Safe on any thread, including fatal and out-of-memory paths.
OS_SYSTEM_MEMORY os_system_memory_cached(usec_t max_age_ut);

// Start the sampler thread. Call it once, after the process has forked into the background.
void os_system_memory_sampler_start(void);

// Stop the sampler thread, waiting at most timeout_ut. Returns false if it is still running (stuck in a read): it is
// then abandoned, and the caller must not run teardown that a running thread could use.
bool os_system_memory_sampler_stop(usec_t timeout_ut);

double os_system_memory_available_percent(OS_SYSTEM_MEMORY mem);

int os_system_memory_unittest(void);

#endif //NETDATA_OS_MEM_AVAILABLE_H
