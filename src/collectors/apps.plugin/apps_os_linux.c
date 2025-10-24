// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"
#include <limits.h>
#include <unistd.h>

#if defined(OS_LINUX)

#define MAX_PROC_PID_LIMITS 8192
#define PROC_PID_LIMITS_MAX_OPEN_FILES_KEY "\nMax open files "

int max_fds_cache_seconds = 60;
kernel_uint_t system_uptime_secs;

void apps_os_init_linux(void) {
    ;
}

// --------------------------------------------------------------------------------------------------------------------
// /proc/pid/fd

struct arl_callback_ptr {
    struct pid_stat *p;
    procfile *ff;
    size_t line;
};

#if (PROCESSES_HAVE_SMAPS_ROLLUP == 1)

struct arl_callback_smaps_ptr {
    struct pid_stat *p;
    procfile *ff;
    size_t line;
};

static procfile *smaps_rollup_ff = NULL;
static struct arl_callback_smaps_ptr smaps_rollup_ctx;
static bool smaps_rollup_checked = false;
static bool smaps_rollup_available = false;
static bool smaps_rollup_warned = false;

struct smaps_candidate {
    struct pid_stat *p;
    kernel_uint_t delta;
};

struct smaps_age_candidate {
    struct pid_stat *p;
    size_t age;
    kernel_uint_t vmshared;
};

static int compare_smaps_candidate_desc(const void *a, const void *b) {
    const struct smaps_candidate *A = (const struct smaps_candidate *)a;
    const struct smaps_candidate *B = (const struct smaps_candidate *)b;
    if(A->delta < B->delta) return 1;
    if(A->delta > B->delta) return -1;
    return 0;
}

static int compare_smaps_age_desc(const void *a, const void *b) {
    const struct smaps_age_candidate *A = (const struct smaps_age_candidate *)a;
    const struct smaps_age_candidate *B = (const struct smaps_age_candidate *)b;
    if(A->age < B->age) return 1;
    if(A->age > B->age) return -1;
    if(A->vmshared < B->vmshared) return 1;
    if(A->vmshared > B->vmshared) return -1;
    return 0;
}

static inline void pid_update_estimated_memory(struct pid_stat *p) {
    kernel_uint_t vmrss = p->values[PDF_VMRSS];
#if (PROCESSES_HAVE_SMAPS_ROLLUP == 1)
    p->values[PDF_PSS] = p->pss_bytes;
#endif
    if(vmrss == 0) {
        p->values[PDF_MEM_ESTIMATED] = 0;
        return;
    }

    NETDATA_DOUBLE ratio = p->pss_total_ratio;
    if(unlikely(ratio < 0.0)) ratio = 0.0;
    if(unlikely(ratio > 1.0)) ratio = 1.0;

    NETDATA_DOUBLE scaled = (NETDATA_DOUBLE)vmrss * ratio;
    if(unlikely(scaled > (NETDATA_DOUBLE)UINT64_MAX))
        p->values[PDF_MEM_ESTIMATED] = vmrss;
    else
        p->values[PDF_MEM_ESTIMATED] = (kernel_uint_t)(scaled + 0.5);
}

static inline kernel_uint_t smaps_value_to_bytes(const char *value) {
    return str2kernel_uint_t(value) * 1024ULL;
}

static void arl_callback_smaps_pss(const char *name __maybe_unused, uint32_t hash __maybe_unused, const char *value __maybe_unused, void *dst) {
    struct arl_callback_smaps_ptr *ctx = (struct arl_callback_smaps_ptr *)dst;
    if(unlikely(procfile_linewords(ctx->ff, ctx->line) < 2))
        return;

    ctx->p->values[PDF_PSS] = smaps_value_to_bytes(procfile_lineword(ctx->ff, ctx->line, 1));
}

bool apps_os_have_smaps_rollup_linux(void) {
    if(likely(smaps_rollup_checked))
        return smaps_rollup_available;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/self/smaps_rollup", netdata_configured_host_prefix);

    if(access(filename, R_OK) == 0) {
        smaps_rollup_available = true;
    }
    else {
        if(!smaps_rollup_warned) {
            netdata_log_info("apps.plugin: /proc/*/smaps_rollup is not available on this kernel. PSS metrics will be disabled.");
            smaps_rollup_warned = true;
        }
        smaps_rollup_available = false;
    }

    smaps_rollup_checked = true;
    return smaps_rollup_available;
}

