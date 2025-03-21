// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "machine-guid.h"

static char *blacklisted[] = {
    "8a795b0c-2311-11e6-8563-000c295076a6",
    "4aed1458-1c3e-11e6-a53f-000c290fc8f5",
};

static bool machine_guid_check_blacklisted(const char *guid) {
    // these are machine GUIDs that have been included in distribution packages.
    // we blacklist them here, so that the next version of netdata will generate
    // new ones.

    for(size_t i = 0; i < _countof(blacklisted); i++) {
        if (!strcmp(guid, blacklisted[i])) {
            netdata_log_error("Blacklisted machine GUID '%s' found.", guid);
            return true;
        }
    }

    return false;
}

static bool machine_guid_read_from_file(const char *filename, ND_MACHINE_GUID *host_id) {
    if(!filename || !*filename)
        return false;

    ND_MACHINE_GUID h;
    memset(&h, 0, sizeof(h));

    int fd = open(filename, O_RDONLY | O_CLOEXEC);
    if(fd == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot open file '%s' for reading", filename);
        return false;
    }

    if(read(fd, h.txt, sizeof(h.txt) - 1) != sizeof(h.txt) - 1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot read file '%s'", filename);
        close(fd);
        return false;
    }

    if(uuid_parse(h.txt, h.uuid.uuid) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot parse GUID from file '%s'", filename);
        close(fd);
        return false;
    }

    if(UUIDiszero(h.uuid)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: GUID read from file '%s' is zero", filename);
        close(fd);
        return false;
    }

    struct stat st;
    if(fstat(fd, &st) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot stat file '%s'", filename);
        close(fd);
        return false;
    }
    close(fd);

    // recreate the text version of it, to always be lowercase
    uuid_unparse_lower(h.uuid.uuid, h.txt);

    if(machine_guid_check_blacklisted(h.txt))
        return false;

    // update its last modified timestamp
    h.last_modified_ut = st.st_mtim.tv_sec * USEC_PER_SEC + st.st_mtim.tv_nsec / 1000;
    *host_id = h;

    nd_log(NDLS_DAEMON, NDLP_INFO, "MACHINE_GUID: read from file '%s'", filename);

    return true;
}

static struct timespec usec_to_timespec(usec_t usec) {
    struct timespec ts;
    ts.tv_sec = usec / USEC_PER_SEC;
    ts.tv_nsec = (usec % USEC_PER_SEC) * 1000;
    return ts;
}

static bool machine_guid_write_to_file(const char *filename, ND_MACHINE_GUID *host_id) {
    if(!filename || !*filename)
        return false;

    ND_MACHINE_GUID h;
    memset(&h, 0, sizeof(h));

    if(host_id) {
        h.uuid = host_id->uuid;
        h.last_modified_ut = host_id->last_modified_ut;
    }

    int fd = open(filename, O_WRONLY|O_CREAT|O_TRUNC, 444);
    if(fd == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot open file '%s' for writing", filename);
        return false;
    }

    if(write(fd, h.txt, sizeof(h.txt) - 1) != sizeof(h.txt) - 1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot save file '%s'", filename);
        close(fd);
        return false;
    }

    close(fd);

    struct timespec times[2] = {
        usec_to_timespec(h.last_modified_ut), // access time
        usec_to_timespec(h.last_modified_ut), // modification time
    };

    // update its timestamps to reflect the last modified time
    if (utimensat(AT_FDCWD, filename, times, 0) < 0)
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot update the timestamps of '%s'", filename);

    nd_log(NDLS_DAEMON, NDLP_INFO, "MACHINE_GUID: saved to file '%s'", filename);

    return true;
}

static ND_MACHINE_GUID nd_machine_guid = { 0 };

static ND_MACHINE_GUID machine_guid_get_or_create(void) {
    char pathname[FILENAME_MAX];
    char filename[FILENAME_MAX];

    netdata_conf_section_directories();

    ND_MACHINE_GUID h;
    memset(&h, 0, sizeof(h));

    snprintfz(pathname, sizeof(pathname), "%s/registry", netdata_configured_varlib_dir);
    snprintfz(filename, sizeof(filename), "%s/%s", pathname, "netdata.public.unique.id");
    if(machine_guid_read_from_file(filename, &h))
        return h;

    // could not read the file
    nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: failed to read file '%s'", filename);

    // get the last one saved in the daemon status file
    h.uuid = daemon_status_file_get_host_id();
    if(UUIDiszero(h.uuid)) {
        // daemon status file does not have a machine guid
        // generate a new one
        nd_log(NDLS_DAEMON, NDLP_INFO, "MACHINE_GUID: generating a new one");
        uuid_generate_time(h.uuid.uuid);
    }
    else
        nd_log(NDLS_DAEMON, NDLP_INFO, "MACHINE_GUID: got previous one from daemon status file");

    uuid_unparse_lower(h.uuid.uuid, h.txt);
    h.last_modified_ut = now_realtime_usec();
    nd_machine_guid = h;

    if(access(pathname, W_OK) != 0) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "MACHINE_GUID: cannot access directory '%s'. Will try to create it.", pathname);

        errno_clear();
        if(mkdir(pathname, 0775) != 0 &&  errno != EEXIST) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot create directory '%s'", pathname);
            return h;
        }
    }

    if (!machine_guid_write_to_file(filename, &h))
        nd_log(NDLS_DAEMON, NDLP_ERR, "MACHINE_GUID: cannot save to file '%s'", filename);

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
