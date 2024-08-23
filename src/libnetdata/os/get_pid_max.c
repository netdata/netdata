// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

pid_t pid_max = 4194304;

pid_t os_get_system_pid_max(void) {
    static bool read = false;
    if(read) return pid_max;
    read = true;

#if defined(OS_MACOS)
    int mib[2];
    int maxproc;
    size_t len = sizeof(maxproc);

    mib[0] = CTL_KERN;
    mib[1] = KERN_MAXPROC;

    if (sysctl(mib, 2, &maxproc, &len, NULL, 0) == -1) {
        pid_max = 99999; // Fallback value
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot find system max pid. Assuming %d.", pid_max);
    }
    else pid_max = (pid_t)maxproc;

    return pid_max;

#elif defined(OS_FREEBSD)

    int32_t tmp_pid_max;

    if (unlikely(GETSYSCTL_BY_NAME("kern.pid_max", tmp_pid_max))) {
        pid_max = 99999;
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot get system max pid. Assuming %d.", pid_max);
    }
    else
        pid_max = tmp_pid_max;

    return pid_max;

#elif defined(OS_LINUX)

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/sys/kernel/pid_max", netdata_configured_host_prefix?netdata_configured_host_prefix:"");

    unsigned long long max = 0;
    if(read_single_number_file(filename, &max) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot open file '%s'. Assuming system supports %d pids.", filename, pid_max);
        return pid_max;
    }

    if(!max) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot parse file '%s'. Assuming system supports %d pids.", filename, pid_max);
        return pid_max;
    }

    pid_max = (pid_t) max;
    return pid_max;

#elif defined(OS_WINDOWS)

    pid_max = (pid_t)0x7FFFFFFF;
    return pid_max;

#else

    // return the default
    return pid_max;

#endif
}
