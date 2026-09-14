// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "status-file-io.h"

// List of fallback directories to try
static const char *status_file_io_fallback_dirs[] = {
    CACHE_DIR,
    "/tmp",
    "/run",
    "/var/run",
    ".",
};

static bool status_file_io_obsolete_removed = false;
static _Alignas(8) uint64_t status_file_io_tmp_attempt_counter = 0;

_Static_assert(__atomic_always_lock_free(sizeof(status_file_io_obsolete_removed),
                                         &status_file_io_obsolete_removed),
               "signal-safe status cleanup state must be lock-free");
_Static_assert(__alignof__(status_file_io_tmp_attempt_counter) >= sizeof(status_file_io_tmp_attempt_counter),
               "signal-safe status temporary counter must be naturally aligned");
_Static_assert(__atomic_always_lock_free(sizeof(status_file_io_tmp_attempt_counter), NULL),
               "signal-safe status temporary counter must be lock-free");

static void status_file_io_fallback_dirs_update(void) {
    status_file_io_fallback_dirs[0] = netdata_configured_cache_dir;
}

static bool status_file_io_check(const char *directory, const char *filename, char *dst, size_t dst_size, time_t *mtime) {
    // Used by the non-signal load path; it may probe filesystem metadata.

    if(!directory || !*directory || !filename || !*filename || !dst || !dst_size || !mtime)
        return false;

    size_t len = 0;
    len = strcatz(dst, len, directory, dst_size);
    if(!len || dst[len - 1] != '/')
        len = strcatz(dst, len, "/", dst_size);
    len = strcatz(dst, len, filename, dst_size);

    // Get file metadata
    OS_FILE_METADATA metadata = os_get_file_metadata(dst);
    if (!OS_FILE_METADATA_OK(metadata)) {
        *mtime = 0;
        return false;
    }

    *mtime = metadata.modified_time;
    return true;
}

static void status_file_io_remove_obsolete(const char *protected_dir, const char *filename) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    if(__atomic_exchange_n(&status_file_io_obsolete_removed, true, __ATOMIC_RELAXED))
        return;

    char dst[FILENAME_MAX];

    status_file_io_fallback_dirs_update();
    for(size_t i = 0; i < _countof(status_file_io_fallback_dirs); i++) {
        if(strcmp(status_file_io_fallback_dirs[i], protected_dir) == 0)
            continue;

        size_t len = 0;
        len = strcatz(dst, len, status_file_io_fallback_dirs[i], sizeof(dst));
        if(!len || dst[len - 1] != '/')
            len = strcatz(dst, len, "/", sizeof(dst));
        len = strcatz(dst, len, filename, sizeof(dst));

        unlink(dst);
    }

    errno_clear();
}

bool status_file_io_load(const char *filename, bool (*cb)(const char *, void *), void *data, bool log) {
    // Do not call this from signal handlers; it probes filesystem metadata and may log.

    char newest[FILENAME_MAX] = "";
    char current[FILENAME_MAX];
    time_t newest_mtime = 0, current_mtime;

    // Check the primary directory first
    if(status_file_io_check(netdata_configured_varlib_dir, filename, current, sizeof(current), &current_mtime)) {
        strncpyz(newest, current, sizeof(newest) - 1);
        newest_mtime = current_mtime;
    }

    // Check each fallback location
    status_file_io_fallback_dirs_update();
    for(size_t i = 0; i < _countof(status_file_io_fallback_dirs); i++) {
        if(status_file_io_check(status_file_io_fallback_dirs[i], filename, current, sizeof(current), &current_mtime) &&
            (!*newest || current_mtime > newest_mtime)) {
            strncpyz(newest, current, sizeof(newest) - 1);
            newest_mtime = current_mtime;
        }
    }

    // Load the newest file found
    if(*newest && cb(newest, data))
        return true;

    if(log)
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot find a status file in any location");

    return false;
}

