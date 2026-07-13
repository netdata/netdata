// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "machine-guid.h"

static bool machine_guid_check_blacklisted(const char *guid) {
    // these are machine GUIDs that have been included in distribution packages.
    // we blacklist them here, so that the next version of netdata will generate
    // new ones.

    static char *blacklisted[] = {
        // Third party packaging problems
        "8a795b0c-2311-11e6-8563-000c295076a6",
        "4aed1458-1c3e-11e6-a53f-000c290fc8f5",

        // GitHub runner problems
        "a177c1dc-09d9-11f0-a920-0242ac110002",
        "983624e2-09d9-11f0-b90c-0242ac110002",
        "477f97ae-09d9-11f0-903d-0242ac110002",
        "ded81380-09e1-11f0-ae4c-0242ac110002",
        "9abc69ec-09d9-11f0-a8a4-0242ac110002",
        "68a2d17a-0aa2-11f0-97f3-0242ac110002",
        "6499dbbe-0aa2-11f0-9ccd-0242ac110002",
        "a9708cba-0aa2-11f0-98b6-0242ac110002",
        "26903986-0aab-11f0-818e-0242ac110002",
        "ab576242-0aa2-11f0-89c3-0242ac110002",
        "eab387c6-0b6b-11f0-b715-0242ac110002",
        "eaee7dfe-0b6b-11f0-870f-0242ac110002",
        "c7d4e6b4-0b6b-11f0-878c-0242ac110002",
        "40ac6d48-0b74-11f0-9cf4-0242ac110002",
        "e366fc5a-0b6b-11f0-bd77-0242ac110002",
        "c5955806-0c34-11f0-a302-0242ac110002",
        "1d4d05d0-0c35-11f0-a01d-0242ac110002",
        "edfc72b0-0c35-11f0-8e50-0242ac110002",
        "536a030e-0c3d-11f0-837b-0242ac110002",
        "10846e2e-0c35-11f0-8422-0242ac110002",
        "4339f742-0dc7-11f0-838c-0242ac110002",
        "3f28d7e0-0dc7-11f0-b75f-0242ac110002",
        "41815788-0dc7-11f0-88e0-0242ac110002",
        "104b408a-0dd0-11f0-8ca5-0242ac110002",
        "8e45bc30-0dc7-11f0-8e50-0242ac110002",
    };

    for(size_t i = 0; i < _countof(blacklisted); i++) {
        if (!strcmp(guid, blacklisted[i])) {
            nd_log(NDLS_DAEMON, NDLP_INFO, "MACHINE_GUID: blacklisted machine GUID '%s' found, generating new one.", guid);
            return true;
        }
    }

    return false;
}

static bool machine_guid_read_from_file(const char *filename, ND_MACHINE_GUID *host_id, bool log_errors) {
    if (!filename || !*filename)
        return false;

    ND_MACHINE_GUID h;
    memset(&h, 0, sizeof(h));

    struct stat st;
    if (stat(filename, &st) != 0 || !S_ISREG(st.st_mode)) {
        if(log_errors)
            nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot open GUID file '%s' for reading", filename);
        return false;
    }

    int fd = open(filename, O_RDONLY | O_CLOEXEC | O_NONBLOCK);
    if (fd == -1) {
        if(log_errors)
            nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot open GUID file '%s' for reading", filename);
        return false;
    }

    if (fstat(fd, &st) != 0 || !S_ISREG(st.st_mode)) {
        if(log_errors)
            nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot stat the GUID file '%s'", filename);
        close(fd);
        return false;
    }

    ssize_t bytes;
    do {
        bytes = read(fd, h.txt, sizeof(h.txt) - 1);
    } while(bytes < 0 && errno == EINTR);

    if (bytes != sizeof(h.txt) - 1) {
        if(log_errors)
            nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot read GUID file '%s'", filename);
        close(fd);
        return false;
    }
    h.txt[sizeof(h.txt) - 1] = '\0';

    if (uuid_parse(h.txt, h.uuid.uuid) != 0) {
        if(log_errors)
            nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot parse GUID from file '%s'", filename);
        close(fd);
        return false;
    }

    if (UUIDiszero(h.uuid)) {
        if(log_errors)
            nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: GUID read from file '%s' is zero", filename);
        close(fd);
        return false;
    }

    // Preserve the original post-read timestamp and metadata-error semantics.
    if (fstat(fd, &st) != 0) {
        if(log_errors)
            nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot stat the GUID file '%s'", filename);
        close(fd);
        return false;
    }

    close(fd);

    // Recreate the text version of it, ensuring lowercase format.
    uuid_unparse_lower(h.uuid.uuid, h.txt);

    if (machine_guid_check_blacklisted(h.txt))
        return false;

    // Update last modified timestamp.
    h.last_modified_ut = STAT_GET_MTIME_SEC(st) * USEC_PER_SEC + STAT_GET_MTIME_NSEC(st) / 1000;
    rfc3339_datetime_ut(h.last_modified_ut_rfc3339, sizeof(h.last_modified_ut_rfc3339), h.last_modified_ut, 2, true);
    *host_id = h;

    nd_log(NDLS_DAEMON, NDLP_INFO, "MACHINE_GUID: GUID read from file '%s'", filename);

    return true;
}

