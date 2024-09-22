// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

kernel_uint_t MemTotal = 0;

#if defined(OS_FREEBSD)
static inline bool get_MemTotal_per_os(void) {
    int mib[2] = {CTL_HW, HW_PHYSMEM};
    size_t size = sizeof(MemTotal);
    if (sysctl(mib, 2, &MemTotal, &size, NULL, 0) == -1) {
        netdata_log_error("Failed to get total memory using sysctl");
        return false;
    }
    // FreeBSD returns bytes; convert to kB
    MemTotal /= 1024;
    return true;
}
#endif // __FreeBSD__

#if defined(OS_MACOS)
static inline bool get_MemTotal_per_os(void) {
    int mib[2] = {CTL_HW, HW_MEMSIZE};
    size_t size = sizeof(MemTotal);
    if (sysctl(mib, 2, &MemTotal, &size, NULL, 0) == -1) {
        netdata_log_error("Failed to get total memory using sysctl");
        return false;
    }
    // MacOS returns bytes; convert to kB
    MemTotal /= 1024;
    return true;
}
#endif // __APPLE__

#if defined(OS_LINUX)
static inline bool get_MemTotal_per_os(void) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/meminfo", netdata_configured_host_prefix);

    procfile *ff = procfile_open(filename, ": \t", PROCFILE_FLAG_DEFAULT);
    if(!ff)
        return false;

    ff = procfile_readall(ff);
    if(!ff)
        return false;

    size_t line, lines = procfile_lines(ff);

    for(line = 0; line < lines ;line++) {
        size_t words = procfile_linewords(ff, line);
        if(words == 3 && strcmp(procfile_lineword(ff, line, 0), "MemTotal") == 0 && strcmp(procfile_lineword(ff, line, 2), "kB") == 0) {
            kernel_uint_t n = str2ull(procfile_lineword(ff, line, 1), NULL);
            if(n) MemTotal = n;
            break;
        }
    }

    procfile_close(ff);

    return true;
}
#endif

#if defined(OS_WINDOWS)
static inline bool get_MemTotal_per_os(void) {
    MEMORYSTATUSEX memStat = { 0 };
    memStat.dwLength = sizeof(memStat);

    if (!GlobalMemoryStatusEx(&memStat)) {
        netdata_log_error("GlobalMemoryStatusEx() failed.");
        return false;
    }

    MemTotal = memStat.ullTotalPhys;
    return true;
}
#endif

void get_MemTotal(void) {
    if(!get_MemTotal_per_os())
        MemTotal = 0;
}
