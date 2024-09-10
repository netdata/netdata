// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNETDATA_OS_TIMESTAMPS_H
#define LIBNETDATA_OS_TIMESTAMPS_H

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

#endif //LIBNETDATA_OS_TIMESTAMPS_H
