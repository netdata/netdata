// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"
/*
{
  "SystemName": "WIN11",
  "NumObjectTypes": 1,
  "LittleEndian": 1,
  "Version": 1,
  "Revision": 1,
  "DefaultObject": 238,
  "PerfFreq": 10000000,
  "PerfTime": 9242655165203,
  "PerfTime100nSec": 133716612800215149,
  "SystemTime": {
    "Year": 2024,
    "Month": 9,
    "DayOfWeek": 2,
    "Day": 24,
    "Hour": 14,
    "Minute": 21,
    "Second": 20,
    "Milliseconds": 21
  },
  "Objects": [
    {
      "NameId": 230,
      "Name": "Process",
      "HelpId": 231,
      "Help": "The Process performance object consists of counters that monitor running application program and system processes.  All the threads in a process share the same address space and have access to the same data.",
      "NumInstances": 274,
      "NumCounters": 28,
      "PerfTime": 133716612800215149,
      "PerfFreq": 10000000,
      "CodePage": 0,
      "DefaultCounter": 0,
      "DetailLevel": "Novice (100)",
      "Instances": [
        {
          "Instance": "Idle",
          "UniqueID": -1,
          "Labels": [
            {
              "key": "Process",
              "value": "Idle"
            }
          ],
          "Counters": [
            {
              "Counter": "% Processor Time",
              "Value": {
                "data": 106881107812500,
                "time": 133716612800215149,
                "type": 542180608,
                "multi": 0,
                "frequency": 0
              },
              "Help": "% Processor Time is the percentage of elapsed time that all of process threads used the processor to execution instructions. An instruction is the basic unit of execution in a computer, a thread is the object that executes instructions, and a process is the object created when a program is run. Code executed to handle some hardware interrupts and trap conditions are included in this count.",
              "Type": "PERF_100NSEC_TIMER",
              "Algorithm": "100 * (data1 - data0) / (time1 - time0)",
              "Description": "64-bit Timer in 100 nsec units. Display delta divided by delta time. Display suffix: \"%\""
            },
            {
              "Counter": "% User Time",
              "Value": {
                "data": 0,
                "time": 133716612800215149,
                "type": 542180608,
                "multi": 0,
                "frequency": 0
              },
              "Help": "% User Time is the percentage of elapsed time that the process threads spent executing code in user mode. Applications, environment subsystems, and integral subsystems execute in user mode. Code executing in user mode cannot damage the integrity of the Windows executive, kernel, and device drivers. Unlike some early operating systems, Windows uses process boundaries for subsystem protection in addition to the traditional protection of user and privileged modes. Some work done by Windows on behalf of the application might appear in other subsystem processes in addition to the privileged time in the process.",
              "Type": "PERF_100NSEC_TIMER",
              "Algorithm": "100 * (data1 - data0) / (time1 - time0)",
              "Description": "64-bit Timer in 100 nsec units. Display delta divided by delta time. Display suffix: \"%\""
            },
            {
              "Counter": "% Privileged Time",
              "Value": {
                "data": 106881107812500,
                "time": 133716612800215149,
                "type": 542180608,
                "multi": 0,
                "frequency": 0
              },
              "Help": "% Privileged Time is the percentage of elapsed time that the process threads spent executing code in privileged mode. When a Windows system service is called, the service will often run in privileged mode to gain access to system-private data. Such data is protected from access by threads executing in user mode. Calls to the system can be explicit or implicit, such as page faults or interrupts. Unlike some early operating systems, Windows uses process boundaries for subsystem protection in addition to the traditional protection of user and privileged modes. Some work done by Windows on behalf of the application might appear in other subsystem processes in addition to the privileged time in the process.",
              "Type": "PERF_100NSEC_TIMER",
              "Algorithm": "100 * (data1 - data0) / (time1 - time0)",
              "Description": "64-bit Timer in 100 nsec units. Display delta divided by delta time. Display suffix: \"%\""
            },
            {
              "Counter": "Virtual Bytes Peak",
              "Value": {
                "data": 8192,
                "time": 0,
                "type": 65792,
                "multi": 0,
                "frequency": 0
              },
              "Help": "Virtual Bytes Peak is the maximum size, in bytes, of virtual address space the process has used at any one time. Use of virtual address space does not necessarily imply corresponding use of either disk or main memory pages. However, virtual space is finite, and the process might limit its ability to load libraries.",
              "Type": "PERF_COUNTER_LARGE_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            },
            {
              "Counter": "Virtual Bytes",
              "Value": {
                "data": 8192,
                "time": 0,
                "type": 65792,
                "multi": 0,
                "frequency": 0
              },
              "Help": "Virtual Bytes is the current size, in bytes, of the virtual address space the process is using. Use of virtual address space does not necessarily imply corresponding use of either disk or main memory pages. Virtual space is finite, and the process can limit its ability to load libraries.",
              "Type": "PERF_COUNTER_LARGE_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            },
            {
              "Counter": "Page Faults/sec",
              "Value": {
                "data": 9,
                "time": 9242655165203,
                "type": 272696320,
                "multi": 0,
                "frequency": 10000000
              },
              "Help": "Page Faults/sec is the rate at which page faults by the threads executing in this process are occurring.  A page fault occurs when a thread refers to a virtual memory page that is not in its working set in main memory. This may not cause the page to be fetched from disk if it is on the standby list and hence already in main memory, or if it is in use by another process with whom the page is shared.",
              "Type": "PERF_COUNTER_COUNTER",
              "Algorithm": "(data1 - data0) / ((time1 - time0) / frequency)",
              "Description": "32-bit Counter. Divide delta by delta time. Display suffix: \"/sec\""
            },
            {
              "Counter": "Working Set Peak",
              "Value": {
                "data": 8192,
                "time": 0,
                "type": 65792,
                "multi": 0,
                "frequency": 0
              },
              "Help": "Working Set Peak is the maximum size, in bytes, of the Working Set of this process at any point in time. The Working Set is the set of memory pages touched recently by the threads in the process. If free memory in the computer is above a threshold, pages are left in the Working Set of a process even if they are not in use. When free memory falls below a threshold, pages are trimmed from Working Sets. If they are needed they will then be soft-faulted back into the Working Set before they leave main memory.",
              "Type": "PERF_COUNTER_LARGE_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            },
            {
              "Counter": "Working Set",
              "Value": {
                "data": 8192,
                "time": 0,
                "type": 65792,
                "multi": 0,
                "frequency": 0
              },
              "Help": "Working Set is the current size, in bytes, of the Working Set of this process. The Working Set is the set of memory pages touched recently by the threads in the process. If free memory in the computer is above a threshold, pages are left in the Working Set of a process even if they are not in use.  When free memory falls below a threshold, pages are trimmed from Working Sets. If they are needed they will then be soft-faulted back into the Working Set before leaving main memory.",
              "Type": "PERF_COUNTER_LARGE_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            },
            {
              "Counter": "Page File Bytes Peak",
              "Value": {
                "data": 61440,
                "time": 0,
                "type": 65792,
                "multi": 0,
                "frequency": 0
              },
              "Help": "Page File Bytes Peak is the maximum amount of virtual memory, in bytes, that this process has reserved for use in the paging file(s). Paging files are used to store pages of memory used by the process that are not contained in other files.  Paging files are shared by all processes, and the lack of space in paging files can prevent other processes from allocating memory. If there is no paging file, this counter reflects the maximum amount of virtual memory that the process has reserved for use in physical memory.",
              "Type": "PERF_COUNTER_LARGE_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            },
            {
              "Counter": "Page File Bytes",
              "Value": {
                "data": 61440,
                "time": 0,
                "type": 65792,
                "multi": 0,
                "frequency": 0
              },
              "Help": "Page File Bytes is the current amount of virtual memory, in bytes, that this process has reserved for use in the paging file(s). Paging files are used to store pages of memory used by the process that are not contained in other files. Paging files are shared by all processes, and the lack of space in paging files can prevent other processes from allocating memory. If there is no paging file, this counter reflects the current amount of virtual memory that the process has reserved for use in physical memory.",
              "Type": "PERF_COUNTER_LARGE_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            },
            {
              "Counter": "Private Bytes",
              "Value": {
                "data": 61440,
                "time": 0,
                "type": 65792,
                "multi": 0,
                "frequency": 0
              },
              "Help": "Private Bytes is the current size, in bytes, of memory that this process has allocated that cannot be shared with other processes.",
              "Type": "PERF_COUNTER_LARGE_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            },
            {
              "Counter": "Thread Count",
              "Value": {
                "data": 24,
                "time": 0,
                "type": 65536,
                "multi": 0,
                "frequency": 0
              },
              "Help": "The number of threads currently active in this process. An instruction is the basic unit of execution in a processor, and a thread is the object that executes instructions. Every running process has at least one thread.",
              "Type": "PERF_COUNTER_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            },
            {
              "Counter": "Priority Base",
              "Value": {
                "data": 0,
                "time": 0,
                "type": 65536,
                "multi": 0,
                "frequency": 0
              },
              "Help": "The current base priority of this process. Threads within a process can raise and lower their own base priority relative to the process' base priority.",
              "Type": "PERF_COUNTER_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            },
            {
              "Counter": "Elapsed Time",
              "Value": {
                "data": 133707369666486855,
                "time": 133716612800215149,
                "type": 807666944,
                "multi": 0,
                "frequency": 10000000
              },
              "Help": "The total elapsed time, in seconds, that this process has been running.",
              "Type": "PERF_ELAPSED_TIME",
              "Algorithm": "(time0 - data0) / frequency0",
              "Description": "The data collected in this counter is actually the start time of the item being measured. For display, this data is subtracted from the sample time to yield the elapsed time as the difference between the two. In the definition below, the PerfTime field of the Object contains the sample time as indicated by the PERF_OBJECT_TIMER bit and the difference is scaled by the PerfFreq of the Object to convert the time units into seconds."
            },
            {
              "Counter": "ID Process",
              "Value": {
                "data": 0,
                "time": 0,
                "type": 65536,
                "multi": 0,
                "frequency": 0
              },
              "Help": "ID Process is the unique identifier of this process. ID Process numbers are reused, so they only identify a process for the lifetime of that process.",
              "Type": "PERF_COUNTER_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            },
            {
              "Counter": "Creating Process ID",
              "Value": {
                "data": 0,
                "time": 0,
                "type": 65536,
                "multi": 0,
                "frequency": 0
              },
              "Help": "The Creating Process ID value is the Process ID of the process that created the process. The creating process may have terminated, so this value may no longer identify a running process.",
              "Type": "PERF_COUNTER_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            },
            {
              "Counter": "Pool Paged Bytes",
              "Value": {
                "data": 0,
                "time": 0,
                "type": 65536,
                "multi": 0,
                "frequency": 0
              },
              "Help": "Pool Paged Bytes is the size, in bytes, of the paged pool, an area of the system virtual memory that is used for objects that can be written to disk when they are not being used.  Memory\\\\Pool Paged Bytes is calculated differently than Process\\\\Pool Paged Bytes, so it might not equal Process(_Total)\\\\Pool Paged Bytes. This counter displays the last observed value only; it is not an average.",
              "Type": "PERF_COUNTER_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            },
            {
              "Counter": "Pool Nonpaged Bytes",
              "Value": {
                "data": 272,
                "time": 0,
                "type": 65536,
                "multi": 0,
                "frequency": 0
              },
              "Help": "Pool Nonpaged Bytes is the size, in bytes, of the nonpaged pool, an area of the system virtual memory that is used for objects that cannot be written to disk, but must remain in physical memory as long as they are allocated.  Memory\\\\Pool Nonpaged Bytes is calculated differently than Process\\\\Pool Nonpaged Bytes, so it might not equal Process(_Total)\\\\Pool Nonpaged Bytes.  This counter displays the last observed value only; it is not an average.",
              "Type": "PERF_COUNTER_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            },
            {
              "Counter": "Handle Count",
              "Value": {
                "data": 0,
                "time": 0,
                "type": 65536,
                "multi": 0,
                "frequency": 0
              },
              "Help": "The total number of handles currently open by this process. This number is equal to the sum of the handles currently open by each thread in this process.",
              "Type": "PERF_COUNTER_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            },
            {
              "Counter": "IO Read Operations/sec",
              "Value": {
                "data": 0,
                "time": 9242655165203,
                "type": 272696576,
                "multi": 0,
                "frequency": 10000000
              },
              "Help": "The rate at which the process is issuing read I/O operations. This counter counts all I/O activity generated by the process to include file, network and device I/Os.",
              "Type": "PERF_COUNTER_BULK_COUNT",
              "Algorithm": "(data1 - data0) / ((time1 - time0) / frequency)",
              "Description": "64-bit Counter.  Divide delta by delta time. Display Suffix: \"/sec\""
            },
            {
              "Counter": "IO Write Operations/sec",
              "Value": {
                "data": 0,
                "time": 9242655165203,
                "type": 272696576,
                "multi": 0,
                "frequency": 10000000
              },
              "Help": "The rate at which the process is issuing write I/O operations. This counter counts all I/O activity generated by the process to include file, network and device I/Os.",
              "Type": "PERF_COUNTER_BULK_COUNT",
              "Algorithm": "(data1 - data0) / ((time1 - time0) / frequency)",
              "Description": "64-bit Counter.  Divide delta by delta time. Display Suffix: \"/sec\""
            },
            {
              "Counter": "IO Data Operations/sec",
              "Value": {
                "data": 0,
                "time": 9242655165203,
                "type": 272696576,
                "multi": 0,
                "frequency": 10000000
              },
              "Help": "The rate at which the process is issuing read and write I/O operations. This counter counts all I/O activity generated by the process to include file, network and device I/Os.",
              "Type": "PERF_COUNTER_BULK_COUNT",
              "Algorithm": "(data1 - data0) / ((time1 - time0) / frequency)",
              "Description": "64-bit Counter.  Divide delta by delta time. Display Suffix: \"/sec\""
            },
            {
              "Counter": "IO Other Operations/sec",
              "Value": {
                "data": 0,
                "time": 9242655165203,
                "type": 272696576,
                "multi": 0,
                "frequency": 10000000
              },
              "Help": "The rate at which the process is issuing I/O operations that are neither read nor write operations (for example, a control function). This counter counts all I/O activity generated by the process to include file, network and device I/Os.",
              "Type": "PERF_COUNTER_BULK_COUNT",
              "Algorithm": "(data1 - data0) / ((time1 - time0) / frequency)",
              "Description": "64-bit Counter.  Divide delta by delta time. Display Suffix: \"/sec\""
            },
            {
              "Counter": "IO Read Bytes/sec",
              "Value": {
                "data": 0,
                "time": 9242655165203,
                "type": 272696576,
                "multi": 0,
                "frequency": 10000000
              },
              "Help": "The rate at which the process is reading bytes from I/O operations. This counter counts all I/O activity generated by the process to include file, network and device I/Os.",
              "Type": "PERF_COUNTER_BULK_COUNT",
              "Algorithm": "(data1 - data0) / ((time1 - time0) / frequency)",
              "Description": "64-bit Counter.  Divide delta by delta time. Display Suffix: \"/sec\""
            },
            {
              "Counter": "IO Write Bytes/sec",
              "Value": {
                "data": 0,
                "time": 9242655165203,
                "type": 272696576,
                "multi": 0,
                "frequency": 10000000
              },
              "Help": "The rate at which the process is writing bytes to I/O operations. This counter counts all I/O activity generated by the process to include file, network and device I/Os.",
              "Type": "PERF_COUNTER_BULK_COUNT",
              "Algorithm": "(data1 - data0) / ((time1 - time0) / frequency)",
              "Description": "64-bit Counter.  Divide delta by delta time. Display Suffix: \"/sec\""
            },
            {
              "Counter": "IO Data Bytes/sec",
              "Value": {
                "data": 0,
                "time": 9242655165203,
                "type": 272696576,
                "multi": 0,
                "frequency": 10000000
              },
              "Help": "The rate at which the process is reading and writing bytes in I/O operations. This counter counts all I/O activity generated by the process to include file, network and device I/Os.",
              "Type": "PERF_COUNTER_BULK_COUNT",
              "Algorithm": "(data1 - data0) / ((time1 - time0) / frequency)",
              "Description": "64-bit Counter.  Divide delta by delta time. Display Suffix: \"/sec\""
            },
            {
              "Counter": "IO Other Bytes/sec",
              "Value": {
                "data": 0,
                "time": 9242655165203,
                "type": 272696576,
                "multi": 0,
                "frequency": 10000000
              },
              "Help": "The rate at which the process is issuing bytes to I/O operations that do not involve data such as control operations. This counter counts all I/O activity generated by the process to include file, network and device I/Os.",
              "Type": "PERF_COUNTER_BULK_COUNT",
              "Algorithm": "(data1 - data0) / ((time1 - time0) / frequency)",
              "Description": "64-bit Counter.  Divide delta by delta time. Display Suffix: \"/sec\""
            },
            {
              "Counter": "Working Set - Private",
              "Value": {
                "data": 8192,
                "time": 0,
                "type": 65792,
                "multi": 0,
                "frequency": 0
              },
              "Help": "Working Set - Private displays the size of the working set, in bytes, that is use for this process only and not shared nor sharable by other processes.",
              "Type": "PERF_COUNTER_LARGE_RAWCOUNT",
              "Algorithm": "data0",
              "Description": "A counter which should not be time averaged on display (such as an error counter on a serial line). Display as is. No Display Suffix."
            }
          ]
        },
 */


