// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

#if defined(OS_FREEBSD)

usec_t system_current_time_ut;
long global_block_size = 512;

static long get_fs_block_size(void) {
    struct statvfs vfs;
    static long block_size = 0;

    if (block_size == 0) {
        if (statvfs("/", &vfs) == 0) {
            block_size = vfs.f_frsize ? vfs.f_frsize : vfs.f_bsize;
        } else {
            // If statvfs fails, fall back to the typical block size
            block_size = 512;
        }
    }

    return block_size;
}

void apps_os_init_freebsd(void) {
    global_block_size = get_fs_block_size();
}

static inline void get_current_time(void) {
    struct timeval current_time;
    gettimeofday(&current_time, NULL);
    system_current_time_ut = timeval_usec(&current_time);
}

uint64_t apps_os_get_total_memory_freebsd(void) {
    uint64_t ret = 0;

    int mib[2] = {CTL_HW, HW_PHYSMEM};
    size_t size = sizeof(ret);
    if (sysctl(mib, 2, &ret, &size, NULL, 0) == -1) {
        netdata_log_error("Failed to get total memory using sysctl");
        return 0;
    }

    return ret;
}

bool apps_os_read_pid_fds_freebsd(struct pid_stat *p, void *ptr) {
    int mib[4];
    size_t size;
    struct kinfo_file *fds;
    static char *fdsbuf;
    char *bfdsbuf, *efdsbuf;
    char fdsname[FILENAME_MAX + 1];
#define SHM_FORMAT_LEN 31 // format: 21 + size: 10
    char shm_name[FILENAME_MAX - SHM_FORMAT_LEN + 1];

    // we make all pid fds negative, so that
    // we can detect unused file descriptors
    // at the end, to free them
    make_all_pid_fds_negative(p);

    mib[0] = CTL_KERN;
    mib[1] = KERN_PROC;
    mib[2] = KERN_PROC_FILEDESC;
    mib[3] = p->pid;

    if (unlikely(sysctl(mib, 4, NULL, &size, NULL, 0))) {
        netdata_log_error("sysctl error: Can't get file descriptors data size for pid %d", p->pid);
        return false;
    }
    if (likely(size > 0))
        fdsbuf = reallocz(fdsbuf, size);
    if (unlikely(sysctl(mib, 4, fdsbuf, &size, NULL, 0))) {
        netdata_log_error("sysctl error: Can't get file descriptors data for pid %d", p->pid);
        return false;
    }

    bfdsbuf = fdsbuf;
    efdsbuf = fdsbuf + size;
    while (bfdsbuf < efdsbuf) {
        fds = (struct kinfo_file *)(uintptr_t)bfdsbuf;
        if (unlikely(fds->kf_structsize == 0))
            break;

        // do not process file descriptors for current working directory, root directory,
        // jail directory, ktrace vnode, text vnode and controlling terminal
        if (unlikely(fds->kf_fd < 0)) {
            bfdsbuf += fds->kf_structsize;
            continue;
        }

        // get file descriptors array index
        size_t fdid = fds->kf_fd;

        // check if the fds array is small
        if (unlikely(fdid >= p->fds_size)) {
            // it is small, extend it

            uint32_t new_size = fds_new_size(p->fds_size, fdid);

            debug_log("extending fd memory slots for %s from %u to %u",
                      pid_stat_comm(p), p->fds_size, new_size);

            p->fds = reallocz(p->fds, new_size * sizeof(struct pid_fd));

            // and initialize it
            init_pid_fds(p, p->fds_size, new_size - p->fds_size);
            p->fds_size = new_size;
        }

        if (unlikely(p->fds[fdid].fd == 0)) {
            // we don't know this fd, get it

            switch (fds->kf_type) {
                case KF_TYPE_FIFO:
                case KF_TYPE_VNODE:
                    if (unlikely(!fds->kf_path[0])) {
                        sprintf(fdsname, "other: inode: %lu", fds->kf_un.kf_file.kf_file_fileid);
                        break;
                    }
                    sprintf(fdsname, "%s", fds->kf_path);
                    break;
                case KF_TYPE_SOCKET:
                    switch (fds->kf_sock_domain) {
                        case AF_INET:
                        case AF_INET6:
#if __FreeBSD_version < 1400074
                            if (fds->kf_sock_protocol == IPPROTO_TCP)
                                sprintf(fdsname, "socket: %d %lx", fds->kf_sock_protocol, fds->kf_un.kf_sock.kf_sock_inpcb);
                            else
#endif
                                sprintf(fdsname, "socket: %d %lx", fds->kf_sock_protocol, fds->kf_un.kf_sock.kf_sock_pcb);
                            break;
                        case AF_UNIX:
                            /* print address of pcb and connected pcb */
                            sprintf(fdsname, "socket: %lx %lx", fds->kf_un.kf_sock.kf_sock_pcb, fds->kf_un.kf_sock.kf_sock_unpconn);
                            break;
                        default:
                            /* print protocol number and socket address */
#if __FreeBSD_version < 1200031
                            sprintf(fdsname, "socket: other: %d %s %s", fds->kf_sock_protocol, fds->kf_sa_local.__ss_pad1, fds->kf_sa_local.__ss_pad2);
#else
                            sprintf(fdsname, "socket: other: %d %s %s", fds->kf_sock_protocol, fds->kf_un.kf_sock.kf_sa_local.__ss_pad1, fds->kf_un.kf_sock.kf_sa_local.__ss_pad2);
#endif
                    }
                    break;
                case KF_TYPE_PIPE:
                    sprintf(fdsname, "pipe: %lu %lu", fds->kf_un.kf_pipe.kf_pipe_addr, fds->kf_un.kf_pipe.kf_pipe_peer);
                    break;
                case KF_TYPE_PTS:
#if __FreeBSD_version < 1200031
                    sprintf(fdsname, "other: pts: %u", fds->kf_un.kf_pts.kf_pts_dev);
#else
                    sprintf(fdsname, "other: pts: %lu", fds->kf_un.kf_pts.kf_pts_dev);
#endif
                    break;
                case KF_TYPE_SHM:
                    strncpyz(shm_name, fds->kf_path, FILENAME_MAX - SHM_FORMAT_LEN);
                    sprintf(fdsname, "other: shm: %s size: %lu", shm_name, fds->kf_un.kf_file.kf_file_size);
                    break;
                case KF_TYPE_SEM:
                    sprintf(fdsname, "other: sem: %u", fds->kf_un.kf_sem.kf_sem_value);
                    break;
                default:
                    sprintf(fdsname, "other: pid: %d fd: %d", fds->kf_un.kf_proc.kf_pid, fds->kf_fd);
            }

            // if another process already has this, we will get
            // the same id
            p->fds[fdid].fd = file_descriptor_find_or_add(fdsname, 0);
        }

        // else make it positive again, we need it
        // of course, the actual file may have changed

        else
            p->fds[fdid].fd = -p->fds[fdid].fd;

        bfdsbuf += fds->kf_structsize;
    }

    return true;
}

