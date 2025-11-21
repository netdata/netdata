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

static bool machine_guid_read_from_file(const char *filename, ND_MACHINE_GUID *host_id) {
    if (!filename || !*filename)
        return false;

    ND_MACHINE_GUID h;
    memset(&h, 0, sizeof(h));

    int fd = open(filename, O_RDONLY | O_CLOEXEC);
    if (fd == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot open GUID file '%s' for reading", filename);
        return false;
    }

    if (read(fd, h.txt, sizeof(h.txt) - 1) != sizeof(h.txt) - 1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot read GUID file '%s'", filename);
        close(fd);
        return false;
    }
    h.txt[sizeof(h.txt) - 1] = '\0';

    if (uuid_parse(h.txt, h.uuid.uuid) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot parse GUID from file '%s'", filename);
        close(fd);
        return false;
    }

    if (UUIDiszero(h.uuid)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: GUID read from file '%s' is zero", filename);
        close(fd);
        return false;
    }

    struct stat st;
    if (fstat(fd, &st) != 0) {
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

static bool machine_guid_write_to_file(const char *filename, ND_MACHINE_GUID *host_id) {
    static size_t save_id = 0;

    if (!filename || !*filename)
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

    // Use a temporary filename for atomic writes.
    char tmp_filename[FILENAME_MAX];
    snprintf(tmp_filename, sizeof(tmp_filename), "%s.%zu", filename, __atomic_add_fetch(&save_id, 1, __ATOMIC_RELAXED));

    int fd = open(tmp_filename, O_WRONLY | O_CREAT | O_TRUNC, 0444);
    if (fd == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot create the temporary GUID file '%s'", tmp_filename);
        return false;
    }

    if (write(fd, h.txt, sizeof(h.txt) - 1) != sizeof(h.txt) - 1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot write GUID to the temporary GUID file '%s'", tmp_filename);
        close(fd);
        return false;
    }

    close(fd);

    struct timespec times[2] = {
        usec_to_timespec(h.last_modified_ut), // access time
        usec_to_timespec(h.last_modified_ut), // modification time
    };

    // Update file timestamps.
    if (utimensat(AT_FDCWD, tmp_filename, times, 0) < 0)
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot update the timestamps of the temporary GUID file '%s'", tmp_filename);

    // Rename the temporary file to the target filename atomically.
    if (rename(tmp_filename, filename) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot rename temporary GUID file '%s' to '%s'", tmp_filename, filename);
        unlink(tmp_filename);
        return false;
    }

    nd_log(NDLS_DAEMON, NDLP_INFO, "MACHINE_GUID: GUID saved to file '%s'", filename);

    return true;
}

static ND_MACHINE_GUID nd_machine_guid = { 0 };

static ND_MACHINE_GUID machine_guid_get_or_create(void) {
    char pathname[FILENAME_MAX];
    char filename[FILENAME_MAX];

    netdata_conf_section_directories();

    ND_MACHINE_GUID h;
    memset(&h, 0, sizeof(h));

    // Build the file path.
    snprintfz(pathname, sizeof(pathname), "%s/registry", netdata_configured_varlib_dir);
    snprintfz(filename, sizeof(filename), "%s/%s", pathname, "netdata.public.unique.id");

    // Attempt to read the GUID from the file.
    if (machine_guid_read_from_file(filename, &h))
        return h;

    // Log failure to read the file.
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

    // Ensure the registry directory exists.
    if (access(pathname, W_OK) != 0) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "MACHINE_GUID: cannot access directory '%s'. Attempting to create it.", pathname);

        errno_clear();
        if (mkdir(pathname, 0775) != 0 && errno != EEXIST) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot create directory '%s'", pathname);
            // Even if directory creation fails, continue with in-memory GUID.
            return h;
        }
    }

    errno_clear();
    if (!machine_guid_write_to_file(filename, &h))
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot save GUID to file '%s'", filename);

    return h;
}

ND_MACHINE_GUID *machine_guid_get(void) {
    if(UUIDiszero(nd_machine_guid.uuid)) {
        nd_machine_guid = machine_guid_get_or_create();
        nd_setenv("NETDATA_REGISTRY_UNIQUE_ID", nd_machine_guid.txt, 1);
    }

    return &nd_machine_guid;
}

const char *machine_guid_get_txt(void) {
    ND_MACHINE_GUID *h = machine_guid_get();
    return h->txt;
}
