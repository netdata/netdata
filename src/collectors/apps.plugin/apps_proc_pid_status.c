// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

#if defined(OS_FREEBSD)
static inline bool read_proc_pid_status_per_os(struct pid_stat *p, void *ptr) {
    struct kinfo_proc *proc_info = (struct kinfo_proc *)ptr;

    p->uid                  = proc_info->ki_uid;
    p->gid                  = proc_info->ki_groups[0];
    p->status_vmsize        = proc_info->ki_size / 1024; // in KiB
    p->status_vmrss         = proc_info->ki_rssize * pagesize / 1024; // in KiB
    // TODO: what about shared and swap memory on FreeBSD?
    return true;
}
#endif

#if defined(OS_MACOS)
static inline bool read_proc_pid_status_per_os(struct pid_stat *p, void *ptr) {
    struct pid_info *pi = ptr;

    p->uid = pi->bsdinfo.pbi_uid;
    p->gid = pi->bsdinfo.pbi_gid;
    p->status_vmsize = pi->taskinfo.pti_virtual_size / 1024; // Convert bytes to KiB
    p->status_vmrss = pi->taskinfo.pti_resident_size / 1024; // Convert bytes to KiB
    // p->status_vmswap = rusageinfo.ri_swapins + rusageinfo.ri_swapouts; // This is not directly available, consider an alternative representation
    p->status_voluntary_ctxt_switches = pi->taskinfo.pti_csw;
    // p->status_nonvoluntary_ctxt_switches = taskinfo.pti_nivcsw;

    return true;
}
#endif

#if defined(OS_WINDOWS)
static inline bool read_proc_pid_status_per_os(struct pid_stat *p, void *ptr) {
    // TODO: get these statistics from perflib
    return false;
}
#endif

#if defined(OS_LINUX)
struct arl_callback_ptr {
    struct pid_stat *p;
    procfile *ff;
    size_t line;
};

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

    aptr->p->status_vmsize = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1));
}

void arl_callback_status_vmswap(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 3)) return;

    aptr->p->status_vmswap = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1));
}

void arl_callback_status_vmrss(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 3)) return;

    aptr->p->status_vmrss = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1));
}

void arl_callback_status_rssfile(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 3)) return;

    aptr->p->status_rssfile = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1));
}

void arl_callback_status_rssshmem(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 3)) return;

    aptr->p->status_rssshmem = str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1));
}

void arl_callback_status_voluntary_ctxt_switches(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 2)) return;

    struct pid_stat *p = aptr->p;
    pid_incremental_rate(stat, p->status_voluntary_ctxt_switches, str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1)));
}

void arl_callback_status_nonvoluntary_ctxt_switches(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name; (void)hash; (void)value;
    struct arl_callback_ptr *aptr = (struct arl_callback_ptr *)dst;
    if(unlikely(procfile_linewords(aptr->ff, aptr->line) < 2)) return;

    struct pid_stat *p = aptr->p;
    pid_incremental_rate(stat, p->status_nonvoluntary_ctxt_switches, str2kernel_uint_t(procfile_lineword(aptr->ff, aptr->line, 1)));
}

static inline bool read_proc_pid_status_per_os(struct pid_stat *p, void *ptr __maybe_unused) {
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

    p->status_vmshared = p->status_rssfile + p->status_rssshmem;

    // debug_log("%s uid %d, gid %d, VmSize %zu, VmRSS %zu, RssFile %zu, RssShmem %zu, shared %zu", p->comm, (int)p->uid, (int)p->gid, p->status_vmsize, p->status_vmrss, p->status_rssfile, p->status_rssshmem, p->status_vmshared);

    return true;
}
#endif // !__FreeBSD__ !__APPLE__

int read_proc_pid_status(struct pid_stat *p, void *ptr) {
    p->status_vmsize           = 0;
    p->status_vmrss            = 0;
    p->status_vmshared         = 0;
    p->status_rssfile          = 0;
    p->status_rssshmem         = 0;
    p->status_vmswap           = 0;
    p->status_voluntary_ctxt_switches = 0;
    p->status_nonvoluntary_ctxt_switches = 0;

    return read_proc_pid_status_per_os(p, ptr) ? 1 : 0;
}