static bool status_file_io_save_this(const char *directory, const char *filename, const uint8_t *data, size_t size) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    // Linux: https://man7.org/linux/man-pages/man7/signal-safety.7.html
    // memcpy(), strlen(), lstat(), open(), fstat(), ftruncate(), write(), fsync(), close(), fchmod(), rename(), unlink()

    // MacOS: https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/sigaction.2.html#//apple_ref/doc/man/2/sigaction
    // lstat(), open(), fstat(), ftruncate(), write(), fsync(), close(), rename(), unlink()
    // does not explicitly mention fchmod, memcpy(), and strlen(), but they are safe

    if(!directory || !*directory)
        return false;

    char final[FILENAME_MAX];
    char temp[FILENAME_MAX];
    char tid_str[UINT64_MAX_LENGTH];

    print_uint64(tid_str, __atomic_add_fetch(&status_file_io_tmp_attempt_counter, 1, __ATOMIC_RELAXED));
    size_t dir_len = strlen(directory);
    size_t fil_len = strlen(filename);
    size_t tid_len = strlen(tid_str);

    if (dir_len + 1 + fil_len + 1 + tid_len + 1 >= sizeof(final))
        return false; // cannot fit the filename

    // create the filename
    size_t pos = 0;
    memcpy(&final[pos], directory, dir_len); pos += dir_len;
    final[pos] = '/'; pos++;
    memcpy(&final[pos], filename, fil_len); pos += fil_len;
    final[pos] = '\0';

    // create the temp filename
    memcpy(temp, final, pos);
    temp[pos] = '-'; pos++;
    memcpy(&temp[pos], tid_str, tid_len); pos += tid_len;
    temp[pos] = '\0';

    // Reuse a regular temporary file left by an interrupted save; create absent names exclusively.
    struct stat before;
    bool reuse = lstat(temp, &before) == 0;
    if(reuse && !S_ISREG(before.st_mode))
        return false;

    if(!reuse && errno != ENOENT)
        return false;

    int flags = O_WRONLY | O_NONBLOCK;
#ifdef O_NOFOLLOW
    flags |= O_NOFOLLOW;
#endif
    if(!reuse)
        flags |= O_CREAT | O_EXCL;

    int fd = open(temp, flags, 0664);
    if (fd == -1)
        return false;

    struct stat after;
    if(fstat(fd, &after) != 0 || !S_ISREG(after.st_mode) ||
       (reuse && (before.st_dev != after.st_dev || before.st_ino != after.st_ino))) {
        close(fd);
        return false;
    }

    if(reuse && ftruncate(fd, 0) != 0) {
        close(fd);
        return false;
    }

    /* Write content to file using write() */
    size_t total_written = 0;

    while (total_written < size) {
        ssize_t bytes_written = write(fd, data + total_written, size - total_written);

        if (bytes_written <= 0) {
            if (errno == EINTR)
                continue; /* Retry if interrupted by signal */

            close(fd);
            unlink(temp);  /* Remove the temp file */
            return false;
        }

        total_written += bytes_written;
    }

    /* Fsync to ensure data is written to disk */
    if (fsync(fd) == -1) {
        close(fd);
        unlink(temp);
        return false;
    }

    /* Set permissions using chmod() */
    if (fchmod(fd, 0664) != 0) {
        close(fd);
        unlink(temp);
        return false;
    }

    /* Close file */
    if (close(fd) == -1) {
        unlink(temp);
        return false;
    }

    /* Rename temp file to target file */
    if (rename(temp, final) != 0) {
        unlink(temp);
        return false;
    }

    return true;
}

bool status_file_io_save(const char *filename, const void *data, size_t size, bool log) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    // wb should have enough space to hold the JSON content, to avoid any allocations

    // Try primary directory first
    bool saved = false;
    if (status_file_io_save_this(netdata_configured_varlib_dir, filename, data, size)) {
        status_file_io_remove_obsolete(netdata_configured_varlib_dir, filename);
        saved = true;
    }
    else {
        if(log)
            nd_log(NDLS_DAEMON, NDLP_DEBUG, "Failed to save status file in primary directory %s",
                   netdata_configured_varlib_dir);

        // Try each fallback directory until successful
        status_file_io_fallback_dirs_update();
        for(size_t i = 0; i < _countof(status_file_io_fallback_dirs); i++) {
            if (status_file_io_save_this(status_file_io_fallback_dirs[i], filename, data, size)) {
                if(log)
                    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Saved status file in fallback %s", status_file_io_fallback_dirs[i]);

                saved = true;
                break;
            }
        }
    }

    if (!saved && log)
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to save status file in any location");

    return saved;
}
