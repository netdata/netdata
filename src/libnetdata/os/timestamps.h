// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNETDATA_OS_TIMESTAMPS_H
#define LIBNETDATA_OS_TIMESTAMPS_H

#include <time.h>

// Windows file time starts on January 1, 1601, Unix epoch starts on January 1, 1970
// Difference in 100-nanosecond intervals between these two dates is 116444736000000000ULL

// Convert Windows file time (in 100-nanosecond intervals) to Unix epoch in nanoseconds
#define os_windows_ulonglong_to_unix_epoch_ns(ft) (((uint64_t)(ft) - 116444736000000000ULL) * 100ULL)

// Convert Unix epoch time (in nanoseconds) to Windows file time (in 100-nanosecond intervals)
#define os_unix_epoch_ns_to_windows_ulonglong(ns) (((uint64_t)(ns) / 100ULL) + 116444736000000000ULL)

#if defined(OS_WINDOWS)
// Convert FILETIME to Unix epoch in nanoseconds
#define os_filetime_to_unix_epoch_ns(ft) \
    ((((uint64_t)(ft).dwHighDateTime << 32 | (ft).dwLowDateTime) - 116444736000000000ULL) * 100ULL)

// Convert Unix epoch in nanoseconds to FILETIME (returns FILETIME)
#define os_unix_epoch_ns_to_filetime(ns)                                        \
    ({                                                                          \
        uint64_t temp = ((uint64_t)(ns) / 100ULL) + 116444736000000000ULL;      \
        FILETIME ft;                                                            \
        ft.dwLowDateTime = (uint32_t)(temp & 0xFFFFFFFF);                       \
        ft.dwHighDateTime = (uint32_t)(temp >> 32);                             \
        ft;                                                                     \
    })

// Convert Unix epoch in microseconds to FILETIME (returns FILETIME)
#define os_unix_epoch_ut_to_filetime(ns)                                        \
    ({                                                                          \
        uint64_t temp = ((uint64_t)(ns) * 10ULL) + 116444736000000000ULL;       \
        FILETIME ft;                                                            \
        ft.dwLowDateTime = (uint32_t)(temp & 0xFFFFFFFF);                       \
        ft.dwHighDateTime = (uint32_t)(temp >> 32);                             \
        ft;                                                                     \
    })

#endif //OS_WINDOWS

// Portable replacement for struct tm::tm_gmtoff. POSIX/glibc/BSD expose
// the local-time UTC offset (seconds east of UTC, DST included) as a
// struct field, but it is a non-standard extension that UCRT's
// <time.h> does not provide. Use this helper instead of touching
// tm_gmtoff directly so call sites compile on every platform.
//
// The CMake-time check_c_source_compiles probe HAVE_TM_GMTOFF stays the
// authoritative source: when present, return the field directly; when
// absent, fall back to the C runtime's timezone state (UCRT's
// _get_timezone + _get_dstbias on Windows, the global `timezone` /
// `daylight` POSIX externs elsewhere).
static inline long nd_tm_gmtoff(const struct tm *tm) {
#if defined(HAVE_TM_GMTOFF)
    return tm ? tm->tm_gmtoff : 0;
#elif defined(OS_WINDOWS)
    long tz = 0;
    _get_timezone(&tz); // seconds west of UTC (POSIX `timezone` sign)
    long offset = -tz;
    if (tm && tm->tm_isdst > 0) {
        long dst_bias = 0;
        _get_dstbias(&dst_bias); // typically -3600 (DST is one hour ahead)
        offset -= dst_bias;
    }
    return offset;
#else
    extern long timezone;
    long offset = -timezone;
    if (tm && tm->tm_isdst > 0)
        offset += 3600;
    return offset;
#endif
}

#endif //LIBNETDATA_OS_TIMESTAMPS_H