static struct timespec usec_to_timespec(usec_t usec) {
    struct timespec ts;
    ts.tv_sec = usec / USEC_PER_SEC;
    ts.tv_nsec = (usec % USEC_PER_SEC) * 1000;
    return ts;
}

static bool machine_guid_unlink_temporary(const char *tmp_filename, int saved_errno, const char *reason) {
    if (unlink(tmp_filename) != 0 && errno != ENOENT)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "MACHINE_GUID: cannot remove the temporary GUID file '%s' after %s",
               tmp_filename, reason);

    errno = saved_errno;
    return false;
}

static bool machine_guid_close_and_unlink_temporary(
    int fd, const char *tmp_filename, int saved_errno, const char *reason) {
    if (close(fd) != 0)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "MACHINE_GUID: cannot close the temporary GUID file '%s' after %s",
               tmp_filename, reason);

    return machine_guid_unlink_temporary(tmp_filename, saved_errno, reason);
}

static bool machine_guid_rename_and_sync(const char *directory, const char *tmp_filename, const char *filename) {
#if defined(OS_WINDOWS)
    UNUSED(directory);

    wchar_t *windows_tmp = os_translate_msys_to_windows_pathW(tmp_filename);
    wchar_t *windows_final = os_translate_msys_to_windows_pathW(filename);
    if (!windows_tmp || !windows_final) {
        freez(windows_tmp);
        freez(windows_final);
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "MACHINE_GUID: cannot translate GUID publication paths for Windows");
        errno = EINVAL;
        return false;
    }

    bool moved = MoveFileExW(windows_tmp, windows_final, MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH);
    DWORD error = moved ? ERROR_SUCCESS : GetLastError();
    freez(windows_tmp);
    freez(windows_final);

    if (!moved) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "MACHINE_GUID: cannot move temporary GUID file '%s' to '%s' (Windows error %lu)",
               tmp_filename, filename, (unsigned long)error);
        errno = EIO;
        return false;
    }

    return true;
#else
    if (rename(tmp_filename, filename) != 0) {
        int saved_errno = errno;
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "MACHINE_GUID: cannot rename temporary GUID file '%s' to '%s'",
               tmp_filename, filename);
        errno = saved_errno;
        return false;
    }

    int directory_flags = O_RDONLY | O_CLOEXEC;
#ifdef O_DIRECTORY
    directory_flags |= O_DIRECTORY;
#endif
    int directory_fd = open(directory, directory_flags);
    if (directory_fd == -1) {
        int saved_errno = errno;
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "MACHINE_GUID: cannot open GUID directory '%s' for synchronization", directory);
        errno = saved_errno;
        return false;
    }

    if (fsync(directory_fd) != 0) {
        int saved_errno = errno;
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "MACHINE_GUID: cannot synchronize GUID directory '%s'", directory);
        if (close(directory_fd) != 0)
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "MACHINE_GUID: cannot close GUID directory '%s' after synchronization failure", directory);
        errno = saved_errno;
        return false;
    }

    if (close(directory_fd) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "MACHINE_GUID: cannot close GUID directory '%s' after synchronization", directory);
        return false;
    }

    return true;