bool apps_os_get_pid_cmdline_freebsd(struct pid_stat *p, char *cmdline, size_t bytes) {
    size_t i, b = bytes - 1;
    int mib[4];

    mib[0] = CTL_KERN;
    mib[1] = KERN_PROC;
    mib[2] = KERN_PROC_ARGS;
    mib[3] = p->pid;
    if (unlikely(sysctl(mib, 4, cmdline, &b, NULL, 0)))
        return false;

    cmdline[b] = '\0';
    for(i = 0; i < b ; i++)
        if(unlikely(!cmdline[i])) cmdline[i] = ' ';

    return true;
}

bool apps_os_read_pid_io_freebsd(struct pid_stat *p, void *ptr) {
    struct kinfo_proc *proc_info = (struct kinfo_proc *)ptr;

    pid_incremental_rate(io, PDF_LREAD,  proc_info->ki_rusage.ru_inblock * global_block_size);
    pid_incremental_rate(io, PDF_LWRITE, proc_info->ki_rusage.ru_oublock * global_block_size);

    return true;
}

bool apps_os_read_pid_limits_freebsd(struct pid_stat *p __maybe_unused, void *ptr __maybe_unused) {
    return false;
}

bool apps_os_read_pid_status_freebsd(struct pid_stat *p, void *ptr) {
    struct kinfo_proc *proc_info = (struct kinfo_proc *)ptr;

    p->uid                  = proc_info->ki_uid;
    p->gid                  = proc_info->ki_groups[0];
    p->values[PDF_VMSIZE]   = proc_info->ki_size;
    p->values[PDF_VMRSS]    = proc_info->ki_rssize * pagesize;
    // TODO: what about shared and swap memory on FreeBSD?
    return true;
}

