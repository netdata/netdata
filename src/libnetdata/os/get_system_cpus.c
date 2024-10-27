// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#define CPUS_FOR_COLLECTORS 0
#define CPUS_FOR_NETDATA 1

long os_get_system_cpus_cached(bool cache, bool for_netdata) {
    static long processors[2] = { 0, 0 };

    int index = for_netdata ? CPUS_FOR_NETDATA : CPUS_FOR_COLLECTORS;

    if(likely(cache && processors[index] > 0))
        return processors[index];

#if defined(OS_FREEBSD) || defined(OS_MACOS)
#if defined(OS_MACOS)
#define HW_CPU_NAME "hw.logicalcpu"
#else
#define HW_CPU_NAME "hw.ncpu"
#endif

    int32_t tmp_processors;
    bool error = false;

    if (unlikely(GETSYSCTL_BY_NAME(HW_CPU_NAME, tmp_processors)))
        error = true;
    else
        processors[index] = tmp_processors;

    if(processors[index] < 1) {
        processors[index] = 1;

        if(error)
            netdata_log_error("Assuming system has %ld processors.", processors[index]);
    }

    return processors[index];
#elif defined(OS_LINUX)

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/stat",
              (!for_netdata && netdata_configured_host_prefix) ? netdata_configured_host_prefix : "");

    procfile *ff = procfile_open(filename, NULL, PROCFILE_FLAG_DEFAULT);
    if(!ff) {
        processors[index] = 1;
        netdata_log_error("Cannot open file '%s'. Assuming system has %ld processors.", filename, processors[index]);
        return processors[index];
    }

    ff = procfile_readall(ff);
    if(!ff) {
        processors[index] = 1;
        netdata_log_error("Cannot open file '%s'. Assuming system has %ld processors.", filename, processors[index]);
        return processors[index];
    }

    long tmp_processors = 0;
    unsigned int i;
    for(i = 0; i < procfile_lines(ff); i++) {
        if(!procfile_linewords(ff, i)) continue;

        if(strncmp(procfile_lineword(ff, i, 0), "cpu", 3) == 0)
            tmp_processors++;
    }
    procfile_close(ff);

    processors[index] = --tmp_processors;

    if(processors[index] < 1)
        processors[index] = 1;

    netdata_log_debug(D_SYSTEM, "System has %ld processors.", processors[index]);
    return processors[index];

#elif defined(OS_WINDOWS)

    SYSTEM_INFO sysInfo;
    GetSystemInfo(&sysInfo);
    processors[index] = sysInfo.dwNumberOfProcessors;

    if(processors[index] < 1) {
        processors[index] = 1;
        netdata_log_error("Assuming system has %ld processors.", processors[index]);
    }

    return processors[index];

#else

    processors[index] = 1;
    return processors[index];

#endif
}
