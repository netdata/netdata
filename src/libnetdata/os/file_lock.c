// SPDX-License-Identifier: GPL-3.0-or-later

#include "file_lock.h"
#include "os.h"
#include "../memory/nd-mallocz.h"

#include <errno.h>
#include <stdbool.h>

#if defined(OS_LINUX) || defined(OS_FREEBSD) || defined(OS_MACOS)
#include <sys/file.h>
#include <fcntl.h>
#include <unistd.h>
#endif

#if defined(OS_WINDOWS)
#include <windows.h>
#endif

static FILE_LOCK file_lock_get_internal(const char *filename, bool wait) {
    if(!filename || !*filename)
        return FILE_LOCK_INVALID;

#if defined(OS_LINUX) || defined(OS_FREEBSD) || defined(OS_MACOS)
    // Try to create a new file, or open existing one
    int fd = open(filename, O_RDWR | O_CREAT, 0600);
    if(fd == -1)
        return FILE_LOCK_INVALID;

    int rc;
    do {
        rc = flock(fd, LOCK_EX | (wait ? 0 : LOCK_NB));
    } while(wait && rc == -1 && errno == EINTR);

    if(rc == -1) {
        int saved_errno = errno;
        close(fd);
        errno = saved_errno;
        return FILE_LOCK_INVALID;
    }

    return (FILE_LOCK){ .fd = fd };

#elif defined(OS_WINDOWS)
    wchar_t *wpath = os_translate_msys_to_windows_pathW(filename);
    if(!wpath)
        return FILE_LOCK_INVALID;

    // Open existing file or create new one
    HANDLE hFile = CreateFileW(
        wpath,
        GENERIC_READ | GENERIC_WRITE,
        FILE_SHARE_READ | FILE_SHARE_WRITE,
        NULL,
        OPEN_ALWAYS,  // Open if exists, create if doesn't
        FILE_ATTRIBUTE_NORMAL,
        NULL
    );

    freez(wpath);

    if(hFile == INVALID_HANDLE_VALUE)
        return FILE_LOCK_INVALID;

    // Check if file is empty
    LARGE_INTEGER size;
    if(!GetFileSizeEx(hFile, &size)) {
        CloseHandle(hFile);
        return FILE_LOCK_INVALID;
    }

    // Write a byte only if file is empty
    if(size.QuadPart == 0) {
        DWORD written;
        if(!WriteFile(hFile, "!", 1, &written, NULL) || written != 1) {
            CloseHandle(hFile);
            return FILE_LOCK_INVALID;
        }
    }

    // Try to lock the entire file
    OVERLAPPED overlapped = {0};
    DWORD flags = LOCKFILE_EXCLUSIVE_LOCK;
    if(!wait)
        flags |= LOCKFILE_FAIL_IMMEDIATELY;

    if(!LockFileEx(
            hFile,
            flags,
            0,
            MAXDWORD,
            MAXDWORD,
            &overlapped)) {
        CloseHandle(hFile);
        return FILE_LOCK_INVALID;
    }

    return (FILE_LOCK){ .handle = hFile };

#else
#error "Unsupported operating system"
#endif
}

FILE_LOCK file_lock_get(const char *filename) {
    return file_lock_get_internal(filename, false);
}

FILE_LOCK file_lock_get_wait(const char *filename) {
    return file_lock_get_internal(filename, true);
}

void file_lock_release(FILE_LOCK lock) {
#if defined(OS_LINUX) || defined(OS_FREEBSD) || defined(OS_MACOS)
    if(FILE_LOCK_OK(lock)) {
        // flock is automatically released when file is closed
        close(lock.fd);
    }
#elif defined(OS_WINDOWS)
    if(FILE_LOCK_OK(lock)) {
        // File lock is automatically released when handle is closed
        CloseHandle(lock.handle);
    }
#endif
}
