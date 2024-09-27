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

struct perflib_data {
    PERF_DATA_BLOCK *pDataBlock;
    PERF_OBJECT_TYPE *pObjectType;
    PERF_INSTANCE_DEFINITION *pi;
    DWORD pid;
};

void apps_os_init(void) {
    PerflibNamesRegistryInitialize();
}

bool apps_os_read_global_cpu_utilization_windows(void) {
    // dummy - not needed
    return false;
}

bool apps_os_read_pid_status_windows(struct pid_stat *p __maybe_unused, void *ptr __maybe_unused) {
    // dummy - not needed
    return false;
}

bool apps_os_read_pid_stat_windows(struct pid_stat *p __maybe_unused, void *ptr __maybe_unused) {
    // dummy - not needed
    return false;
}

bool read_proc_pid_limits_per_os(struct pid_stat *p __maybe_unused, void *ptr __maybe_unused) {
    // dummy - not needed
    return false;
}

bool read_proc_pid_io_per_os(struct pid_stat *p __maybe_unused, void *ptr __maybe_unused) {
    // dummy - not needed
    return false;
}

bool get_cmdline_per_os(struct pid_stat *p __maybe_unused, char *cmdline __maybe_unused, size_t bytes __maybe_unused) {
    // dummy - not needed
    return false;
}

uint64_t apps_os_get_total_memory_windows(void) {
    MEMORYSTATUSEX memStat = { 0 };
    memStat.dwLength = sizeof(memStat);

    if (!GlobalMemoryStatusEx(&memStat)) {
        netdata_log_error("GlobalMemoryStatusEx() failed.");
        return 0;
    }

    return memStat.ullTotalPhys;
}

static inline kernel_uint_t perflib_cpu_utilization(COUNTER_DATA *d) {
    internal_fatal(d->current.CounterType != PERF_100NSEC_TIMER,
                   "Wrong timer type");

    ULONGLONG data1 = d->current.Data;
    ULONGLONG data0 = d->previous.Data;
    LONGLONG time1 = d->current.Time;
    LONGLONG time0 = d->previous.Time;

    /*
     * The Windows documentation provides the formula for percentage:
     *
     *           100 * (data1 - data0) / (time1 - time0)
     *
     * To get a fraction (0.0 to 1.0) instead of a percentage, we
     * simply remove the 100 multiplier:
     *
     *           (data1 - data0) / (time1 - time0)
     *
     * This fraction represents the portion of a single CPU core used
     * over the time period. Multiplying this fraction by NSEC_PER_SEC
     * converts it to nanosecond-cores:
     *
     *           NSEC_PER_SEC * (data1 - data0) / (time1 - time0)
     */

    LONGLONG dt = time1 - time0;
    if(dt > 0)
        return NSEC_PER_SEC * (data1 - data0) / dt;
    else
        return 0;
}

static inline kernel_uint_t perflib_rate(COUNTER_DATA *d) {
    ULONGLONG data1 = d->current.Data;
    ULONGLONG data0 = d->previous.Data;
    LONGLONG time1 = d->current.Time;
    LONGLONG time0 = d->previous.Time;

    LONGLONG dt = (time1 - time0);
    if(dt > 0)
        return (RATES_DETAIL * (data1 - data0)) / dt;
    else
        return 0;
}

static inline kernel_uint_t perflib_value(COUNTER_DATA *d) {
    internal_fatal(d->current.CounterType != PERF_COUNTER_LARGE_RAWCOUNT &&
                   d->current.CounterType != PERF_COUNTER_RAWCOUNT,
                   "Wrong gauge type");

    return d->current.Data;
}

static inline kernel_uint_t perflib_elapsed(COUNTER_DATA *d) {
    ULONGLONG data1 = d->current.Data;
    LONGLONG time1 = d->current.Time;
    LONGLONG freq1 = d->current.Frequency;

    internal_fatal(d->current.CounterType != PERF_ELAPSED_TIME || !freq1,
                   "Wrong gauge type");

    return (time1 - data1) / freq1;
}

