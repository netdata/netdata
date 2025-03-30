// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#if defined(OS_LINUX)
#include <sys/statvfs.h>

OS_SYSTEM_DISK_SPACE os_disk_space(const char *path) {
    OS_SYSTEM_DISK_SPACE space = OS_SYSTEM_DISK_SPACE_EMPTY;
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
    space.is_read_only  = (buf.f_flag & ST_RDONLY) != 0;
    return space;
}
#endif

#if defined(OS_FREEBSD) || defined(OS_MACOS)
#include <sys/param.h>
#include <sys/mount.h>

OS_SYSTEM_DISK_SPACE os_disk_space(const char *path) {
    OS_SYSTEM_DISK_SPACE space = OS_SYSTEM_DISK_SPACE_EMPTY;
    struct statfs buf;

    if (statfs(path, &buf) != 0) {
        // Error occurred; errno is set
        return space;
    }

    space.total_bytes   = buf.f_blocks * buf.f_bsize;
    space.free_bytes    = buf.f_bavail * buf.f_bsize;
    space.total_inodes  = buf.f_files;
    space.free_inodes   = buf.f_ffree;
    space.is_read_only  = (buf.f_flags & MNT_RDONLY) != 0;
    return space;
}
#endif

#if defined(OS_WINDOWS)
#include <windows.h>

OS_SYSTEM_DISK_SPACE os_disk_space(const char *path_utf8) {
    OS_SYSTEM_DISK_SPACE space = OS_SYSTEM_DISK_SPACE_EMPTY;

    ssize_t wpath_size = cygwin_conv_path(CCP_POSIX_TO_WIN_W, path_utf8, NULL, 0);
    if(wpath_size < 0)
        return space;

    wchar_t *wpath = mallocz(wpath_size);
    if(cygwin_conv_path(CCP_POSIX_TO_WIN_W, path_utf8, wpath, wpath_size) != 0) {
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

    // Get the drive type and attributes
    DWORD attributes = GetFileAttributesW(wpath);
    if (attributes != INVALID_FILE_ATTRIBUTES) {
        space.is_read_only = (attributes & FILE_ATTRIBUTE_READONLY) != 0;
    }

    freez(wpath);

    space.total_bytes  = totalNumberOfBytes.QuadPart;
    space.free_bytes   = totalNumberOfFreeBytes.QuadPart;
    space.total_inodes = 0;  // Windows does not have inodes
    space.free_inodes  = 0;  // Windows does not have inodes
    return space;
}
#endif
