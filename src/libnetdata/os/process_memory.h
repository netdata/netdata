// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PROCESS_MEMORY_H
#define NETDATA_PROCESS_MEMORY_H

#include "libnetdata/libnetdata.h"

/**
 * Process memory information
 *
 * This structure contains memory usage information for a process.
 */
typedef struct {
    uint64_t rss;             // Resident Set Size in bytes
    uint64_t virtual_size;    // Virtual memory size in bytes
    uint64_t shared;          // Shared memory in bytes
    uint64_t text;            // Text (code) size in bytes
    uint64_t data;            // Data size in bytes
    uint64_t max_rss;         // Peak resident set size in bytes
} OS_PROCESS_MEMORY;

/**
 * Check if the process memory information is valid
 */
#define OS_PROCESS_MEMORY_OK(proc_mem) ((proc_mem).rss > 0)

/**
 * Empty process memory structure
 */
#define OS_PROCESS_MEMORY_EMPTY (OS_PROCESS_MEMORY){ 0 }

/**
 * Get process memory information
 *
 * Returns memory information for the specified process, or the current
 * process if pid is 0.
 *
 * @param pid The process ID or 0 for current process
 * @return OS_PROCESS_MEMORY The memory information
 */
OS_PROCESS_MEMORY os_process_memory(pid_t pid);

#endif // NETDATA_PROCESS_MEMORY_H