bool apps_os_collect_all_pids_windows(void) {
    calls_counter++;

    struct perflib_data d = { 0 };
    d.pDataBlock = perflibGetPerformanceData(RegistryFindIDByName("Process"));
    if(!d.pDataBlock) return false;

    d.pObjectType = perflibFindObjectTypeByName(d.pDataBlock, "Process");
    if(!d.pObjectType) {
        perflibFreePerformanceData();
        return false;
    }

    // we need these outside the loop to avoid searching by name all the time
    // (our perflib library caches the id inside the COUNTER_DATA).
    COUNTER_DATA processId = {.key = "ID Process"};

    d.pi = NULL;
    for(LONG i = 0; i < d.pObjectType->NumInstances; i++) {
        d.pi = perflibForEachInstance(d.pDataBlock, d.pObjectType, d.pi);
        if (!d.pi) break;

        perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &processId);
        d.pid = (DWORD) processId.current.Data;
        if (d.pid <= 0) continue; // 0 = Idle (this takes all the spare resources)

        // Get or create pid_stat structure
        struct pid_stat *p = get_or_allocate_pid_entry((pid_t) d.pid);

        if (unlikely(!p->perflib[PDF_UTIME].key)) {
            // a new pid

            static __thread char name[MAX_PATH];
            if (getInstanceName(d.pDataBlock, d.pObjectType, d.pi, name, sizeof(name))) {
                // remove the PID from the end of the name
                char pid[UINT64_MAX_LENGTH + 1]; // +1 for the underscore
                pid[0] = '_';
                print_uint64(&pid[1], p->pid);
                size_t pid_len = strlen(pid);
                size_t name_len = strlen(name);
                if(pid_len < name_len) {
                    char *compare = &name[name_len - pid_len];
                    if(strcmp(pid, compare) == 0)
                        *compare = '\0';
                }
            }
            else
                strncpyz(name, "unknown", sizeof(name) - 1);

            update_pid_comm(p, name);

            COUNTER_DATA ppid = {.key = "Creating Process ID"};
            perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &ppid);
            p->ppid = (pid_t) ppid.current.Data;

            p->perflib[PDF_UTIME].key = "% User Time";
            p->perflib[PDF_STIME].key = "% Privileged Time";
            p->perflib[PDF_VMSIZE].key = "Virtual Bytes";
            p->perflib[PDF_VMRSS].key = "Working Set";
            p->perflib[PDF_VMSWAP].key = "Page File Bytes";
            p->perflib[PDF_LREAD].key = "IO Read Bytes/sec";
            p->perflib[PDF_LWRITE].key = "IO Write Bytes/sec";
            p->perflib[PDF_OREAD].key = "IO Read Operations/sec";
            p->perflib[PDF_OWRITE].key = "IO Write Operations/sec";
            p->perflib[PDF_THREADS].key = "Thread Count";
            p->perflib[PDF_HANDLES].key = "Handle Count";
            p->perflib[PDF_MINFLT].key = "Page Faults/sec";
            p->perflib[PDF_UPTIME].key = "Elapsed Time";
        }

        pid_collection_started(p);

        // get all data from perflib
        size_t ok = 0, failed = 0, invalid = 0;
        for (PID_FIELD f = 0; f < PDF_MAX; f++) {
            if (p->perflib[f].key) {
                if (!perflibGetInstanceCounter(d.pDataBlock, d.pObjectType, d.pi, &p->perflib[f])) {
                    failed++;
                    nd_log(NDLS_COLLECTORS, NDLP_ERR,
                           "Cannot find field '%s' in processes data", p->perflib[f].key);
                } else
                    ok++;
            } else
                invalid++;
        }

        if(failed) {
            pid_collection_failed(p);
            continue;
        }

        // CPU time
        p->values[PDF_UTIME] = perflib_cpu_utilization(&p->perflib[PDF_UTIME]);
        p->values[PDF_STIME] = perflib_cpu_utilization(&p->perflib[PDF_STIME]);

        // Memory
        p->values[PDF_VMRSS] = perflib_value(&p->perflib[PDF_VMRSS]);
        p->values[PDF_VMSIZE] = perflib_value(&p->perflib[PDF_VMSIZE]);
        p->values[PDF_VMSWAP] = perflib_value(&p->perflib[PDF_VMSWAP]);

        // I/O
        p->values[PDF_LREAD] = perflib_rate(&p->perflib[PDF_LREAD]);
        p->values[PDF_LWRITE] = perflib_rate(&p->perflib[PDF_LWRITE]);
        p->values[PDF_OREAD] = perflib_rate(&p->perflib[PDF_OREAD]);
        p->values[PDF_OWRITE] = perflib_rate(&p->perflib[PDF_OWRITE]);

        // Threads
        p->values[PDF_THREADS] = perflib_value(&p->perflib[PDF_THREADS]);

        // Handle count
        p->values[PDF_HANDLES] = perflib_value(&p->perflib[PDF_HANDLES]);

        // Page faults
        // Windows doesn't distinguish between minor and major page faults
        p->values[PDF_MINFLT] = perflib_rate(&p->perflib[PDF_MINFLT]);

        // Process uptime
        // Convert 100-nanosecond units to seconds
        p->values[PDF_UPTIME] = perflib_elapsed(&p->perflib[PDF_UPTIME]);

        pid_collection_completed(p);

//        if(p->perflib[PDF_UTIME].current.Data != p->perflib[PDF_UTIME].previous.Data &&
//           p->perflib[PDF_UTIME].current.Data && p->perflib[PDF_UTIME].previous.Data &&
//           p->pid == 61812) {
//            const char *cmd = string2str(p->comm);
//            uint64_t cpu_divisor = NSEC_PER_SEC / 100ULL;
//            uint64_t cpus = os_get_system_cpus();
//            double u = (double)p->values[PDF_UTIME] / cpu_divisor;
//            double s = (double)p->values[PDF_STIME] / cpu_divisor;
//            int x = 0;
//            x++;
//        }
    }

    perflibFreePerformanceData();

    return true;
}

#endif
