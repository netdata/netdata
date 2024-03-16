// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

int read_proc_pid_cmdline(struct pid_stat *p) {
    static char cmdline[MAX_CMDLINE + 1];

#ifdef __FreeBSD__
    size_t i, bytes = MAX_CMDLINE;
    int mib[4];

    mib[0] = CTL_KERN;
    mib[1] = KERN_PROC;
    mib[2] = KERN_PROC_ARGS;
    mib[3] = p->pid;
    if (unlikely(sysctl(mib, 4, cmdline, &bytes, NULL, 0)))
        goto cleanup;
#else
    if(unlikely(!p->cmdline_filename)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/cmdline", netdata_configured_host_prefix, p->pid);
        p->cmdline_filename = strdupz(filename);
    }

    int fd = open(p->cmdline_filename, procfile_open_flags, 0666);
    if(unlikely(fd == -1)) goto cleanup;

    ssize_t i, bytes = read(fd, cmdline, MAX_CMDLINE);
    close(fd);

    if(unlikely(bytes < 0)) goto cleanup;
#endif

    cmdline[bytes] = '\0';
    for(i = 0; i < bytes ; i++) {
        if(unlikely(!cmdline[i])) cmdline[i] = ' ';
    }

    if(p->cmdline) freez(p->cmdline);
    p->cmdline = strdupz(cmdline);

    debug_log("Read file '%s' contents: %s", p->cmdline_filename, p->cmdline);

    return 1;

cleanup:
    // copy the command to the command line
    if(p->cmdline) freez(p->cmdline);
    p->cmdline = strdupz(p->comm);
    return 0;
}
