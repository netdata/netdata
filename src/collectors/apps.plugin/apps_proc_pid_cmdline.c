// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

#if defined(OS_MACOS)
bool get_cmdline_per_os(struct pid_stat *p, char *cmdline, size_t maxBytes) {
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
#endif

#if defined(OS_FREEBSD)
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
#endif

#if defined(OS_LINUX)
static inline bool get_cmdline_per_os(struct pid_stat *p, char *cmdline, size_t bytes) {
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

    return true;
}
#endif

#if defined(OS_WINDOWS)
static inline bool get_cmdline_per_os(struct pid_stat *p, char *cmdline, size_t bytes) {
    // TODO: get the command line from perflib, if available
    return false;
}
#endif

int read_proc_pid_cmdline(struct pid_stat *p) {
    static char cmdline[MAX_CMDLINE];

    if(unlikely(!get_cmdline_per_os(p, cmdline, sizeof(cmdline))))
        goto cleanup;

    string_freez(p->cmdline);
    p->cmdline = string_strdupz(cmdline);

    return 1;

cleanup:
    // copy the command to the command line
    string_freez(p->cmdline);
    p->cmdline = string_dup(p->comm);
    return 0;
}
