// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

#ifdef __APPLE__
bool get_cmdline_per_os(struct pid_stat *p, char *cmdline, size_t maxBytes) {
    int mib[3] = {CTL_KERN, KERN_PROCARGS2, p->pid};
    static char *args = NULL;
    static size_t size = 0;

    size_t new_size;
    if (sysctl(mib, 3, NULL, &new_size, NULL, 0) == -1) {
        return false;
    }

    if(new_size > size) {
        if(args)
            freez(args);

        args = mallocz(new_size);
        size = new_size;
    }

    size_t used_size = size;
    if (sysctl(mib, 3, args, &used_size, NULL, 0) == -1) {
        return false;
    }

    // The first part of the buffer contains the argc value (integer),
    // followed by the null-terminated executable path, and then the arguments.
    int argc;
    memcpy(&argc, args, sizeof(argc));
    char *ptr = args + sizeof(argc);
    size_t bytesCopied = 0;

    // Copy arguments to the cmdline buffer
    size_t i = 0;
    for (; i < size - bytesCopied - 1 && i < maxBytes - 1; ++i) {
        cmdline[i] = ptr[i] ? ptr[i] : ' ';
    }
    cmdline[i] = '\0'; // Null-terminate the string

    return true;
}
#endif // __APPLE__

#if defined(__FreeBSD__)
static inline bool get_cmdline_per_os(struct pid_stat *p, char *cmdline, size_t bytes) {
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
#endif // __FreeBSD__

#if !defined(__FreeBSD__) && !defined(__APPLE__)
static inline bool get_cmdline_per_os(struct pid_stat *p, char *cmdline, size_t bytes) {
    if(unlikely(!p->cmdline_filename)) {
        char filename[FILENAME_MAX + 1];
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

    return true;
}
#endif // !__FreeBSD__ !__APPLE__

int read_proc_pid_cmdline(struct pid_stat *p) {
    static char cmdline[MAX_CMDLINE + 1];

    if(unlikely(!get_cmdline_per_os(p, cmdline, sizeof(cmdline))))
        goto cleanup;

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
