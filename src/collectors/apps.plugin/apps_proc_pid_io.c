// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

static inline void clear_pid_io(struct pid_stat *p) {
#if (PROCESSES_HAVE_LOGICAL_IO == 1)
    p->io_logical_bytes_read        = 0;
    p->io_logical_bytes_written     = 0;
#endif
#if (PROCESSES_HAVE_PHYSICAL_IO == 1)
    p->io_storage_bytes_read        = 0;
    p->io_storage_bytes_written     = 0;
    p->io_cancelled_write_bytes     = 0;
#endif
#if (PROCESSES_HAVE_IO_CALLS == 1)
    p->io_read_calls                = 0;
    p->io_write_calls               = 0;
#endif
}

#if defined(OS_FREEBSD)
static inline bool read_proc_pid_io_per_os(struct pid_stat *p, void *ptr) {
    struct kinfo_proc *proc_info = (struct kinfo_proc *)ptr;

    pid_incremental_rate(io, p->io_storage_bytes_read,       proc_info->ki_rusage.ru_inblock);
    pid_incremental_rate(io, p->io_storage_bytes_written,    proc_info->ki_rusage.ru_oublock);

    p->io_logical_bytes_read = 0;
    p->io_logical_bytes_written = 0;
    p->io_read_calls = 0;
    p->io_write_calls = 0;
    p->io_cancelled_write_bytes = 0;

    return true;
}
#endif

#if defined(OS_MACOS)
static inline bool read_proc_pid_io_per_os(struct pid_stat *p, void *ptr) {
    struct pid_info *pi = ptr;

    // On MacOS, the proc_pid_rusage provides disk_io_statistics which includes io bytes read and written
    // but does not provide the same level of detail as Linux, like separating logical and physical I/O bytes.
    pid_incremental_rate(io, p->io_storage_bytes_read, pi->rusageinfo.ri_diskio_bytesread);
    pid_incremental_rate(io, p->io_storage_bytes_written, pi->rusageinfo.ri_diskio_byteswritten);

    p->io_logical_bytes_read = 0;
    p->io_logical_bytes_written = 0;
    p->io_read_calls = 0;
    p->io_write_calls = 0;
    p->io_cancelled_write_bytes = 0;

    return true;
}
#endif

#if defined(OS_WINDOWS)
static inline bool read_proc_pid_io_per_os(struct pid_stat *p, void *ptr) {
    // TODO: get I/O throughput per process from perflib
    return false;
}
#endif

#if defined(OS_LINUX)
static inline int read_proc_pid_io_per_os(struct pid_stat *p, void *ptr __maybe_unused) {
    static procfile *ff = NULL;

    if(unlikely(!p->io_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/io", netdata_configured_host_prefix, p->pid);
        p->io_filename = strdupz(filename);
    }

    // open the file
    ff = procfile_reopen(ff, p->io_filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(unlikely(!ff)) goto cleanup;

    ff = procfile_readall(ff);
    if(unlikely(!ff)) goto cleanup;

    pid_incremental_rate(io, p->io_logical_bytes_read,       str2kernel_uint_t(procfile_lineword(ff, 0,  1)));
    pid_incremental_rate(io, p->io_logical_bytes_written,    str2kernel_uint_t(procfile_lineword(ff, 1,  1)));
    pid_incremental_rate(io, p->io_read_calls,               str2kernel_uint_t(procfile_lineword(ff, 2,  1)));
    pid_incremental_rate(io, p->io_write_calls,              str2kernel_uint_t(procfile_lineword(ff, 3,  1)));
    pid_incremental_rate(io, p->io_storage_bytes_read,       str2kernel_uint_t(procfile_lineword(ff, 4,  1)));
    pid_incremental_rate(io, p->io_storage_bytes_written,    str2kernel_uint_t(procfile_lineword(ff, 5,  1)));
    pid_incremental_rate(io, p->io_cancelled_write_bytes,    str2kernel_uint_t(procfile_lineword(ff, 6,  1)));

    return true;

cleanup:
    clear_pid_io(p);
    return false;
}
#endif // !__FreeBSD__ !__APPLE__

int read_proc_pid_io(struct pid_stat *p, void *ptr) {
    p->last_io_collected_usec = p->io_collected_usec;
    p->io_collected_usec = now_monotonic_usec();
    calls_counter++;

    bool ret = read_proc_pid_io_per_os(p, ptr);

    if(unlikely(global_iterations_counter == 1))
        clear_pid_io(p);

    return ret ? 1 : 0;
}