#endif
}

static bool machine_guid_write_to_file(const char *directory, const char *filename, ND_MACHINE_GUID *host_id) {
    if (!directory || !*directory || !filename || !*filename)
        return false;

    ND_MACHINE_GUID h;
    memset(&h, 0, sizeof(h));

    if (host_id) {
        h.uuid = host_id->uuid;
        h.last_modified_ut = host_id->last_modified_ut;
        safecpy(h.last_modified_ut_rfc3339, host_id->last_modified_ut_rfc3339);
    }

    // Create the text representation before writing.
    uuid_unparse_lower(h.uuid.uuid, h.txt);

    // Use an exclusive temporary file so concurrent processes and stale files cannot collide.
    char tmp_filename[FILENAME_MAX];
    int rc = snprintf(tmp_filename, sizeof(tmp_filename), "%s.tmp.XXXXXX", filename);
    if (rc < 0 || (size_t)rc >= sizeof(tmp_filename)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "MACHINE_GUID: temporary GUID filename for '%s' is too long", filename);
        errno = ENAMETOOLONG;
        return false;
    }

    int fd = mkstemp(tmp_filename);
    if (fd == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot create the temporary GUID file '%s'", tmp_filename);
        return false;
    }

    size_t written = 0;
    while (written < sizeof(h.txt) - 1) {
        ssize_t bytes = write(fd, &h.txt[written], sizeof(h.txt) - 1 - written);
        if (bytes > 0) {
            written += (size_t)bytes;
            continue;
        }

        if (bytes < 0 && errno == EINTR)
            continue;

        if (bytes == 0)
            errno = EIO;

        int saved_errno = errno;
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot write GUID to the temporary GUID file '%s'", tmp_filename);
        return machine_guid_close_and_unlink_temporary(fd, tmp_filename, saved_errno, "write failure");
    }

    // Keep the established daemon permissions without changing the process-wide umask.
    if (fchmod(fd, 0440) != 0) {
        int saved_errno = errno;
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot set permissions on temporary GUID file '%s'", tmp_filename);
        return machine_guid_close_and_unlink_temporary(fd, tmp_filename, saved_errno, "permission failure");
    }

    struct timespec times[2] = {
        usec_to_timespec(h.last_modified_ut), // access time
        usec_to_timespec(h.last_modified_ut), // modification time
    };

    if (utimensat(AT_FDCWD, tmp_filename, times, 0) < 0)
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot update the timestamps of the temporary GUID file '%s'", tmp_filename);

    if (fsync(fd) != 0) {
        int saved_errno = errno;
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot synchronize temporary GUID file '%s'", tmp_filename);
        return machine_guid_close_and_unlink_temporary(fd, tmp_filename, saved_errno, "synchronization failure");
    }

    if (close(fd) != 0) {
        int saved_errno = errno;
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot close the temporary GUID file '%s'", tmp_filename);
        return machine_guid_unlink_temporary(tmp_filename, saved_errno, "close failure");
    }

    if (!machine_guid_rename_and_sync(directory, tmp_filename, filename)) {
        int saved_errno = errno;
        return machine_guid_unlink_temporary(tmp_filename, saved_errno, "publication failure");
    }

    nd_log(NDLS_DAEMON, NDLP_INFO, "MACHINE_GUID: GUID saved to file '%s'", filename);

    return true;
}

static ND_MACHINE_GUID nd_machine_guid = { 0 };
static bool nd_machine_guid_available = false;
static SPINLOCK nd_machine_guid_spinlock = SPINLOCK_INITIALIZER;

