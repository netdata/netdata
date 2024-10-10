// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

#if defined(OS_MACOS)

usec_t system_current_time_ut;
mach_timebase_info_data_t mach_info;

void apps_os_init_macos(void) {
    mach_timebase_info(&mach_info);
}

uint64_t apps_os_get_total_memory_macos(void) {
    uint64_t ret = 0;
    int mib[2] = {CTL_HW, HW_MEMSIZE};
    size_t size = sizeof(ret);
    if (sysctl(mib, 2, &ret, &size, NULL, 0) == -1) {
        netdata_log_error("Failed to get total memory using sysctl");
        return 0;
    }

    return ret;
}

bool apps_os_read_pid_fds_macos(struct pid_stat *p, void *ptr __maybe_unused) {
    static struct proc_fdinfo *fds = NULL;
    static int fdsCapacity = 0;

    int bufferSize = proc_pidinfo(p->pid, PROC_PIDLISTFDS, 0, NULL, 0);
    if (bufferSize <= 0) {
        netdata_log_error("Failed to get the size of file descriptors for PID %d", p->pid);
        return false;
    }

    // Resize buffer if necessary
    if (bufferSize > fdsCapacity) {
        if(fds)
            freez(fds);

        fds = mallocz(bufferSize);
        fdsCapacity = bufferSize;
    }

    int num_fds = proc_pidinfo(p->pid, PROC_PIDLISTFDS, 0, fds, bufferSize) / PROC_PIDLISTFD_SIZE;
    if (num_fds <= 0) {
        netdata_log_error("Failed to get the file descriptors for PID %d", p->pid);
        return false;
    }

    for (int i = 0; i < num_fds; i++) {
        switch (fds[i].proc_fdtype) {
            case PROX_FDTYPE_VNODE: {
                struct vnode_fdinfowithpath vi;
                if (proc_pidfdinfo(p->pid, fds[i].proc_fd, PROC_PIDFDVNODEPATHINFO, &vi, sizeof(vi)) > 0)
                    p->openfds.files++;
                else
                    p->openfds.other++;

                break;
            }
            case PROX_FDTYPE_SOCKET: {
                p->openfds.sockets++;
                break;
            }
            case PROX_FDTYPE_PIPE: {
                p->openfds.pipes++;
                break;
            }

            default:
                p->openfds.other++;
                break;
        }
    }

    return true;
}

bool apps_os_get_pid_cmdline_macos(struct pid_stat *p, char *cmdline, size_t maxBytes) {
    int mib[3] = {CTL_KERN, KERN_PROCARGS2, p->pid};
    static char *args = NULL;
    static size_t size = 0;

    size_t new_size;
    if (sysctl(mib, 3, NULL, &new_size, NULL, 0) == -1) {
        return false;
    }

    if (new_size > size) {
        if (args)
            freez(args);

        args = (char *)mallocz(new_size);
        size = new_size;
    }

    memset(cmdline, 0, new_size < maxBytes ? new_size : maxBytes);

    size_t used_size = size;
    if (sysctl(mib, 3, args, &used_size, NULL, 0) == -1)
        return false;

    int argc;
    memcpy(&argc, args, sizeof(argc));
    char *ptr = args + sizeof(argc);
    used_size -= sizeof(argc);

    // Skip the executable path
    while (*ptr && used_size > 0) {
        ptr++;
        used_size--;
    }

    // Copy only the arguments to the cmdline buffer, skipping the environment variables
    size_t i = 0, copied_args = 0;
    bool inArg = false;
    for (; used_size > 0 && i < maxBytes - 1 && copied_args < argc; --used_size, ++ptr) {
        if (*ptr == '\0') {
            if (inArg) {
                cmdline[i++] = ' ';  // Replace nulls between arguments with spaces
                inArg = false;
                copied_args++;
            }
        } else {
            cmdline[i++] = *ptr;
            inArg = true;
        }
    }

    if (i > 0 && cmdline[i - 1] == ' ')
        i--;  // Remove the trailing space if present

    cmdline[i] = '\0'; // Null-terminate the string

    return true;
}

