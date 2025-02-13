// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#if defined(OS_LINUX)
#include <sys/statvfs.h>

OS_SYSTEM_DISK_SPACE os_disk_space(const char *path) {
    OS_SYSTEM_DISK_SPACE space = {0};
    struct statvfs buf;

    if (statvfs(path, &buf) != 0) {
        // Error occurred; errno is set
        return space;
    }

    // Use f_frsize (fragment size) for accurate byte calculations.
    space.total_bytes   = buf.f_blocks * buf.f_frsize;
    space.free_bytes    = buf.f_bavail * buf.f_frsize;
    space.total_inodes  = buf.f_files;
    space.free_inodes   = buf.f_favail;
    return space;
}
#endif

#if defined(OS_FREEBSD) || defined(OS_MACOS)
#include <sys/param.h>
#include <sys/mount.h>

OS_SYSTEM_DISK_SPACE os_disk_space(const char *path) {
    OS_SYSTEM_DISK_SPACE space = {0};
    struct statfs buf;

    if (statfs(path, &buf) != 0) {
        // Error occurred; errno is set
        return space;
    }

    space.total_bytes   = buf.f_blocks * buf.f_bsize;
    space.free_bytes    = buf.f_bavail * buf.f_bsize;
    space.total_inodes  = buf.f_files;
    space.free_inodes   = buf.f_ffree;
    return space;
}
#endif

#if defined(OS_WINDOWS)
#include <windows.h>

OS_SYSTEM_DISK_SPACE os_disk_space(const char *path_utf8) {
    OS_SYSTEM_DISK_SPACE space = {0};

    // Convert the UTF-8 path to a wide-character string.
    int wlen = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, path_utf8, -1, NULL, 0);
    if (wlen == 0) {
        // Conversion error; optionally, GetLastError() can provide more details.
        return space;
    }

    wchar_t *wpath = (wchar_t *)mallocz(wlen * sizeof(wchar_t));

    if (MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, path_utf8, -1, wpath, wlen) == 0) {
        // Conversion error.
        freez(wpath);
        return space;
    }

    // Use the wide-character version of GetDiskFreeSpaceEx.
    ULARGE_INTEGER freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes;
    if (!GetDiskFreeSpaceExW(wpath, &freeBytesAvailable, &totalNumberOfBytes, &totalNumberOfFreeBytes)) {
        // API call failed; optionally, GetLastError() can provide more details.
        freez(wpath);
        return space;
    }

    freez(wpath);

    space.total_bytes  = totalNumberOfBytes.QuadPart;
    space.free_bytes   = totalNumberOfFreeBytes.QuadPart;
    space.total_inodes = 0;  // Windows does not have inodes
    space.free_inodes  = 0;  // Windows does not have inodes
    return space;
}
#endif
