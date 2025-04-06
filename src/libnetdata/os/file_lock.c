// SPDX-License-Identifier: GPL-3.0-or-later

#include "file_lock.h"

#if defined(OS_LINUX) || defined(OS_FREEBSD) || defined(OS_MACOS)
#include <sys/file.h>
#include <fcntl.h>
#include <unistd.h>
#endif

#if defined(OS_WINDOWS)
#include <windows.h>
#endif

FILE_LOCK file_lock_get(const char *filename) {
    if(!filename || !*filename)
        return FILE_LOCK_INVALID;

#if defined(OS_LINUX) || defined(OS_FREEBSD) || defined(OS_MACOS)
    // Try to create a new file, or open existing one
    int fd = open(filename, O_RDWR | O_CREAT, 0666);
    if(fd == -1)
        return FILE_LOCK_INVALID;

    // LOCK_NB makes flock() non-blocking
    if(flock(fd, LOCK_EX | LOCK_NB) == -1) {
        close(fd);
        return FILE_LOCK_INVALID;
    }

    return (FILE_LOCK){ .fd = fd };

#elif defined(OS_WINDOWS)
    // Convert MSYS2/Cygwin path directly to Windows wide-char path
    ssize_t wpath_size = cygwin_conv_path(CCP_POSIX_TO_WIN_W, filename, NULL, 0);
    if(wpath_size < 0)
        return FILE_LOCK_INVALID;

    wchar_t *wpath = mallocz(wpath_size);
    if(cygwin_conv_path(CCP_POSIX_TO_WIN_W, filename, wpath, wpath_size) != 0) {
        freez(wpath);
        return FILE_LOCK_INVALID;
    }

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
    if(!LockFileEx(
            hFile,
            LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY,
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