bool apps_os_read_pid_io_macos(struct pid_stat *p, void *ptr) {
    struct pid_info *pi = ptr;

    // On MacOS, the proc_pid_rusage provides disk_io_statistics which includes io bytes read and written
    // but does not provide the same level of detail as Linux, like separating logical and physical I/O bytes.
    pid_incremental_rate(io, PDF_LREAD, pi->rusageinfo.ri_diskio_bytesread);
    pid_incremental_rate(io, PDF_LWRITE, pi->rusageinfo.ri_diskio_byteswritten);

    return true;
}

bool apps_os_read_pid_limits_macos(struct pid_stat *p __maybe_unused, void *ptr __maybe_unused) {
    return false;
}

bool apps_os_read_pid_status_macos(struct pid_stat *p, void *ptr) {
    struct pid_info *pi = ptr;

    p->uid = pi->bsdinfo.pbi_uid;
    p->gid = pi->bsdinfo.pbi_gid;
    p->values[PDF_VMSIZE] = pi->taskinfo.pti_virtual_size;
    p->values[PDF_VMRSS] = pi->taskinfo.pti_resident_size;
    // p->values[PDF_VMSWAP] = rusageinfo.ri_swapins + rusageinfo.ri_swapouts; // This is not directly available, consider an alternative representation
    p->values[PDF_VOLCTX] = pi->taskinfo.pti_csw;
    // p->values[PDF_NVOLCTX] = taskinfo.pti_nivcsw;

    return true;
}

static inline void get_current_time(void) {
    struct timeval current_time;
    gettimeofday(&current_time, NULL);
    system_current_time_ut = timeval_usec(&current_time);
}

// bool apps_os_read_global_cpu_utilization_macos(void) {
//     static kernel_uint_t utime_raw = 0, stime_raw = 0, ntime_raw = 0;
//     static usec_t collected_usec = 0, last_collected_usec = 0;
//
//     host_cpu_load_info_data_t cpuinfo;
//     mach_msg_type_number_t count = HOST_CPU_LOAD_INFO_COUNT;
//
//     if (host_statistics(mach_host_self(), HOST_CPU_LOAD_INFO, (host_info_t)&cpuinfo, &count) != KERN_SUCCESS) {
//         // Handle error
//         goto cleanup;
//     }
//
//     last_collected_usec = collected_usec;
//     collected_usec = now_monotonic_usec();
//
//     calls_counter++;
//
//     // Convert ticks to time
//     // Note: MacOS does not separate nice time from user time in the CPU stats, so you might need to adjust this logic
//     kernel_uint_t global_ntime = 0;  // Assuming you want to keep track of nice time separately
//
//     incremental_rate(global_utime, utime_raw, cpuinfo.cpu_ticks[CPU_STATE_USER] + cpuinfo.cpu_ticks[CPU_STATE_NICE], collected_usec, last_collected_usec, CPU_TO_NANOSECONDCORES);
//     incremental_rate(global_ntime, ntime_raw, cpuinfo.cpu_ticks[CPU_STATE_NICE], collected_usec, last_collected_usec, CPU_TO_NANOSECONDCORES);
//     incremental_rate(global_stime, stime_raw, cpuinfo.cpu_ticks[CPU_STATE_SYSTEM], collected_usec, last_collected_usec, CPU_TO_NANOSECONDCORES);
//
//     global_utime += global_ntime;
//
//     if(unlikely(global_iterations_counter == 1)) {
//         global_utime = 0;
//         global_stime = 0;
//         global_gtime = 0;
//     }
//
//     return 1;
//
// cleanup:
//     global_utime = 0;
//     global_stime = 0;
//     global_gtime = 0;
//     return 0;
// }