//bool apps_os_read_global_cpu_utilization_freebsd(void) {
//    static kernel_uint_t utime_raw = 0, stime_raw = 0, ntime_raw = 0;
//    static usec_t collected_usec = 0, last_collected_usec = 0;
//    long cp_time[CPUSTATES];
//
//    if (unlikely(CPUSTATES != 5)) {
//        goto cleanup;
//    } else {
//        static int mib[2] = {0, 0};
//
//        if (unlikely(GETSYSCTL_SIMPLE("kern.cp_time", mib, cp_time))) {
//            goto cleanup;
//        }
//    }
//
//    last_collected_usec = collected_usec;
//    collected_usec = now_monotonic_usec();
//
//    calls_counter++;
//
//    // temporary - it is added global_ntime;
//    kernel_uint_t global_ntime = 0;
//
//    incremental_rate(global_utime, utime_raw, cp_time[0], collected_usec, last_collected_usec, (NSEC_PER_SEC / system_hz));
//    incremental_rate(global_ntime, ntime_raw, cp_time[1], collected_usec, last_collected_usec, (NSEC_PER_SEC / system_hz));
//    incremental_rate(global_stime, stime_raw, cp_time[2], collected_usec, last_collected_usec, (NSEC_PER_SEC / system_hz));
//
//    global_utime += global_ntime;
//
//    if(unlikely(global_iterations_counter == 1)) {
//        global_utime = 0;
//        global_stime = 0;
//        global_gtime = 0;
//    }
//
//    return 1;
//
//cleanup:
//    global_utime = 0;
//    global_stime = 0;
//    global_gtime = 0;
//    return 0;
//}

