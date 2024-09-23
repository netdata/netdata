// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

// ----------------------------------------------------------------------------

#define MAX_PROC_PID_LIMITS 8192
#define PROC_PID_LIMITS_MAX_OPEN_FILES_KEY "\nMax open files "

static inline kernel_uint_t get_proc_pid_limits_limit(char *buf, const char *key, size_t key_len, kernel_uint_t def) {
    char *line = strstr(buf, key);
    if(!line)
        return def;

    char *v = &line[key_len];
    while(isspace((uint8_t)*v)) v++;

    if(strcmp(v, "unlimited") == 0)
        return 0;

    return str2ull(v, NULL);
}

#if defined(OS_FREEBSD) || defined(OS_MACOS) || defined(OS_WINDOWS)
int read_proc_pid_limits_per_os(struct pid_stat *p, void *ptr __maybe_unused) {
    return false;
}
#endif

#if defined(OS_LINUX)
static inline bool read_proc_pid_limits_per_os(struct pid_stat *p, void *ptr __maybe_unused) {
    static char proc_pid_limits_buffer[MAX_PROC_PID_LIMITS + 1];
    bool ret = false;
    bool read_limits = false;

    errno_clear();
    proc_pid_limits_buffer[0] = '\0';

    kernel_uint_t all_fds = pid_openfds_sum(p);
    if(all_fds < p->limits.max_open_files / 2 && p->io_collected_usec > p->last_limits_collected_usec && p->io_collected_usec - p->last_limits_collected_usec <= 60 * USEC_PER_SEC) {
        // too frequent, we want to collect limits once per minute
        ret = true;
        goto cleanup;
    }

    if(unlikely(!p->limits_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/limits", netdata_configured_host_prefix, p->pid);
        p->limits_filename = strdupz(filename);
    }

    int fd = open(p->limits_filename, procfile_open_flags, 0666);
    if(unlikely(fd == -1)) goto cleanup;

    ssize_t bytes = read(fd, proc_pid_limits_buffer, MAX_PROC_PID_LIMITS);
    close(fd);

    if(bytes <= 0)
        goto cleanup;

    // make it '\0' terminated
    if(bytes < MAX_PROC_PID_LIMITS)
        proc_pid_limits_buffer[bytes] = '\0';
    else
        proc_pid_limits_buffer[MAX_PROC_PID_LIMITS - 1] = '\0';

    p->limits.max_open_files = get_proc_pid_limits_limit(proc_pid_limits_buffer, PROC_PID_LIMITS_MAX_OPEN_FILES_KEY, sizeof(PROC_PID_LIMITS_MAX_OPEN_FILES_KEY) - 1, 0);
    if(p->limits.max_open_files == 1) {
        // it seems a bug in the kernel or something similar
        // it sets max open files to 1 but the number of files
        // the process has open are more than 1...
        // https://github.com/netdata/netdata/issues/15443
        p->limits.max_open_files = 0;
        ret = true;
        goto cleanup;
    }

    p->last_limits_collected_usec = p->io_collected_usec;
    read_limits = true;

    ret = true;

cleanup:
    if(p->limits.max_open_files)
        p->openfds_limits_percent = (NETDATA_DOUBLE)all_fds * 100.0 / (NETDATA_DOUBLE)p->limits.max_open_files;
    else
        p->openfds_limits_percent = 0.0;

    if(p->openfds_limits_percent > 100.0) {
        if(!(p->log_thrown & PID_LOG_LIMITS_DETAIL)) {
            char *line;

            if(!read_limits) {
                proc_pid_limits_buffer[0] = '\0';
                line = "NOT READ";
            }
            else {
                line = strstr(proc_pid_limits_buffer, PROC_PID_LIMITS_MAX_OPEN_FILES_KEY);
                if (line) {
                    line++; // skip the initial newline

                    char *end = strchr(line, '\n');
                    if (end)
                        *end = '\0';
                }
            }

            netdata_log_info(
                "FDS_LIMITS: PID %d (%s) is using "
                "%0.2f %% of its fds limits, "
                "open fds = %"PRIu64 "("
                "files = %"PRIu64 ", "
                "pipes = %"PRIu64 ", "
                "sockets = %"PRIu64", "
                "inotifies = %"PRIu64", "
                "eventfds = %"PRIu64", "
                "timerfds = %"PRIu64", "
                "signalfds = %"PRIu64", "
                "eventpolls = %"PRIu64" "
                "other = %"PRIu64" "
                "), open fds limit = %"PRIu64", "
                "%s, "
                "original line [%s]",
                p->pid, pid_stat_comm(p), p->openfds_limits_percent, all_fds,
                p->openfds.files,
                p->openfds.pipes,
                p->openfds.sockets,
                p->openfds.inotifies,
                p->openfds.eventfds,
                p->openfds.timerfds,
                p->openfds.signalfds,
                p->openfds.eventpolls,
                p->openfds.other,
                p->limits.max_open_files,
                read_limits ? "and we have read the limits AFTER counting the fds"
                              : "but we have read the limits BEFORE counting the fds",
                line);

            p->log_thrown |= PID_LOG_LIMITS_DETAIL;
        }
    }
    else
        p->log_thrown &= ~PID_LOG_LIMITS_DETAIL;

    return ret;
}
#endif // !__FreeBSD__ !__APPLE__

int read_proc_pid_limits(struct pid_stat *p, void *ptr) {
    return read_proc_pid_limits_per_os(p, ptr) ? 1 : 0;
}