bool apps_os_read_pid_smaps_rollup_linux(struct pid_stat *p, void *ptr __maybe_unused) {
    if(unlikely(!apps_os_have_smaps_rollup_linux()))
        return false;

    if(unlikely(!p->smaps_rollup_arl)) {
        p->smaps_rollup_arl = arl_create("/proc/pid/smaps_rollup", NULL, 60);
        arl_expect_custom(p->smaps_rollup_arl, "Pss", arl_callback_smaps_pss, &smaps_rollup_ctx);
    }

    if(unlikely(!p->smaps_rollup_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/smaps_rollup", netdata_configured_host_prefix, p->pid);
        p->smaps_rollup_filename = strdupz(filename);
    }

    smaps_rollup_ff = procfile_reopen(smaps_rollup_ff, p->smaps_rollup_filename, (!smaps_rollup_ff) ? " \t:" : NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(unlikely(!smaps_rollup_ff)) {
        if(errno == EINVAL)
            errno = ENOENT;
        return false;
    }

    smaps_rollup_ff = procfile_readall(smaps_rollup_ff);
    if(unlikely(!smaps_rollup_ff)) {
        if(errno == EINVAL)
            errno = ENOENT;
        return false;
    }

    calls_counter++;

    smaps_rollup_ctx.p = p;
    smaps_rollup_ctx.ff = smaps_rollup_ff;

    size_t lines = procfile_lines(smaps_rollup_ff);
    arl_begin(p->smaps_rollup_arl);

    for(size_t l = 0; l < lines; l++) {
        smaps_rollup_ctx.line = l;
        if(unlikely(arl_check(p->smaps_rollup_arl,
                              procfile_lineword(smaps_rollup_ff, l, 0),
                              procfile_lineword(smaps_rollup_ff, l, 1))))
            break;
    }

    kernel_uint_t vmrss = p->values[PDF_VMRSS];
    kernel_uint_t pss = p->values[PDF_PSS];
    NETDATA_DOUBLE ratio = 1.0;
    if(vmrss > 0)
        ratio = (NETDATA_DOUBLE)pss / (NETDATA_DOUBLE)vmrss;

    if(ratio < 0.0) ratio = 0.0;
    if(ratio > 1.0) ratio = 1.0;

    p->pss_total_ratio = ratio;
    p->pss_bytes = pss;
    pid_update_estimated_memory(p);
    p->vmshared_delta = 0;
    p->last_pss_iteration = global_iterations_counter;

    return true;
}

#define PSS_REFRESH_MIN 1

static void apps_handle_smaps_updates(void) {
    if(pss_refresh_period <= 0)
        return;

    if(unlikely(!apps_os_have_smaps_rollup()))
        return;

    struct pid_stat *p;
    size_t total_pids = 0;

    for(p = root_of_pids(); p ; p = p->next)
        total_pids++;

    if(unlikely(total_pids == 0))
        return;

    int divisor = pss_refresh_period / update_every;
    if(divisor < 1)
        divisor = 1;

    size_t budget = total_pids / (size_t)divisor;
    if(budget < PSS_REFRESH_MIN)
        budget = PSS_REFRESH_MIN;

    struct smaps_candidate *delta_all = mallocz(sizeof(*delta_all) * total_pids);
    size_t delta_count = 0;
    struct smaps_age_candidate *age_all = mallocz(sizeof(*age_all) * total_pids);
    size_t age_count = 0;

    for(p = root_of_pids(); p ; p = p->next) {
        kernel_uint_t vmshared = p->values[PDF_VMSHARED];

        if(vmshared > 0 && p->vmshared_delta > 0) {
            delta_all[delta_count].p = p;
            delta_all[delta_count].delta = p->vmshared_delta;
            delta_count++;
        }

        if(vmshared == 0)
            continue;

        size_t age;
        if(p->last_pss_iteration == 0)
            age = SIZE_MAX;
        else if(global_iterations_counter >= p->last_pss_iteration)
            age = global_iterations_counter - p->last_pss_iteration;
        else
            age = SIZE_MAX;

        age_all[age_count].p = p;
        age_all[age_count].age = age;
        age_all[age_count].vmshared = vmshared;
        age_count++;
    }

    if(delta_count > 1)
        qsort(delta_all, delta_count, sizeof(*delta_all), compare_smaps_candidate_desc);
    if(age_count > 1)
        qsort(age_all, age_count, sizeof(*age_all), compare_smaps_age_desc);

    size_t delta_to_refresh = delta_count;
    if(delta_to_refresh > budget)
        delta_to_refresh = budget;
    for(size_t i = 0; i < delta_to_refresh; i++) {
        struct pid_stat *pid_entry = delta_all[i].p;
        if(!pid_entry)
            continue;
        OS_FUNCTION(apps_os_read_pid_smaps_rollup)(pid_entry, NULL);
    }

    size_t age_to_refresh = budget;
    size_t refreshed = 0;
    for(size_t i = 0; i < age_count && refreshed < age_to_refresh; i++) {
        struct pid_stat *pid_entry = age_all[i].p;
        if(!pid_entry)
            continue;
        if(pid_entry->last_pss_iteration == global_iterations_counter)
            continue;
        OS_FUNCTION(apps_os_read_pid_smaps_rollup)(pid_entry, NULL);
        refreshed++;
    }

    freez(delta_all);
    freez(age_all);
}

#endif // PROCESSES_HAVE_SMAPS_ROLLUP

bool apps_os_read_pid_fds_linux(struct pid_stat *p, void *ptr __maybe_unused) {
    if(unlikely(!p->fds_dirname)) {
        char dirname[FILENAME_MAX+1];
        snprintfz(dirname, FILENAME_MAX, "%s/proc/%d/fd", netdata_configured_host_prefix, p->pid);
        p->fds_dirname = strdupz(dirname);
    }

    DIR *fds = opendir(p->fds_dirname);
    if(unlikely(!fds)) return false;

    struct dirent *de;
    char linkname[FILENAME_MAX + 1];

    // we make all pid fds negative, so that
    // we can detect unused file descriptors
    // at the end, to free them
    make_all_pid_fds_negative(p);

    while((de = readdir(fds))) {
        // we need only files with numeric names

        if(unlikely(de->d_name[0] < '0' || de->d_name[0] > '9'))
            continue;

        // get its number
        int fdid = (int) str2l(de->d_name);
        if(unlikely(fdid < 0)) continue;

        // check if the fds array is small
        if(unlikely((size_t)fdid >= p->fds_size)) {
            // it is small, extend it

            uint32_t new_size = fds_new_size(p->fds_size, fdid);

            debug_log("extending fd memory slots for %s from %u to %u",
                      pid_stat_comm(p), p->fds_size, new_size);

            p->fds = reallocz(p->fds, new_size * sizeof(struct pid_fd));

            // and initialize it
            init_pid_fds(p, p->fds_size, new_size - p->fds_size);
            p->fds_size = new_size;
        }

        if(unlikely(p->fds[fdid].fd < 0 && de->d_ino != p->fds[fdid].inode)) {
            // inodes do not match, clear the previous entry
            inodes_changed_counter++;
            file_descriptor_not_used(-p->fds[fdid].fd);
            clear_pid_fd(&p->fds[fdid]);
        }

        if(p->fds[fdid].fd < 0 && p->fds[fdid].cache_iterations_counter > 0) {
            p->fds[fdid].fd = -p->fds[fdid].fd;
            p->fds[fdid].cache_iterations_counter--;
            continue;
        }

        if(unlikely(!p->fds[fdid].filename)) {
            filenames_allocated_counter++;
            char fdname[FILENAME_MAX + 1];
            snprintfz(fdname, FILENAME_MAX, "%s/proc/%d/fd/%s", netdata_configured_host_prefix, p->pid, de->d_name);
            p->fds[fdid].filename = strdupz(fdname);
        }

        file_counter++;
        ssize_t l = readlink(p->fds[fdid].filename, linkname, FILENAME_MAX);
        if(unlikely(l == -1)) {
            // cannot read the link

            if(debug_enabled)
                netdata_log_error("Cannot read link %s", p->fds[fdid].filename);

            if(unlikely(p->fds[fdid].fd < 0)) {
                file_descriptor_not_used(-p->fds[fdid].fd);
                clear_pid_fd(&p->fds[fdid]);
            }

            continue;
        }
        else
            linkname[l] = '\0';

        uint32_t link_hash = simple_hash(linkname);

        if(unlikely(p->fds[fdid].fd < 0 && p->fds[fdid].link_hash != link_hash)) {
            // the link changed
            links_changed_counter++;
            file_descriptor_not_used(-p->fds[fdid].fd);
            clear_pid_fd(&p->fds[fdid]);
        }

        if(unlikely(p->fds[fdid].fd == 0)) {
            // we don't know this fd, get it

            // if another process already has this, we will get
            // the same id
            p->fds[fdid].fd = (int)file_descriptor_find_or_add(linkname, link_hash);
            p->fds[fdid].inode = de->d_ino;
            p->fds[fdid].link_hash = link_hash;
        }
        else {
            // else make it positive again, we need it
            p->fds[fdid].fd = -p->fds[fdid].fd;
        }

        // caching control
        // without this we read all the files on every iteration
        if(max_fds_cache_seconds > 0) {
            size_t spread = ((size_t)max_fds_cache_seconds > 10) ? 10 : (size_t)max_fds_cache_seconds;

            // cache it for a few iterations
            size_t max = ((size_t) max_fds_cache_seconds + (fdid % spread)) / (size_t) update_every;
            p->fds[fdid].cache_iterations_reset++;

            if(unlikely(p->fds[fdid].cache_iterations_reset % spread == (size_t) fdid % spread))
                p->fds[fdid].cache_iterations_reset++;

            if(unlikely((fdid <= 2 && p->fds[fdid].cache_iterations_reset > 5) ||
                         p->fds[fdid].cache_iterations_reset > max)) {
                // for stdin, stdout, stderr (fdid <= 2) we have checked a few times, or if it goes above the max, goto max
                p->fds[fdid].cache_iterations_reset = max;
            }

            p->fds[fdid].cache_iterations_counter = p->fds[fdid].cache_iterations_reset;
        }
    }

    closedir(fds);

    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// /proc/meminfo

uint64_t apps_os_get_total_memory_linux(void) {
    uint64_t ret = 0;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/meminfo", netdata_configured_host_prefix);

    procfile *ff = procfile_open(filename, ": \t", PROCFILE_FLAG_DEFAULT);
    if(!ff)
        return ret;

    ff = procfile_readall(ff);
    if(!ff)
        return ret;

    size_t line, lines = procfile_lines(ff);

    for(line = 0; line < lines ;line++) {
        size_t words = procfile_linewords(ff, line);
        if(words == 3 && strcmp(procfile_lineword(ff, line, 0), "MemTotal") == 0 && strcmp(procfile_lineword(ff, line, 2), "kB") == 0) {
            ret = str2ull(procfile_lineword(ff, line, 1), NULL) * 1024;
            break;
        }
    }

    procfile_close(ff);

    return ret;
}

// --------------------------------------------------------------------------------------------------------------------
// /proc/pid/cmdline

bool apps_os_get_pid_cmdline_linux(struct pid_stat *p, char *cmdline, size_t bytes) {
    if(unlikely(!p->cmdline_filename)) {
        char filename[FILENAME_MAX];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/cmdline", netdata_configured_host_prefix, p->pid);
        p->cmdline_filename = strdupz(filename);
    }

    int fd = open(p->cmdline_filename, procfile_open_flags, 0666);
    if(unlikely(fd == -1))
        return false;

    ssize_t i, b = read(fd, cmdline, bytes - 1);
    close(fd);

    if(unlikely(b < 0))
        return false;

    cmdline[b] = '\0';
    for(i = 0; i < b ; i++)
        if(unlikely(!cmdline[i])) cmdline[i] = ' ';

    // remove trailing spaces
    while(b > 0 && cmdline[b - 1] == ' ')
        cmdline[--b] = '\0';

    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// /proc/pid/io

bool apps_os_read_pid_io_linux(struct pid_stat *p, void *ptr __maybe_unused) {
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

    pid_incremental_rate(io, PDF_LREAD,     str2kernel_uint_t(procfile_lineword(ff, 0,  1)));
    pid_incremental_rate(io, PDF_LWRITE,    str2kernel_uint_t(procfile_lineword(ff, 1,  1)));
    pid_incremental_rate(io, PDF_OREAD,     str2kernel_uint_t(procfile_lineword(ff, 2,  1)));
    pid_incremental_rate(io, PDF_OWRITE,    str2kernel_uint_t(procfile_lineword(ff, 3,  1)));
    pid_incremental_rate(io, PDF_PREAD,     str2kernel_uint_t(procfile_lineword(ff, 4,  1)));
    pid_incremental_rate(io, PDF_PWRITE,    str2kernel_uint_t(procfile_lineword(ff, 5,  1)));

    return true;

cleanup:
    return false;
}

// --------------------------------------------------------------------------------------------------------------------
// /proc/pid/limits

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

bool apps_os_read_pid_limits_linux(struct pid_stat *p, void *ptr __maybe_unused) {
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

// --------------------------------------------------------------------------------------------------------------------
// /proc/pid/status

void arl_callback_status_uid(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 5)) return;

    //const char *real_uid = procfile_lineword(aptr->ff, aptr->line, 1);
    const char *effective_uid = procfile_lineword(aptr->ff, aptr->line, 2);
    //const char *saved_uid = procfile_lineword(aptr->ff, aptr->line, 3);
    //const char *filesystem_uid = procfile_lineword(aptr->ff, aptr->line, 4);

    if(likely(effective_uid && *effective_uid))
        aptr->p->uid = (uid_t)str2l(effective_uid);
}

void arl_callback_status_gid(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 5)) return;

    //const char *real_gid = procfile_lineword(aptr->ff, aptr->line, 1);
    const char *effective_gid = procfile_lineword(aptr->ff, aptr->line, 2);
    //const char *saved_gid = procfile_lineword(aptr->ff, aptr->line, 3);
    //const char *filesystem_gid = procfile_lineword(aptr->ff, aptr->line, 4);

    if(likely(effective_gid && *effective_gid))
        aptr->p->gid = (uid_t)str2l(effective_gid);
}

void arl_callback_status_vmsize(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 3)) return;

    aptr->p->values[PDF_VMSIZE] = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1)) * 1024;
}

