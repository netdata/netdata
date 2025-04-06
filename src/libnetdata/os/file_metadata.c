// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include <errno.h>

//#if defined(OS_LINUX) || defined(OS_FREEBSD) || defined(OS_MACOS)
#include <sys/stat.h>

OS_FILE_METADATA os_get_file_metadata(const char *path) {
    OS_FILE_METADATA metadata = {0};
    struct stat st;

    if (stat(path, &st) != 0)
        return metadata;

    metadata.size_bytes = st.st_size;
    metadata.modified_time = st.st_mtime;
    return metadata;
}
//#endif

//#if defined(OS_WINDOWS)
//#include <windows.h>
//
//OS_FILE_METADATA os_get_file_metadata(const char *path) {
//    OS_FILE_METADATA metadata = {0};
//
//    // Convert UTF-8 path to wide-character string
//    int wlen = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, path, -1, NULL, 0);
//    if (wlen == 0)
//        return metadata;
//
//    wchar_t *wpath = (wchar_t *)mallocz(wlen * sizeof(wchar_t));
//    if (!wpath)
//        return metadata;
//
//    if (MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, path, -1, wpath, wlen) == 0) {
//        freez(wpath);
//        return metadata;
//    }
//
//    WIN32_FILE_ATTRIBUTE_DATA attr_data;
//    if (!GetFileAttributesExW(wpath, GetFileExInfoStandard, &attr_data)) {
//        freez(wpath);
//        return metadata;
//    }
//
//    freez(wpath);
//
//    // Combine high and low parts for 64-bit file size
//    ULARGE_INTEGER file_size;
//    file_size.HighPart = attr_data.nFileSizeHigh;
//    file_size.LowPart = attr_data.nFileSizeLow;
//    metadata.size_bytes = file_size.QuadPart;
//
//    // Convert Windows FILETIME to Unix timestamp
//    // Windows FILETIME is in 100-nanosecond intervals since January 1, 1601 UTC
//    // Need to convert to seconds since January 1, 1970 UTC
//    ULARGE_INTEGER win_time;
//    win_time.HighPart = attr_data.ftLastWriteTime.dwHighDateTime;
//    win_time.LowPart = attr_data.ftLastWriteTime.dwLowDateTime;
//
//    // Subtract Windows epoch start (January 1, 1601 UTC)
//    // Add Unix epoch start (January 1, 1970 UTC)
//    // Convert from 100-nanosecond intervals to seconds
//    const uint64_t WINDOWS_TICK = 10000000;
//    const uint64_t SEC_TO_UNIX_EPOCH = 11644473600LL;
//    metadata.modified_time = (time_t)((win_time.QuadPart / WINDOWS_TICK) - SEC_TO_UNIX_EPOCH);
//
//    return metadata;
//}
//#endif