#if defined(OS_WINDOWS)

uint64_t apps_os_time_factor(void) {
    time_factor = 10000000ULL / RATES_DETAIL; // Windows uses 100-nanosecond intervals
    PerflibNamesRegistryInitialize();
}

bool apps_os_read_global_cpu_utilization(void) {
    return false;
}

bool read_proc_pid_status_per_os(struct pid_stat *p __maybe_unused, void *ptr) {
    struct perflib_data *d = ptr;

    // TODO: get these statistics from perflib
    return false;
}

bool read_proc_pid_stat_per_os(struct pid_stat *p __maybe_unused, void *ptr) {
    struct perflib_data *d = ptr;

    // TODO: get these statistics from perflib

    return false;
}

bool read_proc_pid_limits_per_os(struct pid_stat *p __maybe_unused, void *ptr) {
    struct perflib_data *d = ptr;

    // TODO: get process limits from perflib

    return false;
}

bool read_proc_pid_io_per_os(struct pid_stat *p, void *ptr) {
    struct perflib_data *d = ptr;

    // TODO: get I/O throughput per process from perflib

    return false;
}

bool get_cmdline_per_os(struct pid_stat *p, char *cmdline, size_t bytes) {
    // TODO: get the command line from perflib, if available
    return false;
}

bool read_pid_file_descriptors_per_os(struct pid_stat *p, void *ptr) {
    struct perflib_data *d = ptr;

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
        p->values[PDF_UTIME] = userTime.current.Data;
        p->values[PDF_STIME] = kernelTime.current.Data;

        // Memory
        COUNTER_DATA workingSet = {.key = "Working Set"};
        COUNTER_DATA privateBytes = {.key = "Private Bytes"};
        COUNTER_DATA virtualBytes = {.key = "Virtual Bytes"};
        COUNTER_DATA pageFileBytes = {.key = "Page File Bytes"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &workingSet);
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &privateBytes);
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &virtualBytes);
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &pageFileBytes);
        p->values[PDF_VMRSS] = workingSet.current.Data / 1024;  // Convert to KB
        p->values[PDF_VMSIZE] = virtualBytes.current.Data / 1024;  // Convert to KB
        p->values[PDF_VMSWAP] = pageFileBytes.current.Data / 1024;  // Page File Bytes in KB

        // I/O
        COUNTER_DATA ioReadBytes = {.key = "IO Read Bytes/sec"};
        COUNTER_DATA ioWriteBytes = {.key = "IO Write Bytes/sec"};
        COUNTER_DATA ioReadOps = {.key = "IO Read Operations/sec"};
        COUNTER_DATA ioWriteOps = {.key = "IO Write Operations/sec"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &ioReadBytes);
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &ioWriteBytes);
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &ioReadOps);
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &ioWriteOps);
        p->values[PDF_LREAD] = ioReadBytes.current.Data;
        p->values[PDF_LWRITE] = ioWriteBytes.current.Data;
        p->values[PDF_CREAD] = ioReadOps.current.Data;
        p->values[PDF_CWRITE] = ioWriteOps.current.Data;

        // Threads
        COUNTER_DATA threadCount = {.key = "Thread Count"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &threadCount);
        p->values[PDF_THREADS] = threadCount.current.Data;

        // Handle count (as a proxy for file descriptors)
        COUNTER_DATA handleCount = {.key = "Handle Count"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &handleCount);
        p->openfds.files = handleCount.current.Data;

        // Page faults
        COUNTER_DATA pageFaults = {.key = "Page Faults/sec"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &pageFaults);
        p->values[PDF_MINFLT] = pageFaults.current.Data;  // Windows doesn't distinguish between minor and major page faults

        // Process uptime
        COUNTER_DATA elapsedTime = {.key = "Elapsed Time"};
        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &elapsedTime);
        p->values[PDF_UPTIME] = elapsedTime.current.Data / 10000000;  // Convert 100-nanosecond units to seconds

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
        // p->values[PDF_VOLCTX]
        // p->values[PDF_NVOLCTX]
        // We could potentially use "Context Switches/sec" as a total, but it's not split into voluntary/nonvoluntary

        // UID and GID don't have direct equivalents in Windows
        // You might want to use a constant value or implement a mapping to Windows SIDs if needed
    }

    perflibFreePerformanceData();

    return true;
}

#endif