void arl_callback_status_vmswap(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 3)) return;

    aptr->p->values[PDF_VMSWAP] = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1)) * 1024;
}

void arl_callback_status_vmrss(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 3)) return;

    aptr->p->values[PDF_VMRSS] = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1)) * 1024;
}

void arl_callback_status_rssfile(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 3)) return;

    aptr->p->values[PDF_RSSFILE] = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1)) * 1024;
}

void arl_callback_status_rssshmem(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 3)) return;

    struct pid_stat *p = aptr->p;
#if (PROCESSES_HAVE_SMAPS_ROLLUP == 1)
    kernel_uint_t old_shared = p->values[PDF_VMSHARED];
#endif

    p->values[PDF_RSSSHMEM] = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1)) * 1024;
    p->values[PDF_VMSHARED] = p->values[PDF_RSSFILE] + p->values[PDF_RSSSHMEM];

#if (PROCESSES_HAVE_SMAPS_ROLLUP == 1)
    if(old_shared > p->values[PDF_VMSHARED])
        p->vmshared_delta += old_shared - p->values[PDF_VMSHARED];
    else
        p->vmshared_delta += p->values[PDF_VMSHARED] - old_shared;

    p->values[PDF_PSS] = p->pss_bytes;

    pid_update_estimated_memory(p);