static ND_MACHINE_GUID machine_guid_get_or_create(void) {
    char pathname[FILENAME_MAX];
    char filename[FILENAME_MAX];
    char lock_filename[FILENAME_MAX];

    netdata_conf_section_directories();

    ND_MACHINE_GUID h;
    memset(&h, 0, sizeof(h));

    bool paths_valid = true;
    int rc = snprintf(pathname, sizeof(pathname), "%s/registry", netdata_configured_varlib_dir);
    if (rc < 0 || (size_t)rc >= sizeof(pathname)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: registry directory path is too long");
        paths_valid = false;
    }

    if (paths_valid) {
        rc = snprintf(filename, sizeof(filename), "%s/%s", pathname, "netdata.public.unique.id");
        if (rc < 0 || (size_t)rc >= sizeof(filename)) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: GUID filename is too long");
            paths_valid = false;
        }
    }

    // Attempt to read the GUID from the file.
    if (paths_valid && machine_guid_read_from_file(filename, &h, true))
        return h;

    // Log failure to read the file.
    if (paths_valid)
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: failed to read GUID from file '%s'", filename);

    // Attempt to retrieve GUID from daemon status file.
    h = daemon_status_file_get_host_id();
    uuid_unparse_lower(h.uuid.uuid, h.txt);
    if (UUIDiszero(h.uuid) || machine_guid_check_blacklisted(h.txt)) {
        // If the status file does not contain a GUID, generate a new one.
        nd_log(NDLS_DAEMON, NDLP_INFO, "MACHINE_GUID: generating a new GUID");
        uuid_generate(h.uuid.uuid);
        uuid_unparse_lower(h.uuid.uuid, h.txt);
    }
    else
        nd_log(NDLS_DAEMON, NDLP_INFO, "MACHINE_GUID: got previous GUID from daemon status file");

    h.last_modified_ut = now_realtime_usec();
    rfc3339_datetime_ut(h.last_modified_ut_rfc3339, sizeof(h.last_modified_ut_rfc3339), h.last_modified_ut, 2, true);
    nd_machine_guid = h;

    if (!paths_valid)
        return h;

    // Avoid a check-then-create race; mkdir() + EEXIST is sufficient.
    errno_clear();
    if (mkdir(pathname, 0775) != 0) {
        if (errno != EEXIST) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot create directory '%s'", pathname);
            // Even if directory creation fails, continue with in-memory GUID.
            return h;
        }

        // EEXIST — verify it is actually a directory.
        struct stat st;
        if (stat(pathname, &st) != 0 || !S_ISDIR(st.st_mode)) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "MACHINE_GUID: path '%s' exists but is not a directory", pathname);
            return h;
        }
    }

    // The lock file must persist so every contender continues to lock the same inode.
    rc = snprintf(lock_filename, sizeof(lock_filename), "%s.lock", filename);
    if (rc < 0 || (size_t)rc >= sizeof(lock_filename)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: lock filename for '%s' is too long", filename);
        return h;
    }

    FILE_LOCK lock = file_lock_get_wait(lock_filename);
    if (!FILE_LOCK_OK(lock)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot acquire publication lock '%s'", lock_filename);
        return h;
    }

    ND_MACHINE_GUID published;
    if (machine_guid_read_from_file(filename, &published, false)) {
        file_lock_release(lock);
        return published;
    }

    errno_clear();
    if (!machine_guid_write_to_file(pathname, filename, &h))
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot save GUID to file '%s'", filename);

    file_lock_release(lock);

    return h;
}

ND_MACHINE_GUID *machine_guid_get(void) {
    if(unlikely(!__atomic_load_n(&nd_machine_guid_available, __ATOMIC_ACQUIRE))) {
        spinlock_lock(&nd_machine_guid_spinlock);

        if(unlikely(!__atomic_load_n(&nd_machine_guid_available, __ATOMIC_ACQUIRE))) {
            nd_machine_guid = machine_guid_get_or_create();
            nd_setenv("NETDATA_REGISTRY_UNIQUE_ID", nd_machine_guid.txt, 1);
            __atomic_store_n(&nd_machine_guid_available, true, __ATOMIC_RELEASE);
        }

        spinlock_unlock(&nd_machine_guid_spinlock);
    }

    return &nd_machine_guid;
}

const char *machine_guid_get_txt(void) {
    ND_MACHINE_GUID *h = machine_guid_get();
    return h->txt;
}
