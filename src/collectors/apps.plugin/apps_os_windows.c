// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

#if defined(OS_WINDOWS)

bool read_proc_pid_status_per_os(struct pid_stat *p, void *ptr) {
    // TODO: get these statistics from perflib
    return false;
}

bool read_proc_pid_stat_per_os(struct pid_stat *p, void *ptr) {
    struct perflib_data *d = ptr;

    // TODO: get these statistics from perflib

    return false;
}

bool read_proc_pid_limits_per_os(struct pid_stat *p __maybe_unused, void *ptr __maybe_unused) {
    return false;
}

bool read_proc_pid_io_per_os(struct pid_stat *p, void *ptr) {
    // TODO: get I/O throughput per process from perflib
    return false;
}

bool get_cmdline_per_os(struct pid_stat *p, char *cmdline, size_t bytes) {
    // TODO: get the command line from perflib, if available
    return false;
}

bool read_pid_file_descriptors_per_os(struct pid_stat *p, void *ptr) {
    // TODO: get file descriptors per process, if available
    return false;
}

bool get_MemTotal_per_os(void) {
    MEMORYSTATUSEX memStat = { 0 };
    memStat.dwLength = sizeof(memStat);

    if (!GlobalMemoryStatusEx(&memStat)) {
        netdata_log_error("GlobalMemoryStatusEx() failed.");
        return false;
    }

    MemTotal = memStat.ullTotalPhys;
    return true;
}

bool apps_os_collect(void) {
    for(struct pid_stat *p = root_of_pids(); p; p = p->next)
        mark_pid_as_unread(p);

    struct perflib_data d = { 0 };
    d.pDataBlock = perflibGetPerformanceData(RegistryFindIDByName("Process"));
    if(!d.pDataBlock) return false;

    d.pObjectType = perflibFindObjectTypeByName(d.pDataBlock, "Process");
    if(!d.pObjectType) {
        perflibFreePerformanceData();
        return false;
    }

    d.pi = NULL;
    for(LONG i = 0; i < d.pObjectType->NumInstances; i++) {
        d.pi = perflibForEachInstance(d.pDataBlock, d.pObjectType, d.pi);
        if(!d.pi) break;

        if(!getInstanceName(d.pDataBlock, d.pObjectType, d.pi, d.name, sizeof(d.name)))
            strncpyz(d.name, "unknown", sizeof(d.name) - 1);

        COUNTER_DATA processId = {.key = "ID Process"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &processId);
        d.pid = (DWORD)processId.current.Data;
        if(d.pid <= 0) continue; // pid 0 is the Idle, which is not useful for us

        // Get or create pid_stat structure
        struct pid_stat *p = get_or_allocate_pid_entry((pid_t)d.pid);
        p->updated = true;

        // Parent Process ID
        COUNTER_DATA ppid = {.key = "Creating Process ID"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &ppid);
        p->ppid = (pid_t)ppid.current.Data;

        // Update process name
        update_pid_comm(p, d.name);

        collect_data_for_pid_stat(p, &d);

        // CPU time
        COUNTER_DATA userTime = {.key = "% User Time"};
        COUNTER_DATA kernelTime = {.key = "% Privileged Time"};
        COUNTER_DATA totalTime = {.key = "% Processor Time"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &userTime);
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &kernelTime);
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &totalTime);
        p->utime = userTime.current.Data;
        p->stime = kernelTime.current.Data;

        // Memory
        COUNTER_DATA workingSet = {.key = "Working Set"};
        COUNTER_DATA privateBytes = {.key = "Private Bytes"};
        COUNTER_DATA virtualBytes = {.key = "Virtual Bytes"};
        COUNTER_DATA pageFileBytes = {.key = "Page File Bytes"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &workingSet);
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &privateBytes);
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &virtualBytes);
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &pageFileBytes);
        p->status_vmrss = workingSet.current.Data / 1024;  // Convert to KB
        p->status_vmsize = virtualBytes.current.Data / 1024;  // Convert to KB
        p->status_vmswap = pageFileBytes.current.Data / 1024;  // Page File Bytes in KB

        // I/O
        COUNTER_DATA ioReadBytes = {.key = "IO Read Bytes/sec"};
        COUNTER_DATA ioWriteBytes = {.key = "IO Write Bytes/sec"};
        COUNTER_DATA ioReadOps = {.key = "IO Read Operations/sec"};
        COUNTER_DATA ioWriteOps = {.key = "IO Write Operations/sec"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &ioReadBytes);
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &ioWriteBytes);
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &ioReadOps);
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &ioWriteOps);
        p->io_logical_bytes_read = ioReadBytes.current.Data;
        p->io_logical_bytes_written = ioWriteBytes.current.Data;
        p->io_read_calls = ioReadOps.current.Data;
        p->io_write_calls = ioWriteOps.current.Data;

        // Threads
        COUNTER_DATA threadCount = {.key = "Thread Count"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &threadCount);
        p->num_threads = threadCount.current.Data;

        // Handle count (as a proxy for file descriptors)
        COUNTER_DATA handleCount = {.key = "Handle Count"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &handleCount);
        p->openfds.files = handleCount.current.Data;

        // Page faults
        COUNTER_DATA pageFaults = {.key = "Page Faults/sec"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &pageFaults);
        p->minflt = pageFaults.current.Data;  // Windows doesn't distinguish between minor and major page faults

        // Process uptime
        COUNTER_DATA elapsedTime = {.key = "Elapsed Time"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &elapsedTime);
        p->uptime = elapsedTime.current.Data / 10000000;  // Convert 100-nanosecond units to seconds

        // // Priority
        // COUNTER_DATA priority = {.key = "Priority Base"};
        // perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &priority);
        // p->priority = priority.current.Data;

        // // Pool memory
        // COUNTER_DATA poolPaged = {.key = "Pool Paged Bytes"};
        // COUNTER_DATA poolNonPaged = {.key = "Pool Nonpaged Bytes"};
        // perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &poolPaged);
        // perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &poolNonPaged);
        // p->status_pool_paged = poolPaged.current.Data / 1024;  // Convert to KB
        // p->status_pool_nonpaged = poolNonPaged.current.Data / 1024;  // Convert to KB

        // Windows doesn't provide direct equivalents for these Linux-specific fields:
        // p->status_voluntary_ctxt_switches
        // p->status_nonvoluntary_ctxt_switches
        // We could potentially use "Context Switches/sec" as a total, but it's not split into voluntary/nonvoluntary

        // UID and GID don't have direct equivalents in Windows
        // You might want to use a constant value or implement a mapping to Windows SIDs if needed
    }

    perflibFreePerformanceData();

    return true;
}

#endif