#endif
}

void arl_callback_status_voluntary_ctxt_switches(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 2)) return;

    struct pid_stat *p = aptr->p;
    pid_incremental_rate(stat, PDF_VOLCTX, str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1)));
}

void arl_callback_status_nonvoluntary_ctxt_switches(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 2)) return;

    struct pid_stat *p = aptr->p;
    pid_incremental_rate(stat, PDF_NVOLCTX, str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1)));
}

bool apps_os_read_pid_status_linux(struct pid_stat *p, void *ptr __maybe_unused) {
    static struct arl_callback_ptr arl_ptr;
    static procfile *ff = NULL;

    if(unlikely(!p->status_arl)) {
        p->status_arl = arl_create("/proc/pid/status", NULL, 60);
        arl_expect_custom(p->status_arl, "Uid", arl_callback_status_uid, &arl_ptr);
        arl_expect_custom(p->status_arl, "Gid", arl_callback_status_gid, &arl_ptr);
        arl_expect_custom(p->status_arl, "VmSize", arl_callback_status_vmsize, &arl_ptr);
        arl_expect_custom(p->status_arl, "VmRSS", arl_callback_status_vmrss, &arl_ptr);
        arl_expect_custom(p->status_arl, "RssFile", arl_callback_status_rssfile, &arl_ptr);
        arl_expect_custom(p->status_arl, "RssShmem", arl_callback_status_rssshmem, &arl_ptr);
        arl_expect_custom(p->status_arl, "VmSwap", arl_callback_status_vmswap, &arl_ptr);
        arl_expect_custom(p->status_arl, "voluntary_ctxt_switches", arl_callback_status_voluntary_ctxt_switches, &arl_ptr);
        arl_expect_custom(p->status_arl, "nonvoluntary_ctxt_switches", arl_callback_status_nonvoluntary_ctxt_switches, &arl_ptr);
    }

    if(unlikely(!p->status_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/status", netdata_configured_host_prefix, p->pid);
        p->status_filename = strdupz(filename);
    }

    ff = procfile_reopen(ff, p->status_filename, (!ff)?" \t:,-()/":NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(unlikely(!ff)) return false;

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return false;

    calls_counter++;

    // let ARL use this pid
    arl_ptr.p = p;
    arl_ptr.ff = ff;

    size_t lines = procfile_lines(ff), l;
    arl_begin(p->status_arl);

    for(l = 0; l < lines ;l++) {
        // debug_log("CHECK: line %zu of %zu, key '%s' = '%s'", l, lines, procfile_lineword(ff, l, 0), procfile_lineword(ff, l, 1));
        arl_ptr.line = l;
        if(unlikely(arl_check(p->status_arl,
                               procfile_lineword(ff, l, 0),
                               procfile_lineword(ff, l, 1)))) break;
    }

    p->values[PDF_VMSHARED] = p->values[PDF_RSSFILE] + p->values[PDF_RSSSHMEM];
    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// global CPU utilization

bool apps_os_read_global_cpu_utilization_linux(void) {
    static char filename[FILENAME_MAX + 1] = "";
    static procfile *ff = NULL;
    static kernel_uint_t utime_raw = 0, stime_raw = 0, gtime_raw = 0, gntime_raw = 0, ntime_raw = 0;
    static usec_t collected_usec = 0, last_collected_usec = 0;

    if(unlikely(!ff)) {
        snprintfz(filename, FILENAME_MAX, "%s/proc/stat", netdata_configured_host_prefix);
        ff = procfile_open(filename, " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) goto cleanup;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) goto cleanup;

    last_collected_usec = collected_usec;
    collected_usec = now_monotonic_usec();

    calls_counter++;

    // temporary - it is added global_ntime;
    kernel_uint_t global_ntime = 0;

    incremental_rate(global_utime, utime_raw, str2kernel_uint_t(procfile_lineword(ff, 0,  1)), collected_usec, last_collected_usec, CPU_TO_NANOSECONDCORES);
    incremental_rate(global_ntime, ntime_raw, str2kernel_uint_t(procfile_lineword(ff, 0,  2)), collected_usec, last_collected_usec, CPU_TO_NANOSECONDCORES);
    incremental_rate(global_stime, stime_raw, str2kernel_uint_t(procfile_lineword(ff, 0,  3)), collected_usec, last_collected_usec, CPU_TO_NANOSECONDCORES);
    incremental_rate(global_gtime, gtime_raw, str2kernel_uint_t(procfile_lineword(ff, 0, 10)), collected_usec, last_collected_usec, CPU_TO_NANOSECONDCORES);

    global_utime += global_ntime;

    if(enable_guest_charts) {
        // temporary - it is added global_ntime;
        kernel_uint_t global_gntime = 0;

        // guest nice time, on guest time
        incremental_rate(global_gntime, gntime_raw, str2kernel_uint_t(procfile_lineword(ff, 0, 11)), collected_usec, last_collected_usec, 1);

        global_gtime += global_gntime;

        // remove guest time from user time
        global_utime -= (global_utime > global_gtime) ? global_gtime : global_utime;
    }

    if(unlikely(global_iterations_counter == 1)) {
        global_utime = 0;
        global_stime = 0;
        global_gtime = 0;
    }

    return true;

cleanup:
    global_utime = 0;
    global_stime = 0;
    global_gtime = 0;
    return false;
}

// --------------------------------------------------------------------------------------------------------------------
// /proc/pid/stat

static inline void update_proc_state_count(char proc_stt) {
    switch (proc_stt) {
        case 'S':
            proc_state_count[PROC_STATUS_SLEEPING] += 1;
            break;
        case 'R':
            proc_state_count[PROC_STATUS_RUNNING] += 1;
            break;
        case 'D':
            proc_state_count[PROC_STATUS_SLEEPING_D] += 1;
            break;
        case 'Z':
            proc_state_count[PROC_STATUS_ZOMBIE] += 1;
            break;
        case 'T':
            proc_state_count[PROC_STATUS_STOPPED] += 1;
            break;
        default:
            break;
    }
}

bool apps_os_read_pid_stat_linux(struct pid_stat *p, void *ptr __maybe_unused) {
    static procfile *ff = NULL;

    if(unlikely(!p->stat_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/stat", netdata_configured_host_prefix, p->pid);
        p->stat_filename = strdupz(filename);
    }

    bool set_quotes = (!ff) ? true : false;

    ff = procfile_reopen(ff, p->stat_filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(unlikely(!ff)) goto cleanup;

    // if(set_quotes) procfile_set_quotes(ff, "()");
    if(unlikely(set_quotes))
        procfile_set_open_close(ff, "(", ")");

    ff = procfile_readall(ff);
    if(unlikely(!ff)) goto cleanup;

    // p->pid           = str2pid_t(procfile_lineword(ff, 0, 0));
    char *comm          = procfile_lineword(ff, 0, 1);
    p->state            = *(procfile_lineword(ff, 0, 2));
    p->ppid             = (int32_t)str2pid_t(procfile_lineword(ff, 0, 3));
    // p->pgrp          = (int32_t)str2pid_t(procfile_lineword(ff, 0, 4));
    // p->session       = (int32_t)str2pid_t(procfile_lineword(ff, 0, 5));
    // p->tty_nr        = (int32_t)str2pid_t(procfile_lineword(ff, 0, 6));
    // p->tpgid         = (int32_t)str2pid_t(procfile_lineword(ff, 0, 7));
    // p->flags         = str2uint64_t(procfile_lineword(ff, 0, 8));

    update_pid_comm(p, comm);

    pid_incremental_rate(stat, PDF_MINFLT,  str2kernel_uint_t(procfile_lineword(ff, 0,  9)));
    pid_incremental_rate(stat, PDF_CMINFLT, str2kernel_uint_t(procfile_lineword(ff, 0, 10)));
    pid_incremental_rate(stat, PDF_MAJFLT,  str2kernel_uint_t(procfile_lineword(ff, 0, 11)));
    pid_incremental_rate(stat, PDF_CMAJFLT, str2kernel_uint_t(procfile_lineword(ff, 0, 12)));
    pid_incremental_cpu(stat, PDF_UTIME,   str2kernel_uint_t(procfile_lineword(ff, 0, 13)));
    pid_incremental_cpu(stat, PDF_STIME,   str2kernel_uint_t(procfile_lineword(ff, 0, 14)));
    pid_incremental_cpu(stat, PDF_CUTIME,  str2kernel_uint_t(procfile_lineword(ff, 0, 15)));
    pid_incremental_cpu(stat, PDF_CSTIME,  str2kernel_uint_t(procfile_lineword(ff, 0, 16)));
    // p->priority      = str2kernel_uint_t(procfile_lineword(ff, 0, 17));
    // p->nice          = str2kernel_uint_t(procfile_lineword(ff, 0, 18));
    p->values[PDF_THREADS] = (int32_t) str2uint32_t(procfile_lineword(ff, 0, 19), NULL);
    // p->itrealvalue   = str2kernel_uint_t(procfile_lineword(ff, 0, 20));
    kernel_uint_t collected_starttime = str2kernel_uint_t(procfile_lineword(ff, 0, 21)) / system_hz;
    p->values[PDF_UPTIME] = (system_uptime_secs > collected_starttime)?(system_uptime_secs - collected_starttime):0;
    // p->vsize         = str2kernel_uint_t(procfile_lineword(ff, 0, 22));
    // p->rss           = str2kernel_uint_t(procfile_lineword(ff, 0, 23));
    // p->rsslim        = str2kernel_uint_t(procfile_lineword(ff, 0, 24));
    // p->starcode      = str2kernel_uint_t(procfile_lineword(ff, 0, 25));
    // p->endcode       = str2kernel_uint_t(procfile_lineword(ff, 0, 26));
    // p->startstack    = str2kernel_uint_t(procfile_lineword(ff, 0, 27));
    // p->kstkesp       = str2kernel_uint_t(procfile_lineword(ff, 0, 28));
    // p->kstkeip       = str2kernel_uint_t(procfile_lineword(ff, 0, 29));
    // p->signal        = str2kernel_uint_t(procfile_lineword(ff, 0, 30));
    // p->blocked       = str2kernel_uint_t(procfile_lineword(ff, 0, 31));
    // p->sigignore     = str2kernel_uint_t(procfile_lineword(ff, 0, 32));
    // p->sigcatch      = str2kernel_uint_t(procfile_lineword(ff, 0, 33));
    // p->wchan         = str2kernel_uint_t(procfile_lineword(ff, 0, 34));
    // p->nswap         = str2kernel_uint_t(procfile_lineword(ff, 0, 35));
    // p->cnswap        = str2kernel_uint_t(procfile_lineword(ff, 0, 36));
    // p->exit_signal   = str2kernel_uint_t(procfile_lineword(ff, 0, 37));
    // p->processor     = str2kernel_uint_t(procfile_lineword(ff, 0, 38));
    // p->rt_priority   = str2kernel_uint_t(procfile_lineword(ff, 0, 39));
    // p->policy        = str2kernel_uint_t(procfile_lineword(ff, 0, 40));
    // p->delayacct_blkio_ticks = str2kernel_uint_t(procfile_lineword(ff, 0, 41));

    if(enable_guest_charts) {
        pid_incremental_cpu(stat, PDF_GTIME,  str2kernel_uint_t(procfile_lineword(ff, 0, 42)));
        pid_incremental_cpu(stat, PDF_CGTIME, str2kernel_uint_t(procfile_lineword(ff, 0, 43)));

        if (show_guest_time || p->values[PDF_GTIME] || p->values[PDF_CGTIME]) {
            p->values[PDF_UTIME] -= (p->values[PDF_UTIME] >= p->values[PDF_GTIME]) ? p->values[PDF_GTIME] : p->values[PDF_UTIME];
            p->values[PDF_CUTIME] -= (p->values[PDF_CUTIME] >= p->values[PDF_CGTIME]) ? p->values[PDF_CGTIME] : p->values[PDF_CUTIME];
            show_guest_time = true;
        }
    }

    if(unlikely(debug_enabled))
        debug_log_int("READ PROC/PID/STAT: %s/proc/%d/stat, process: '%s' on target '%s' (dt=%llu) VALUES: utime=" KERNEL_UINT_FORMAT ", stime=" KERNEL_UINT_FORMAT ", cutime=" KERNEL_UINT_FORMAT ", cstime=" KERNEL_UINT_FORMAT ", minflt=" KERNEL_UINT_FORMAT ", majflt=" KERNEL_UINT_FORMAT ", cminflt=" KERNEL_UINT_FORMAT ", cmajflt=" KERNEL_UINT_FORMAT ", threads=" KERNEL_UINT_FORMAT,
                      netdata_configured_host_prefix, p->pid, pid_stat_comm(p), (p->target)?string2str(p->target->name):"UNSET", p->stat_collected_usec - p->last_stat_collected_usec,
                      p->values[PDF_UTIME],
                      p->values[PDF_STIME],
                      p->values[PDF_CUTIME],
                      p->values[PDF_CSTIME],
                      p->values[PDF_MINFLT],
                      p->values[PDF_MAJFLT],
                      p->values[PDF_CMINFLT],
                      p->values[PDF_CMAJFLT],
                      p->values[PDF_THREADS]);

    update_proc_state_count(p->state);
    return true;

cleanup:
    return false;
}

// ----------------------------------------------------------------------------

// 1. read all files in /proc
// 2. for each numeric directory:
//    i.   read /proc/pid/stat
//    ii.  read /proc/pid/status
//    iii. read /proc/pid/io (requires root access)
//    iii. read the entries in directory /proc/pid/fd (requires root access)
//         for each entry:
//         a. find or create a struct file_descriptor
//         b. cleanup any old/unused file_descriptors

// after all these, some pids may be linked to targets, while others may not

// in case of errors, only 1 every 1000 errors is printed
// to avoid filling up all disk space
// if debug is enabled, all errors are printed

bool apps_os_collect_all_pids_linux(void) {
#if (PROCESSES_HAVE_STATE == 1)
    // clear process state counter
    memset(proc_state_count, 0, sizeof proc_state_count);
#endif

    // preload the parents and then their children
    collect_parents_before_children();

    static char uptime_filename[FILENAME_MAX + 1] = "";
    if(*uptime_filename == '\0')
        snprintfz(uptime_filename, FILENAME_MAX, "%s/proc/uptime", netdata_configured_host_prefix);

    system_uptime_secs = (kernel_uint_t)(uptime_msec(uptime_filename) / MSEC_PER_SEC);

    char dirname[FILENAME_MAX + 1];

    snprintfz(dirname, FILENAME_MAX, "%s/proc", netdata_configured_host_prefix);
    DIR *dir = opendir(dirname);
    if(!dir) return false;

    struct dirent *de = NULL;

    while((de = readdir(dir))) {
        char *endptr = de->d_name;

        if(unlikely(de->d_type != DT_DIR || de->d_name[0] < '0' || de->d_name[0] > '9'))
            continue;

        pid_t pid = (pid_t) strtoul(de->d_name, &endptr, 10);

        // make sure we read a valid number
        if(unlikely(endptr == de->d_name || *endptr != '\0'))
            continue;

        incrementally_collect_data_for_pid(pid, NULL);
    }
    closedir(dir);

#if (PROCESSES_HAVE_SMAPS_ROLLUP == 1)
    apps_handle_smaps_updates();
#endif

    return true;
}

#endif
