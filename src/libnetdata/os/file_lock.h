// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FILE_LOCK_H
#define NETDATA_FILE_LOCK_H

#include "libnetdata/libnetdata.h"

typedef struct file_lock {
#if defined(OS_LINUX) || defined(OS_FREEBSD) || defined(OS_MACOS)
    int fd;
#elif defined(OS_WINDOWS)
    HANDLE handle;
#else
#error "Unsupported operating system"
#endif
} FILE_LOCK;

// Initialize to invalid values
#if defined(OS_LINUX) || defined(OS_FREEBSD) || defined(OS_MACOS)
#define FILE_LOCK_INVALID ((FILE_LOCK){ .fd = -1 })
#define FILE_LOCK_OK(lock) ((lock).fd != -1)
#elif defined(OS_WINDOWS)
#define FILE_LOCK_INVALID ((FILE_LOCK){ .handle = INVALID_HANDLE_VALUE })
#define FILE_LOCK_OK(lock) ((lock).handle != INVALID_HANDLE_VALUE)
#endif

/**
 * Get a file lock
 *
 * Attempts to acquire an exclusive lock on a file. The lock is automatically released
 * when the process exits or if the process crashes. Only one process can hold the lock
 * at a time.
 *
 * @param filename UTF-8 encoded filename (MSYS2 path format on Windows)
 * @return FILE_LOCK The lock handle. Use FILE_LOCK_OK() to check if lock was acquired
 */
FILE_LOCK file_lock_get(const char *filename);

/**
 * Release a file lock
 *
 * Releases a previously acquired file lock. After calling this function,
 * another process may acquire the lock.
 *
 * @param lock The lock to release
 */
void file_lock_release(FILE_LOCK lock);

#endif //NETDATA_FILE_LOCK_H
