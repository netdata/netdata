// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

static time_t cached_boottime = 0;
static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

#if defined(OS_LINUX)

static time_t calculate_boottime(void) {
    char buf[8192];

    char filename[FILENAME_MAX + 1];

    // Try to read from /proc/stat first - this provides the absolute timestamp
    snprintfz(filename, sizeof(filename), "%s/proc/stat",
              netdata_configured_host_prefix ? netdata_configured_host_prefix : "");
    if (read_txt_file(filename, buf, sizeof(buf)) == 0) {
        char *btime_line = strstr(buf, "btime ");
        if (btime_line) {
            time_t btime = (time_t)str2ull(btime_line + 6, NULL);
            if (btime > 0)
                return btime;
        }
    }

    // If btime is not available, calculate it from uptime
    snprintfz(filename, sizeof(filename), "%s/proc/uptime",
              netdata_configured_host_prefix ? netdata_configured_host_prefix : "");
    if (read_txt_file(filename, buf, sizeof(buf)) == 0) {
        double uptime;
        if (sscanf(buf, "%lf", &uptime) == 1) {
            time_t now = now_realtime_sec();
            time_t boottime = now - (time_t)uptime;
            if(boottime > 0)
                return boottime;
        }
    }

    return 0;
}

#elif defined(OS_FREEBSD) || defined(OS_MACOS)

#include <sys/sysctl.h>

static time_t calculate_boottime(void) {
    struct timeval boottime;
    size_t size = sizeof(boottime);

    // kern.boottime provides the absolute timestamp
    if (sysctlbyname("kern.boottime", &boottime, &size, NULL, 0) == 0)
        return boottime.tv_sec;

    return 0;
}

#elif defined(OS_WINDOWS)

#include <windows.h>

static time_t calculate_boottime(void) {
    ULONGLONG uptime_ms = GetTickCount64();
    if (uptime_ms > 0) {
        FILETIME ft;
        ULARGE_INTEGER now;

        GetSystemTimeAsFileTime(&ft);
        now.HighPart = ft.dwHighDateTime;
        now.LowPart = ft.dwLowDateTime;

        // Convert to Unix epoch (subtract Windows epoch)
        ULONGLONG unix_time_ms = (now.QuadPart - 116444736000000000ULL) / 10000;
        time_t boottime = (time_t)((unix_time_ms - uptime_ms) / 1000);

        if(boottime > 0)
            return boottime;
    }

    return 0;
}

#endif

static time_t get_stable_boottime(void) {
    const int max_attempts = 100;
    const int required_matches = 5;
    time_t last_boottime = 0;
    int matches = 0;

    for(int i = 0; i < max_attempts; i++) {
        time_t new_boottime = calculate_boottime();
        if(new_boottime == 0)
            new_boottime = now_realtime_sec() - now_boottime_sec();

        if(new_boottime == last_boottime)
            matches++;
        else {
            matches = 1;
            last_boottime = new_boottime;
        }

        if(matches >= required_matches)
            return new_boottime;

        microsleep(1000); // 1ms
    }

    return 0;
}

time_t os_boottime(void) {
    // Fast path - return cached value if available
    if(cached_boottime > 0)
        return cached_boottime;

    spinlock_lock(&spinlock);

    // Check again under lock in case another thread set it
    if(cached_boottime == 0)
        cached_boottime = get_stable_boottime();

    spinlock_unlock(&spinlock);
    return cached_boottime;
}