bool apps_os_read_pid_stat_macos(struct pid_stat *p, void *ptr) {
    struct pid_info *pi = ptr;

    p->ppid = pi->proc.kp_eproc.e_ppid;

    // Update command name and target if changed
    char comm[PROC_PIDPATHINFO_MAXSIZE];
    int ret = proc_name(p->pid, comm, sizeof(comm));
    if (ret <= 0)
        strncpyz(comm, "unknown", sizeof(comm) - 1);

    update_pid_comm(p, comm);

    kernel_uint_t userCPU = (pi->taskinfo.pti_total_user * mach_info.numer) / mach_info.denom;
    kernel_uint_t systemCPU = (pi->taskinfo.pti_total_system * mach_info.numer) / mach_info.denom;

    // Map the values from taskinfo to the pid_stat structure
    pid_incremental_rate(stat, PDF_MINFLT, pi->taskinfo.pti_faults);
    pid_incremental_rate(stat, PDF_MAJFLT, pi->taskinfo.pti_pageins);
    pid_incremental_cpu(stat, PDF_UTIME, userCPU);
    pid_incremental_cpu(stat, PDF_STIME, systemCPU);
    p->values[PDF_THREADS] = pi->taskinfo.pti_threadnum;

    usec_t started_ut = timeval_usec(&pi->proc.kp_proc.p_starttime);
    p->values[PDF_UPTIME] = (system_current_time_ut > started_ut) ? (system_current_time_ut - started_ut) / USEC_PER_SEC : 0;

    // Note: Some values such as guest time, cutime, cstime, etc., are not directly available in MacOS.
    // You might need to approximate or leave them unset depending on your needs.

    if(unlikely(debug_enabled)) {
        debug_log_int("READ PROC/PID/STAT for MacOS: process: '%s' on target '%s' VALUES: utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", threads=%d",
                      pid_stat_comm(p), (p->target) ? string2str(p->target->name) : "UNSET",
                      p->values[PDF_UTIME],
                      p->values[PDF_STIME],
                      p->values[PDF_MINFLT],
                      p->values[PDF_MAJFLT],
                      p->values[PDF_THREADS]);
    }

    // MacOS doesn't have a direct concept of process state like Linux,
    // so updating process state count might need a different approach.

    return true;
}

bool apps_os_collect_all_pids_macos(void) {
    // Mark all processes as unread before collecting new data
    struct pid_stat *p;
    static pid_t *pids = NULL;
    static int allocatedProcessCount = 0;

    // Get the number of processes
    int numberOfProcesses = proc_listpids(PROC_ALL_PIDS, 0, NULL, 0);
    if (numberOfProcesses <= 0) {
        netdata_log_error("Failed to retrieve the process count");
        return false;
    }

    // Allocate or reallocate space to hold all the process IDs if necessary
    if (numberOfProcesses > allocatedProcessCount) {
        // Allocate additional space to avoid frequent reallocations
        allocatedProcessCount = numberOfProcesses + 100;
        pids = reallocz(pids, allocatedProcessCount * sizeof(pid_t));
    }

    // this is required, otherwise the PIDs become totally random
    memset(pids, 0, allocatedProcessCount * sizeof(pid_t));

    // get the list of PIDs
    numberOfProcesses = proc_listpids(PROC_ALL_PIDS, 0, pids, allocatedProcessCount * sizeof(pid_t));
    if (numberOfProcesses <= 0) {
        netdata_log_error("Failed to retrieve the process IDs");
        return false;
    }

    get_current_time();

    // Collect data for each process
    for (int i = 0; i < numberOfProcesses; ++i) {
        pid_t pid = pids[i];
        if (pid <= 0) continue;

        struct pid_info pi = { 0 };

        int mib[4] = {CTL_KERN, KERN_PROC, KERN_PROC_PID, pid};

        size_t procSize = sizeof(pi.proc);
        if(sysctl(mib, 4, &pi.proc, &procSize, NULL, 0) == -1) {
            netdata_log_error("Failed to get proc for PID %d", pid);
            continue;
        }
        if(procSize == 0) // no such process
            continue;

        int st = proc_pidinfo(pid, PROC_PIDTASKINFO, 0, &pi.taskinfo, sizeof(pi.taskinfo));
        if (st <= 0) {
            netdata_log_error("Failed to get task info for PID %d", pid);
            continue;
        }

        st = proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &pi.bsdinfo, sizeof(pi.bsdinfo));
        if (st <= 0) {
            netdata_log_error("Failed to get BSD info for PID %d", pid);
            continue;
        }

        st = proc_pid_rusage(pid, RUSAGE_INFO_V4, (rusage_info_t *)&pi.rusageinfo);
        if (st < 0) {
            netdata_log_error("Failed to get resource usage info for PID %d", pid);
            continue;
        }

        incrementally_collect_data_for_pid(pid, &pi);
    }

    return true;
}

#endif