bool apps_os_read_pid_stat_freebsd(struct pid_stat *p, void *ptr) {
    struct kinfo_proc *proc_info = (struct kinfo_proc *)ptr;
    if (unlikely(proc_info->ki_tdflags & TDF_IDLETD))
        goto cleanup;

    char *comm          = proc_info->ki_comm;
    p->ppid             = proc_info->ki_ppid;

    update_pid_comm(p, comm);

    pid_incremental_rate(stat, PDF_MINFLT,  (kernel_uint_t)proc_info->ki_rusage.ru_minflt);
    pid_incremental_rate(stat, PDF_CMINFLT, (kernel_uint_t)proc_info->ki_rusage_ch.ru_minflt);
    pid_incremental_rate(stat, PDF_MAJFLT,  (kernel_uint_t)proc_info->ki_rusage.ru_majflt);
    pid_incremental_rate(stat, PDF_CMAJFLT, (kernel_uint_t)proc_info->ki_rusage_ch.ru_majflt);
    pid_incremental_cpu(stat, PDF_UTIME,   (kernel_uint_t)proc_info->ki_rusage.ru_utime.tv_sec * NSEC_PER_SEC + proc_info->ki_rusage.ru_utime.tv_usec * NSEC_PER_USEC);
    pid_incremental_cpu(stat, PDF_STIME,   (kernel_uint_t)proc_info->ki_rusage.ru_stime.tv_sec * NSEC_PER_SEC + proc_info->ki_rusage.ru_stime.tv_usec * NSEC_PER_USEC);
    pid_incremental_cpu(stat, PDF_CUTIME,  (kernel_uint_t)proc_info->ki_rusage_ch.ru_utime.tv_sec * NSEC_PER_SEC + proc_info->ki_rusage_ch.ru_utime.tv_usec * NSEC_PER_USEC);
    pid_incremental_cpu(stat, PDF_CSTIME,  (kernel_uint_t)proc_info->ki_rusage_ch.ru_stime.tv_sec * NSEC_PER_SEC + proc_info->ki_rusage_ch.ru_stime.tv_usec * NSEC_PER_USEC);

    p->values[PDF_THREADS] = proc_info->ki_numthreads;

    usec_t started_ut = timeval_usec(&proc_info->ki_start);
    p->values[PDF_UPTIME] = (system_current_time_ut > started_ut) ? (system_current_time_ut - started_ut) / USEC_PER_SEC : 0;

    if(unlikely(debug_enabled))
        debug_log_int("READ PROC/PID/STAT: %s/proc/%d/stat, process: '%s' on target '%s' (dt=%llu) VALUES: utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", cutime=" KERNEL_UINT_FORMAT ", cstime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", cminflt=" KERNEL_UINT_FORMAT ", cmajflt=" KERNEL_UINT_FORMAT ", threads=%d",
                      netdata_configured_host_prefix, p->pid, pid_stat_comm(p), (p->target)?string2str(p->target->name):"UNSET",
                      p->stat_collected_usec - p->last_stat_collected_usec,
                      p->values[PDF_UTIME],
                      p->values[PDF_STIME],
                      p->values[PDF_CUTIME],
                      p->values[PDF_CSTIME],
                      p->values[PDF_MINFLT],
                      p->values[PDF_MAJFLT],
                      p->values[PDF_CMINFLT],
                      p->values[PDF_CMAJFLT],
                      p->values[PDF_THREADS]);

    return true;

cleanup:
    return false;
}

bool apps_os_collect_all_pids_freebsd(void) {
    // Mark all processes as unread before collecting new data
    struct pid_stat *p = NULL;
    int i, procnum;

    static size_t procbase_size = 0;
    static struct kinfo_proc *procbase = NULL;

    size_t new_procbase_size;

    int mib[3] = { CTL_KERN, KERN_PROC, KERN_PROC_PROC };
    if (unlikely(sysctl(mib, 3, NULL, &new_procbase_size, NULL, 0))) {
        netdata_log_error("sysctl error: Can't get processes data size");
        return false;
    }

    // give it some air for processes that may be started
    // during this little time.
    new_procbase_size += 100 * sizeof(struct kinfo_proc);

    // increase the buffer if needed
    if(new_procbase_size > procbase_size) {
        procbase_size = new_procbase_size;
        procbase = reallocz(procbase, procbase_size);
    }

    // sysctl() gets from new_procbase_size the buffer size
    // and also returns to it the amount of data filled in
    new_procbase_size = procbase_size;

    // get the processes from the system
    if (unlikely(sysctl(mib, 3, procbase, &new_procbase_size, NULL, 0))) {
        netdata_log_error("sysctl error: Can't get processes data");
        return false;
    }

    // based on the amount of data filled in
    // calculate the number of processes we got
    procnum = new_procbase_size / sizeof(struct kinfo_proc);

    get_current_time();

    for (i = 0 ; i < procnum ; ++i) {
        pid_t pid = procbase[i].ki_pid;
        if (pid <= 0) continue;
        incrementally_collect_data_for_pid(pid, &procbase[i]);
    }

    return true;
}

#endif
