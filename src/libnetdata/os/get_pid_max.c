// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

pid_t pid_max = 32768;
pid_t os_get_system_pid_max(void) {
#if defined(OS_MACOS)

    // As we currently do not know a solution to query pid_max from the os
    // we use the number defined in bsd/sys/proc_internal.h in XNU sources
    pid_max = 99999;
    return pid_max;

#elif defined(OS_FREEBSD)

    int32_t tmp_pid_max;

    if (unlikely(GETSYSCTL_BY_NAME("kern.pid_max", tmp_pid_max))) {
        pid_max = 99999;
        netdata_log_error("Assuming system's maximum pid is %d.", pid_max);
    } else {
        pid_max = tmp_pid_max;
    }

    return pid_max;

#elif defined(OS_LINUX)

    static char read = 0;
    if(unlikely(read)) return pid_max;
    read = 1;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/sys/kernel/pid_max", netdata_configured_host_prefix?netdata_configured_host_prefix:"");

    unsigned long long max = 0;
    if(read_single_number_file(filename, &max) != 0) {
        netdata_log_error("Cannot open file '%s'. Assuming system supports %d pids.", filename, pid_max);
        return pid_max;
    }

    if(!max) {
        netdata_log_error("Cannot parse file '%s'. Assuming system supports %d pids.", filename, pid_max);
        return pid_max;
    }

    pid_max = (pid_t) max;
    return pid_max;

#else

    // just a big default

    pid_max = 4194304;
    return pid_max;

#endif
}
