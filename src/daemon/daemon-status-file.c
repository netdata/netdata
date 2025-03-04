// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "daemon-status-file.h"
#include "buildinfo.h"

#include <curl/curl.h>
#include <openssl/evp.h>
#include <openssl/pem.h>
#include <openssl/err.h>

#define STATUS_FILE_VERSION 9

#define STATUS_FILENAME "status-netdata.json"

ENUM_STR_MAP_DEFINE(DAEMON_STATUS) = {
    { DAEMON_STATUS_NONE, "none"},
    { DAEMON_STATUS_INITIALIZING, "initializing"},
    { DAEMON_STATUS_RUNNING, "running"},
    { DAEMON_STATUS_EXITING, "exiting"},
    { DAEMON_STATUS_EXITED, "exited"},

    // terminator
    { 0, NULL },
};
ENUM_STR_DEFINE_FUNCTIONS(DAEMON_STATUS, DAEMON_STATUS_NONE, "none");

ENUM_STR_MAP_DEFINE(DAEMON_OS_TYPE) = {
    {DAEMON_OS_TYPE_UNKNOWN, "unknown"},
    {DAEMON_OS_TYPE_LINUX, "linux"},
    {DAEMON_OS_TYPE_FREEBSD, "freebsd"},
    {DAEMON_OS_TYPE_MACOS, "macos"},
    {DAEMON_OS_TYPE_WINDOWS, "windows"},

    // terminator
    { 0, NULL },
};
ENUM_STR_DEFINE_FUNCTIONS(DAEMON_OS_TYPE, DAEMON_OS_TYPE_UNKNOWN, "unknown");

static DAEMON_STATUS_FILE last_session_status = {
    .v = 0,
    .spinlock = SPINLOCK_INITIALIZER,
    .fatal = {
        .spinlock = SPINLOCK_INITIALIZER,
    },
    .dedup = {
        .spinlock = SPINLOCK_INITIALIZER,
    },
};

static DAEMON_STATUS_FILE session_status = {
    .v = STATUS_FILE_VERSION,
    .spinlock = SPINLOCK_INITIALIZER,
    .fatal = {
        .spinlock = SPINLOCK_INITIALIZER,
    },
    .dedup = {
        .spinlock = SPINLOCK_INITIALIZER,
    },
};

static void daemon_status_file_out_of_memory(void);

// these are used instead of locks when locks cannot be used (signal handler, out of memory, etc)
#define dsf_acquire(ds) __atomic_load_n(&(ds).v, __ATOMIC_ACQUIRE)
#define dsf_release(ds) __atomic_store_n(&(ds).v, (ds).v, __ATOMIC_RELEASE)

// --------------------------------------------------------------------------------------------------------------------
// json generation

static XXH64_hash_t daemon_status_file_hash(DAEMON_STATUS_FILE *ds, const char *msg, const char *cause) {
    dsf_acquire(*ds);
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_uint64(wb, "version", STATUS_FILE_VERSION);
    buffer_json_member_add_uint64(wb, "version_saved", ds->v);
    buffer_json_member_add_uuid(wb, "host_id", ds->host_id.uuid);
    buffer_json_member_add_uuid(wb, "node_id", ds->node_id.uuid);
    buffer_json_member_add_uuid(wb, "claim_id", ds->claim_id.uuid);
    buffer_json_member_add_string(wb, "agent_version", ds->version);
    buffer_json_member_add_uint64(wb, "fatal_line", ds->fatal.line);
    buffer_json_member_add_string_or_empty(wb, "fatal_filename", ds->fatal.filename);
    buffer_json_member_add_string_or_empty(wb, "fatal_errno", ds->fatal.errno_str);
    buffer_json_member_add_string_or_empty(wb, "fatal_function", ds->fatal.function);
    buffer_json_member_add_string_or_empty(wb, "fatal_stack_trace", ds->fatal.stack_trace);
    buffer_json_member_add_string(wb, "message", msg);
    buffer_json_member_add_string(wb, "cause", cause);
    buffer_json_member_add_string(wb, "status", DAEMON_STATUS_2str(ds->status));
    EXIT_REASON_2json(wb, "exit_reason", ds->exit_reason);
    ND_PROFILE_2json(wb, "profile", ds->profile);
    dsf_release(*ds);
    buffer_json_finalize(wb);
    XXH64_hash_t hash = XXH3_64bits((const void *)buffer_tostring(wb), buffer_strlen(wb));
    return hash;
}

static void daemon_status_file_to_json(BUFFER *wb, DAEMON_STATUS_FILE *ds) {
    dsf_acquire(*ds);

    buffer_json_member_add_datetime_rfc3339(wb, "@timestamp", ds->timestamp_ut, true); // ECS
    buffer_json_member_add_uint64(wb, "version", STATUS_FILE_VERSION); // custom

    buffer_json_member_add_object(wb, "agent"); // ECS
    {
        buffer_json_member_add_uuid(wb, "id", ds->host_id.uuid); // ECS
        buffer_json_member_add_uuid_compact(wb, "ephemeral_id", ds->invocation.uuid); // ECS
        buffer_json_member_add_string(wb, "version", ds->version); // ECS

        buffer_json_member_add_time_t(wb, "uptime", ds->uptime); // custom

        buffer_json_member_add_uuid(wb, "ND_node_id", ds->node_id.uuid); // custom
        buffer_json_member_add_uuid(wb, "ND_claim_id", ds->claim_id.uuid); // custom
        buffer_json_member_add_uint64(wb, "ND_restarts", ds->restarts); // custom

        ND_PROFILE_2json(wb, "ND_profile", ds->profile); // custom
        buffer_json_member_add_string(wb, "ND_status", DAEMON_STATUS_2str(ds->status)); // custom
        EXIT_REASON_2json(wb, "ND_exit_reason", ds->exit_reason); // custom

        buffer_json_member_add_string_or_empty(wb, "ND_install_type", ds->install_type); // custom

        buffer_json_member_add_object(wb, "ND_timings"); // custom
        {
            buffer_json_member_add_time_t(wb, "init", ds->timings.init);
            buffer_json_member_add_time_t(wb, "exit", ds->timings.exit);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "host"); // ECS
    {
        buffer_json_member_add_string_or_empty(wb, "architecture", ds->architecture); // ECS
        buffer_json_member_add_string_or_empty(wb, "virtualization", ds->virtualization); // custom
        buffer_json_member_add_string_or_empty(wb, "container", ds->container); // custom
        buffer_json_member_add_time_t(wb, "uptime", ds->boottime); // ECS

        buffer_json_member_add_object(wb, "boot"); // ECS
        {
            buffer_json_member_add_uuid(wb, "id", ds->boot_id.uuid); // ECS
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "memory"); // custom
        if(OS_SYSTEM_MEMORY_OK(ds->memory)) {
            buffer_json_member_add_uint64(wb, "total", ds->memory.ram_total_bytes);
            buffer_json_member_add_uint64(wb, "free", ds->memory.ram_available_bytes);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "disk"); // ECS
        {
            buffer_json_member_add_object(wb, "db");
            if (OS_SYSTEM_DISK_SPACE_OK(ds->var_cache)) {
                buffer_json_member_add_uint64(wb, "total", ds->var_cache.total_bytes);
                buffer_json_member_add_uint64(wb, "free", ds->var_cache.free_bytes);
                buffer_json_member_add_uint64(wb, "inodes_total", ds->var_cache.total_inodes);
                buffer_json_member_add_uint64(wb, "inodes_free", ds->var_cache.free_inodes);
                buffer_json_member_add_boolean(wb, "read_only", ds->var_cache.is_read_only);
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "os"); // ECS
    {
        buffer_json_member_add_string(wb, "type", DAEMON_OS_TYPE_2str(ds->os_type)); // ECS
        buffer_json_member_add_string_or_empty(wb, "kernel", ds->kernel_version); // ECS
        buffer_json_member_add_string_or_empty(wb, "name", ds->os_name); // ECS
        buffer_json_member_add_string_or_empty(wb, "version", ds->os_version); // ECS
        buffer_json_member_add_string_or_empty(wb, "family", ds->os_id); // ECS
        buffer_json_member_add_string_or_empty(wb, "platform", ds->os_id_like); // ECS
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "fatal"); // custom
    {
        buffer_json_member_add_uint64(wb, "line", ds->fatal.line);
        buffer_json_member_add_string_or_empty(wb, "filename", ds->fatal.filename);
        buffer_json_member_add_string_or_empty(wb, "function", ds->fatal.function);
        buffer_json_member_add_string_or_empty(wb, "message", ds->fatal.message);
        buffer_json_member_add_string_or_empty(wb, "errno", ds->fatal.errno_str);
        buffer_json_member_add_string_or_empty(wb, "thread", ds->fatal.thread);
        buffer_json_member_add_string_or_empty(wb, "stack_trace", ds->fatal.stack_trace);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_array(wb, "dedup"); // custom
    {
        for(size_t i = 0; i < _countof(ds->dedup.slot); i++) {
            if (ds->dedup.slot[i].timestamp_ut == 0)
                continue;

            buffer_json_add_array_item_object(wb); // custom
            {
                buffer_json_member_add_datetime_rfc3339(wb, "@timestamp", ds->dedup.slot[i].timestamp_ut, true); // custom
                buffer_json_member_add_uint64(wb, "hash", ds->dedup.slot[i].hash); // custom
            }
            buffer_json_object_close(wb);
        }
    }
    buffer_json_array_close(wb);

    dsf_release(*ds);
}

// --------------------------------------------------------------------------------------------------------------------
// json parsing

static bool daemon_status_file_from_json(json_object *jobj, void *data, BUFFER *error) {
    char path[1024]; path[0] = '\0';

    DAEMON_STATUS_FILE *ds = data;
    char datetime[RFC3339_MAX_LENGTH]; datetime[0] = '\0';

    // change management, version to know which fields to expect
    uint64_t version = 0;
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "version", version, error, true);
    ds->v = version;

    bool strict = false; // allow missing fields and values
    bool required_v1 = version >= 1 ? strict : false;
    bool required_v3 = version >= 3 ? strict : false;
    bool required_v4 = version >= 4 ? strict : false;
    bool required_v5 = version >= 5 ? strict : false;

    // Parse timestamp
    JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "@timestamp", datetime, error, required_v1);
    if(datetime[0])
        ds->timestamp_ut = rfc3339_parse_ut(datetime, NULL);

    // Parse agent object
    JSONC_PARSE_SUBOBJECT(jobj, path, "agent", error, required_v1, {
        JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "id", ds->host_id.uuid, error, required_v1);
        JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "ephemeral_id", ds->invocation.uuid, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "version", ds->version, error, required_v1);
        JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "uptime", ds->uptime, error, required_v1);
        JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, "ND_profile", ND_PROFILE_2id_one, ds->profile, error, required_v1);
        JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "ND_status", DAEMON_STATUS_2id, ds->status, error, required_v1);
        JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, "ND_exit_reason", EXIT_REASON_2id_one, ds->exit_reason, error, required_v1);
        JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "ND_node_id", ds->node_id.uuid, error, required_v1);
        JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "ND_claim_id", ds->claim_id.uuid, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "ND_install_type", ds->install_type, error, required_v3);

        JSONC_PARSE_SUBOBJECT(jobj, path, "ND_timings", error, required_v1, {
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "init", ds->timings.init, error, required_v1);
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "exit", ds->timings.exit, error, required_v1);
        });

        if(version >= 4)
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "ND_restarts", ds->restarts, error, required_v4);
    });

    // Parse host object
    JSONC_PARSE_SUBOBJECT(jobj, path, "host", error, required_v1, {
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "architecture", ds->architecture, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "virtualization", ds->virtualization, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "container", ds->container, error, required_v1);
        JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "uptime", ds->boottime, error, required_v1);

        JSONC_PARSE_SUBOBJECT(jobj, path, "boot", error, required_v1, {
            JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "id", ds->boot_id.uuid, error, required_v1);
        });

        JSONC_PARSE_SUBOBJECT(jobj, path, "memory", error, required_v1, {
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "total", ds->memory.ram_total_bytes, error, false);
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "free", ds->memory.ram_available_bytes, error, false);
            if(!OS_SYSTEM_MEMORY_OK(ds->memory))
                ds->memory = OS_SYSTEM_MEMORY_EMPTY;
        });

        JSONC_PARSE_SUBOBJECT(jobj, path, "disk", error, required_v1, {
            JSONC_PARSE_SUBOBJECT(jobj, path, "db", error, required_v1, {
                JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "total", ds->var_cache.total_bytes, error, false);
                JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "free", ds->var_cache.free_bytes, error, false);
                JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "inodes_total", ds->var_cache.total_inodes, error, false);
                JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "inodes_free", ds->var_cache.free_inodes, error, false);
                JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, "read_only", ds->var_cache.is_read_only, error, false);
                if(!OS_SYSTEM_DISK_SPACE_OK(ds->var_cache))
                    ds->var_cache = OS_SYSTEM_DISK_SPACE_EMPTY;
            });
        });
    });

    // Parse os object
    JSONC_PARSE_SUBOBJECT(jobj, path, "os", error, required_v1, {
        JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "type", DAEMON_OS_TYPE_2id, ds->os_type, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "kernel", ds->kernel_version, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "name", ds->os_name, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "version", ds->os_version, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "family", ds->os_id, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "platform", ds->os_id_like, error, required_v1);
    });

    // Parse fatal object
    JSONC_PARSE_SUBOBJECT(jobj, path, "fatal", error, required_v1, {
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "filename", ds->fatal.filename, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "function", ds->fatal.function, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "message", ds->fatal.message, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "stack_trace", ds->fatal.stack_trace, error, required_v1);
        JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "line", ds->fatal.line, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "errno", ds->fatal.errno_str, error, required_v3);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "thread", ds->fatal.thread, error, required_v5);
    });

    // Parse the last posted object
    if(version == 3) {
        JSONC_PARSE_SUBOBJECT(jobj, path, "dedup", error, required_v3, {
            datetime[0] = '\0';
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "@timestamp", datetime, error, required_v3);
            if (datetime[0])
                ds->dedup.slot[0].timestamp_ut = rfc3339_parse_ut(datetime, NULL);

            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "hash", ds->dedup.slot[0].hash, error, required_v3);
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "restarts", ds->restarts, error, required_v3);
        });
    }
    else if(version >= 4) {
        JSONC_PARSE_ARRAY(jobj, path, "dedup", error, required_v4, {
            size_t i = 0;
            JSONC_PARSE_ARRAY_ITEM_OBJECT(jobj, path, i, required_v4, {
                if(i < _countof(ds->dedup.slot)) {
                    datetime[0] = '\0';
                    JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "@timestamp", datetime, error, required_v4);
                    if (datetime[0])
                        ds->dedup.slot[i].timestamp_ut = rfc3339_parse_ut(datetime, NULL);

                    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(
                        jobj, path, "hash", ds->dedup.slot[i].hash, error, required_v4);
                }
            });
        });
    }

    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// get the current status

static void daemon_status_file_refresh(DAEMON_STATUS status) {
    usec_t now_ut = now_realtime_usec();

    dsf_acquire(session_status);
    spinlock_lock(&session_status.spinlock);

#if defined(OS_LINUX)
    session_status.os_type = DAEMON_OS_TYPE_LINUX;
#elif defined(OS_FREEBSD)
    session_status.os_type = DAEMON_OS_TYPE_FREEBSD;
#elif defined(OS_MACOS)
    session_status.os_type = DAEMON_OS_TYPE_MACOS;
#elif defined(OS_WINDOWS)
    session_status.os_type = DAEMON_OS_TYPE_WINDOWS;
#endif

    if(!session_status.timings.init_started_ut)
        session_status.timings.init_started_ut = now_ut;

    if(status == DAEMON_STATUS_EXITING && !session_status.timings.exit_started_ut)
        session_status.timings.exit_started_ut = now_ut;

    if(session_status.status == DAEMON_STATUS_INITIALIZING)
        session_status.timings.init = (time_t)((now_ut - session_status.timings.init_started_ut + USEC_PER_SEC/2) / USEC_PER_SEC);

    if(session_status.status == DAEMON_STATUS_EXITING)
        session_status.timings.exit = (time_t)((now_ut - session_status.timings.exit_started_ut + USEC_PER_SEC/2) / USEC_PER_SEC);

    strncpyz(session_status.version, NETDATA_VERSION, sizeof(session_status.version) - 1);

    session_status.boot_id = os_boot_id();
    if(!UUIDeq(session_status.boot_id, last_session_status.boot_id) && os_boot_ids_match(session_status.boot_id, last_session_status.boot_id)) {
        // there is a slight difference in boot_id, but it is still the same boot
        // copy the last boot_id
        session_status.boot_id = last_session_status.boot_id;
    }

    session_status.boottime = now_boottime_sec();
    session_status.uptime = now_realtime_sec() - netdata_start_time;
    session_status.timestamp_ut = now_ut;
    session_status.invocation = nd_log_get_invocation_id();

    session_status.claim_id = claim_id_get_uuid();

    if(localhost) {
        session_status.host_id = localhost->host_id;
        session_status.node_id = localhost->node_id;
    }
    else if(!UUIDiszero(last_session_status.host_id))
        session_status.host_id = last_session_status.host_id;
    else {
        const char *machine_guid = registry_get_this_machine_guid(false);
        if(machine_guid && *machine_guid) {
            if (uuid_parse_flexi(machine_guid, session_status.host_id.uuid) != 0)
                session_status.host_id = UUID_ZERO;
        }
        else
            session_status.host_id = UUID_ZERO;
    }

    // copy items from the old status if they are not set
    if(UUIDiszero(session_status.claim_id))
        session_status.claim_id = last_session_status.claim_id;
    if(UUIDiszero(session_status.node_id))
        session_status.node_id = last_session_status.node_id;
    if(UUIDiszero(session_status.host_id))
        session_status.host_id = last_session_status.host_id;
    if(!session_status.architecture[0] && last_session_status.architecture[0])
        strncpyz(session_status.architecture, last_session_status.architecture, sizeof(session_status.architecture) - 1);
    if(!session_status.virtualization[0] && last_session_status.virtualization[0])
        strncpyz(session_status.virtualization, last_session_status.virtualization, sizeof(session_status.virtualization) - 1);
    if(!session_status.container[0] && last_session_status.container[0])
        strncpyz(session_status.container, last_session_status.container, sizeof(session_status.container) - 1);
    if(!session_status.kernel_version[0] && last_session_status.kernel_version[0])
        strncpyz(session_status.kernel_version, last_session_status.kernel_version, sizeof(session_status.kernel_version) - 1);
    if(!session_status.os_name[0] && last_session_status.os_name[0])
        strncpyz(session_status.os_name, last_session_status.os_name, sizeof(session_status.os_name) - 1);
    if(!session_status.os_version[0] && last_session_status.os_version[0])
        strncpyz(session_status.os_version, last_session_status.os_version, sizeof(session_status.os_version) - 1);
    if(!session_status.os_id[0] && last_session_status.os_id[0])
        strncpyz(session_status.os_id, last_session_status.os_id, sizeof(session_status.os_id) - 1);
    if(!session_status.os_id_like[0] && last_session_status.os_id_like[0])
        strncpyz(session_status.os_id_like, last_session_status.os_id_like, sizeof(session_status.os_id_like) - 1);
    if(!session_status.restarts)
        session_status.restarts = last_session_status.restarts + 1;

    if(last_session_status.v == STATUS_FILE_VERSION) {
        if (!session_status.dedup.slot[0].timestamp_ut || !session_status.dedup.slot[0].hash) {
            for (size_t i = 0; i < _countof(session_status.dedup.slot); i++)
                session_status.dedup.slot[i] = last_session_status.dedup.slot[i];
        }
    }

    if(!session_status.install_type[0]) {
        char *install_type = NULL, *prebuilt_arch = NULL, *prebuilt_dist = NULL;
        get_install_type_internal(&install_type, &prebuilt_arch, &prebuilt_dist);

        if(install_type)
            strncpyz(session_status.install_type, install_type, sizeof(session_status.install_type) - 1);

        freez(prebuilt_arch);
        freez(prebuilt_dist);
        freez(install_type);
    }

    get_daemon_status_fields_from_system_info(&session_status);

    session_status.exit_reason = exit_initiated;
    session_status.profile = nd_profile_detect_and_configure(false);

    if(status != DAEMON_STATUS_NONE)
        session_status.status = status;

    session_status.memory = os_system_memory(true);
    session_status.var_cache = os_disk_space(netdata_configured_cache_dir);

    spinlock_unlock(&session_status.spinlock);
    dsf_release(session_status);
}

// --------------------------------------------------------------------------------------------------------------------
// file helpers

// List of fallback directories to try
static const char *status_file_fallbacks[] = {
    "/tmp",
    "/run",
    "/var/run",
};

static bool check_status_file(const char *directory, char *filename, size_t filename_size, time_t *mtime) {
    if(!directory || !*directory)
        return false;

    snprintfz(filename, filename_size, "%s/%s", directory, STATUS_FILENAME);

    // Get file metadata
    OS_FILE_METADATA metadata = os_get_file_metadata(filename);
    if (!OS_FILE_METADATA_OK(metadata)) {
        *mtime = 0;
        return false;
    }

    *mtime = metadata.modified_time;
    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// load a saved status

static bool load_status_file(const char *filename, DAEMON_STATUS_FILE *status) {
    FILE *fp = fopen(filename, "r");
    if (!fp)
        return false;

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    CLEAN_BUFFER *error = buffer_create(0, NULL);

    // Get file size
    fseek(fp, 0, SEEK_END);
    long file_size = ftell(fp);
    fseek(fp, 0, SEEK_SET);

    // Read the file
    buffer_need_bytes(wb, file_size + 1);
    size_t read_bytes = fread(wb->buffer, 1, file_size, fp);
    fclose(fp);

    if (read_bytes == 0)
        return false;

    wb->buffer[read_bytes] = '\0';
    wb->len = read_bytes;

    // Parse the JSON
    return json_parse_payload_or_error(wb, error, daemon_status_file_from_json, status) == HTTP_RESP_OK;
}

void daemon_status_file_load(DAEMON_STATUS_FILE *ds) {
    char newest_filename[FILENAME_MAX] = "";
    char current_filename[FILENAME_MAX];
    time_t newest_mtime = 0, current_mtime;

    // Check the primary directory first
    if(check_status_file(netdata_configured_cache_dir, current_filename, sizeof(current_filename), &current_mtime)) {
        strncpyz(newest_filename, current_filename, sizeof(newest_filename) - 1);
        newest_mtime = current_mtime;
    }

    // Check each fallback location
    for(size_t i = 0; i < _countof(status_file_fallbacks); i++) {
        if(check_status_file(status_file_fallbacks[i], current_filename, sizeof(current_filename), &current_mtime) &&
            (!*newest_filename || current_mtime > newest_mtime)) {
            strncpyz(newest_filename, current_filename, sizeof(newest_filename) - 1);
            newest_mtime = current_mtime;
        }
    }

    // Load the newest file found
    if(*newest_filename) {
        if(!load_status_file(newest_filename, ds))
            nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to load newest status file: %s", newest_filename);
    }
    else
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot find a status file in any location");
}

// --------------------------------------------------------------------------------------------------------------------
// save the current status

static bool save_status_file(const char *directory, const char *content, size_t content_size) {
    // THIS FUNCTION MUST USE ONLY ASYNC-SAFE OPERATIONS

    if(!directory || !*directory)
        return false;

    char filename[FILENAME_MAX];
    char temp_filename[FILENAME_MAX];

    /* Construct filenames using async-safe string operations */
    /* Using simple string concatenation instead of snprintf */
    size_t dir_len = strlen(directory);
    if (dir_len + 1 + strlen(STATUS_FILENAME) >= FILENAME_MAX)
        return false;  /* Path too long */

    memcpy(filename, directory, dir_len);
    filename[dir_len] = '/';
    memcpy(filename + dir_len + 1, STATUS_FILENAME, strlen(STATUS_FILENAME) + 1);

    /* Create a unique temp filename using thread id */
    unsigned int tid = (unsigned int)gettid_cached();
    char tid_str[16];
    char *tid_ptr = tid_str + sizeof(tid_str) - 1;
    *tid_ptr = '\0';

    unsigned int tid_copy = tid;
    do {
        tid_ptr--;
        *tid_ptr = "0123456789abcdef"[tid_copy & 0xf];
        tid_copy >>= 4;
    } while (tid_copy && tid_ptr > tid_str);

    size_t temp_name_len = dir_len + 1 + strlen(STATUS_FILENAME) + 1 + (sizeof(tid_str) - (tid_ptr - tid_str));
    if (temp_name_len >= FILENAME_MAX)
        return false;  /* Path too long */

    memcpy(temp_filename, directory, dir_len);
    temp_filename[dir_len] = '/';
    char *ptr = temp_filename + dir_len + 1;
    memcpy(ptr, STATUS_FILENAME, strlen(STATUS_FILENAME));
    ptr += strlen(STATUS_FILENAME);
    *ptr++ = '-';
    memcpy(ptr, tid_ptr, strlen(tid_ptr) + 1);

    /* Open file with O_WRONLY, O_CREAT, and O_TRUNC flags */
    int fd = open(temp_filename, O_WRONLY | O_CREAT | O_TRUNC, 0664);
    if (fd == -1)
        return false;

    /* Write content to file using write() */
    ssize_t bytes_written = 0;
    size_t total_written = 0;

    while (total_written < content_size) {
        bytes_written = write(fd, content + total_written, content_size - total_written);

        if (bytes_written == -1) {
            if (errno == EINTR)
                continue;  /* Retry if interrupted by signal */

            close(fd);
            unlink(temp_filename);  /* Remove the temp file */
            return false;
        }

        total_written += bytes_written;
    }

    /* Fsync to ensure data is written to disk */
    if (fsync(fd) == -1) {
        close(fd);
        unlink(temp_filename);
        return false;
    }

    /* Close file */
    if (close(fd) == -1) {
        unlink(temp_filename);
        return false;
    }

    /* Set permissions using chmod() */
    if (chmod(temp_filename, 0664) != 0) {
        unlink(temp_filename);
        return false;
    }

    /* Rename temp file to target file */
    if (rename(temp_filename, filename) != 0) {
        unlink(temp_filename);
        return false;
    }

    return true;
}

static BUFFER *static_save_buffer = NULL;
static void static_save_buffer_init(void) {
    if (!static_save_buffer)
        static_save_buffer = buffer_create(16384, NULL);

    buffer_flush(static_save_buffer);
}

static void daemon_status_file_save(BUFFER *wb, DAEMON_STATUS_FILE *ds, bool log) {
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    daemon_status_file_to_json(wb, ds);
    buffer_json_finalize(wb);

    const char *content = buffer_tostring(wb);
    size_t content_size = buffer_strlen(wb);

    // Try primary directory first
    bool saved = false;
    if (save_status_file(netdata_configured_cache_dir, content, content_size))
        saved = true;
    else {
        if(log)
            nd_log(NDLS_DAEMON, NDLP_DEBUG, "Failed to save status file in primary directory %s",
                   netdata_configured_cache_dir);

        // Try each fallback directory until successful
        for(size_t i = 0; i < _countof(status_file_fallbacks); i++) {
            if (save_status_file(status_file_fallbacks[i], content, content_size)) {
                if(log)
                    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Saved status file in fallback %s", status_file_fallbacks[i]);
                saved = true;
                break;
            }
        }
    }

    if (!saved && log)
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to save status file in any location");
}

// --------------------------------------------------------------------------------------------------------------------
// deduplication hashes management

static bool dedup_already_posted(DAEMON_STATUS_FILE *ds, XXH64_hash_t hash) {
    spinlock_lock(&ds->dedup.spinlock);

    usec_t now_ut = now_realtime_usec();

    for(size_t i = 0; i < _countof(ds->dedup.slot); i++) {
        if(ds->dedup.slot[i].timestamp_ut == 0)
            continue;

        if(hash == ds->dedup.slot[i].hash &&
            now_ut - ds->dedup.slot[i].timestamp_ut < 86400 * USEC_PER_SEC) {
            // we have already posted this crash
            spinlock_unlock(&ds->dedup.spinlock);
            return true;
        }
    }

    spinlock_unlock(&ds->dedup.spinlock);
    return false;
}

static void dedup_keep_hash(DAEMON_STATUS_FILE *ds, XXH64_hash_t hash) {
    spinlock_lock(&ds->dedup.spinlock);

    // find the same hash
    for(size_t i = 0; i < _countof(ds->dedup.slot); i++) {
        if(ds->dedup.slot[i].hash == hash) {
            ds->dedup.slot[i].hash = hash;
            ds->dedup.slot[i].timestamp_ut = now_realtime_usec();
            spinlock_unlock(&ds->dedup.spinlock);
            return;
        }
    }

    // find an empty slot
    for(size_t i = 0; i < _countof(ds->dedup.slot); i++) {
        if(!ds->dedup.slot[i].hash) {
            ds->dedup.slot[i].hash = hash;
            ds->dedup.slot[i].timestamp_ut = now_realtime_usec();
            spinlock_unlock(&ds->dedup.spinlock);
            return;
        }
    }

    // find the oldest slot
    size_t store_at_slot = 0;
    for(size_t i = 1; i < _countof(ds->dedup.slot); i++) {
        if(ds->dedup.slot[i].timestamp_ut < ds->dedup.slot[store_at_slot].timestamp_ut)
            store_at_slot = i;
    }

    ds->dedup.slot[store_at_slot].hash = hash;
    ds->dedup.slot[store_at_slot].timestamp_ut = now_realtime_usec();

    spinlock_unlock(&ds->dedup.spinlock);
}

// --------------------------------------------------------------------------------------------------------------------
// POST the last status to agent-events

struct post_status_file_thread_data {
    const char *cause;
    const char *msg;
    ND_LOG_FIELD_PRIORITY priority;
    DAEMON_STATUS_FILE *status;
};

void post_status_file(struct post_status_file_thread_data *d) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_string(wb, "exit_cause", d->cause); // custom
    buffer_json_member_add_string(wb, "message", d->msg); // ECS
    buffer_json_member_add_uint64(wb, "priority", d->priority); // custom
    buffer_json_member_add_uint64(wb, "version_saved", d->status->v); // custom
    daemon_status_file_to_json(wb, d->status);
    buffer_json_finalize(wb);

    const char *json_data = buffer_tostring(wb);

    CURL *curl = curl_easy_init();
    if(!curl)
        return;

    curl_easy_setopt(curl, CURLOPT_URL, "https://agent-events.netdata.cloud/agent-events");
    curl_easy_setopt(curl, CURLOPT_POST, 1L);
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, json_data);
    struct curl_slist *headers = NULL;
    headers = curl_slist_append(headers, "Content-Type: application/json");
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);

    CURLcode rc = curl_easy_perform(curl);
    if(rc == CURLE_OK) {
        XXH64_hash_t hash = daemon_status_file_hash(d->status, d->msg, d->cause);
        dedup_keep_hash(&session_status, hash);
        daemon_status_file_save(wb, &session_status, true);
    }

    curl_easy_cleanup(curl);
    curl_slist_free_all(headers);
}

void *post_status_file_thread(void *ptr) {
    struct post_status_file_thread_data *d = (struct post_status_file_thread_data *)ptr;
    post_status_file(d);
    freez((void *)d->cause);
    freez((void *)d->msg);
    freez(d);
    return NULL;
}

// --------------------------------------------------------------------------------------------------------------------
// check last status on startup and post-crash report

struct log_priority {
    ND_LOG_FIELD_PRIORITY user;
    ND_LOG_FIELD_PRIORITY post;
};

struct log_priority PRI_ALL_NORMAL          = { NDLP_NOTICE, NDLP_DEBUG };
struct log_priority PRI_USER_SHOULD_FIX     = { NDLP_WARNING, NDLP_INFO };
struct log_priority PRI_NETDATA_BUG         = { NDLP_CRIT, NDLP_ERR };
struct log_priority PRI_BAD_BUT_NO_REASON   = { NDLP_ERR, NDLP_WARNING };

void daemon_status_file_check_crash(void) {
    FUNCTION_RUN_ONCE();

    static_save_buffer_init();

    mallocz_register_out_of_memory_cb(daemon_status_file_out_of_memory);

    daemon_status_file_load(&last_session_status);
    daemon_status_file_update_status(DAEMON_STATUS_INITIALIZING);
    struct log_priority pri = PRI_ALL_NORMAL;

    bool new_version = strcmp(last_session_status.version, session_status.version) != 0;
    bool post_crash_report = false;
    bool disable_crash_report = false;
    bool dump_json = true;
    const char *msg, *cause;
    switch(last_session_status.status) {
        default:
        case DAEMON_STATUS_NONE:
            // probably a previous version of netdata was running
            cause = "no last status";
            msg = "No status found for the previous Netdata session";
            disable_crash_report = true;
            break;

        case DAEMON_STATUS_EXITED:
            if(last_session_status.exit_reason == EXIT_REASON_NONE) {
                cause = "exit no reason";
                msg = "Netdata was last stopped gracefully, without setting a reason";
                if(!last_session_status.timestamp_ut)
                    dump_json = false;
            }
            else if(is_deadly_signal(last_session_status.exit_reason)) {
                cause = "deadly signal and exit";
                msg = "Netdata was last stopped gracefully after receiving a deadly signal";
                pri = PRI_NETDATA_BUG;
                post_crash_report = true;
            }
            else if(last_session_status.exit_reason != EXIT_REASON_NONE &&
                     !is_exit_reason_normal(last_session_status.exit_reason)) {
                cause = "fatal and exit";
                msg = "Netdata was last stopped gracefully after it encountered a fatal error";
                pri = PRI_NETDATA_BUG;
                post_crash_report = true;
            }
            else if(last_session_status.exit_reason & EXIT_REASON_SYSTEM_SHUTDOWN) {
                cause = "exit on system shutdown";
                msg = "Netdata has gracefully stopped due to system shutdown";
            }
            else if(last_session_status.exit_reason & EXIT_REASON_UPDATE) {
                cause = "exit to update";
                msg = "Netdata has gracefully restarted to update to a new version";
            }
            else if(new_version) {
                cause = "exit and updated";
                msg = "Netdata has gracefully restarted and updated to a new version";
                last_session_status.exit_reason |= EXIT_REASON_UPDATE;
            }
            else {
                cause = "exit instructed";
                msg = "Netdata was last stopped gracefully";
            }
            break;

        case DAEMON_STATUS_INITIALIZING:
            if (last_session_status.exit_reason == EXIT_REASON_NONE &&
                     !UUIDiszero(session_status.boot_id) &&
                     !UUIDiszero(last_session_status.boot_id) &&
                     !os_boot_ids_match(session_status.boot_id, last_session_status.boot_id)) {
                cause = "abnormal power off";
                msg = "The system was abnormally powered off while Netdata was starting";
                pri = PRI_USER_SHOULD_FIX;
            }
            else if(is_deadly_signal(last_session_status.exit_reason)) {
                cause = "deadly signal on start";
                msg = "Netdata was last crashed while starting after receiving a deadly signal";
                pri = PRI_NETDATA_BUG;
                post_crash_report = true;
            }
            else if (last_session_status.exit_reason & EXIT_REASON_OUT_OF_MEMORY) {
                cause = "out of memory";
                msg = "Netdata was last crashed while starting, because it couldn't allocate memory";
                pri = PRI_USER_SHOULD_FIX;
            }
            else if (last_session_status.exit_reason & EXIT_REASON_ALREADY_RUNNING) {
                cause = "already running";
                msg = "Netdata couldn't start, because it was already running";
                pri = PRI_USER_SHOULD_FIX;
            }
            else if (OS_SYSTEM_DISK_SPACE_OK(last_session_status.var_cache) &&
                last_session_status.var_cache.is_read_only) {
                cause = "disk read-only";
                msg = "Netdata couldn't start because the disk is readonly";
                pri = PRI_USER_SHOULD_FIX;
            }
            else if (OS_SYSTEM_DISK_SPACE_OK(last_session_status.var_cache) &&
                last_session_status.var_cache.free_bytes == 0) {
                cause = "disk full";
                msg = "Netdata couldn't start because the disk is full";
                pri = PRI_USER_SHOULD_FIX;
            }
            else if (OS_SYSTEM_DISK_SPACE_OK(last_session_status.var_cache) &&
                     last_session_status.var_cache.free_bytes < 10 * 1024 * 1024) {
                cause = "disk almost full";
                msg = "Netdata couldn't start while the disk is almost full";
                pri = PRI_USER_SHOULD_FIX;
            }
            else if (last_session_status.exit_reason != EXIT_REASON_NONE &&
                     !is_exit_reason_normal(last_session_status.exit_reason)) {
                cause = "fatal on start";
                msg = "Netdata was last crashed while starting, because of a fatal error";
                pri = PRI_NETDATA_BUG;
            }
            else {
                cause = "killed hard on start";
                msg = "Netdata was last killed/crashed while starting";
                pri = PRI_BAD_BUT_NO_REASON;
            }
            post_crash_report = true;

            break;

        case DAEMON_STATUS_EXITING:
            if(is_deadly_signal(last_session_status.exit_reason)) {
                cause = "deadly signal on exit";
                msg = "Netdata was last crashed while exiting after receiving a deadly signal";
                pri = PRI_NETDATA_BUG;
                post_crash_report = true;
            }
            else if(last_session_status.exit_reason != EXIT_REASON_NONE &&
                !is_exit_reason_normal(last_session_status.exit_reason)) {
                cause = "fatal on exit";
                msg = "Netdata was last killed/crashed while exiting after encountering an error";
            }
            else if(last_session_status.exit_reason & EXIT_REASON_SYSTEM_SHUTDOWN) {
                cause = "killed hard on shutdown";
                msg = "Netdata was last killed/crashed while exiting due to system shutdown";
            }
            else if(new_version || (last_session_status.exit_reason & EXIT_REASON_UPDATE)) {
                cause = "killed hard on update";
                msg = "Netdata was last killed/crashed while exiting to update to a new version";
            }
            else {
                cause = "killed hard on exit";
                msg = "Netdata was last killed/crashed while it was instructed to exit";
            }
            pri = PRI_NETDATA_BUG;
            post_crash_report = true;
            break;

        case DAEMON_STATUS_RUNNING: {
            if (last_session_status.exit_reason == EXIT_REASON_NONE &&
                !UUIDiszero(session_status.boot_id) &&
                !UUIDiszero(last_session_status.boot_id) &&
                !os_boot_ids_match(session_status.boot_id, last_session_status.boot_id)) {
                cause = "abnormal power off";
                msg = "The system was abnormally powered off while Netdata was running";
                pri = PRI_USER_SHOULD_FIX;
            }
            else if (last_session_status.exit_reason & EXIT_REASON_OUT_OF_MEMORY) {
                cause = "out of memory";
                msg = "Netdata was last crashed because it couldn't allocate memory";
                pri = PRI_USER_SHOULD_FIX;
            }
            else if(is_deadly_signal(last_session_status.exit_reason)) {
                cause = "deadly signal";
                msg = "Netdata was last crashed after receiving a deadly signal";
                pri = PRI_NETDATA_BUG;
                post_crash_report = true;
            }
            else if (last_session_status.exit_reason != EXIT_REASON_NONE &&
                     !is_exit_reason_normal(last_session_status.exit_reason)) {
                cause = "killed fatal";
                msg = "Netdata was last crashed due to a fatal error";
                pri = PRI_NETDATA_BUG;
            }
            else {
                cause = "killed hard";
                msg = "Netdata was last killed/crashed while operating normally";
                pri = PRI_BAD_BUT_NO_REASON;
                post_crash_report = true;
            }
            break;
        }
    }

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    if(dump_json)
        daemon_status_file_to_json(wb, &last_session_status);
    buffer_json_finalize(wb);

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &netdata_startup_msgid),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_DAEMON, pri.user,
           "Netdata Agent version '%s' is starting...\n"
           "Last exit status: %s (%s):\n\n%s",
           NETDATA_VERSION, msg, cause, buffer_tostring(wb));

    // check if we have already posted this crash in the last 24 hours
    XXH64_hash_t hash = daemon_status_file_hash(&last_session_status, msg, cause);
    if(dedup_already_posted(&session_status, hash))
        disable_crash_report = true;

    if(!disable_crash_report && (analytics_check_enabled() || post_crash_report)) {
        netdata_conf_ssl();

        struct post_status_file_thread_data *d = calloc(1, sizeof(*d));
        d->cause = strdupz(cause);
        d->msg = strdupz(msg);
        d->status = &last_session_status;
        d->priority = pri.post;
        nd_thread_create("post_status_file", NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_DEFAULT, post_status_file_thread, d);
    }
}

// --------------------------------------------------------------------------------------------------------------------
// ng_log() hook for receiving fatal message information

void daemon_status_file_register_fatal(const char *filename, const char *function, const char *message, const char *errno_str, const char *stack_trace, long line) {
    FUNCTION_RUN_ONCE();

    dsf_acquire(session_status);
    spinlock_lock(&session_status.fatal.spinlock);

    exit_initiated_add(EXIT_REASON_FATAL);
    strncpyz(session_status.fatal.thread, nd_thread_tag(), sizeof(session_status.fatal.thread) - 1);

    if(!session_status.fatal.filename[0])
        strncpyz(session_status.fatal.filename, filename, sizeof(session_status.fatal.filename) - 1);

    if(!session_status.fatal.function[0])
        strncpyz(session_status.fatal.function, function, sizeof(session_status.fatal.function) - 1);

    if(!session_status.fatal.message[0])
        strncpyz(session_status.fatal.message, message, sizeof(session_status.fatal.message) - 1);

    if(!session_status.fatal.errno_str[0])
        strncpyz(session_status.fatal.errno_str, errno_str, sizeof(session_status.fatal.errno_str) - 1);

    if(!session_status.fatal.stack_trace[0])
        strncpyz(session_status.fatal.stack_trace, stack_trace, sizeof(session_status.fatal.stack_trace) - 1);

    if(!session_status.fatal.line)
        session_status.fatal.line = line;

    spinlock_unlock(&session_status.fatal.spinlock);
    dsf_release(session_status);

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    daemon_status_file_save(wb, &session_status, false);

    freez((void *)filename);
    freez((void *)function);
    freez((void *)message);
    freez((void *)errno_str);
    freez((void *)stack_trace);
}

// --------------------------------------------------------------------------------------------------------------------

void daemon_status_file_update_status(DAEMON_STATUS status) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    daemon_status_file_refresh(status);
    daemon_status_file_save(wb, &session_status, true);
}

static void daemon_status_file_out_of_memory(void) {
    FUNCTION_RUN_ONCE();

    // DO NOT LOCK OR ALLOCATE IN THIS FUNCTION - WE DON'T HAVE ANY MEMORY AVAILABLE - IT HAPPENED ALREADY!

    exit_initiated_add(EXIT_REASON_OUT_OF_MEMORY);

    // the buffer should already be allocated, so this should normally do nothing
    static_save_buffer_init();

    dsf_acquire(session_status);
    session_status.exit_reason = exit_initiated;
    dsf_release(session_status);

    daemon_status_file_save(static_save_buffer, &session_status, false);
}

void daemon_status_file_deadly_signal_received(EXIT_REASON reason) {
    FUNCTION_RUN_ONCE();

    // DO NOT LOCK OR ALLOCATE IN THIS FUNCTION - WE CRASHED ALREADY AND WE ARE INSIDE THE SIGNAL HANDLER!

    dsf_acquire(session_status);

    session_status.exit_reason |= reason;
    if(!session_status.fatal.thread[0])
        strncpyz(session_status.fatal.thread, nd_thread_tag(), sizeof(session_status.fatal.thread) - 1);

    dsf_release(session_status);

    // the buffer should already be allocated, so this should normally do nothing
    static_save_buffer_init();

    // save what we know already
    daemon_status_file_save(static_save_buffer, &session_status, false);

    if(!session_status.fatal.stack_trace[0]) {
        buffer_flush(static_save_buffer);
        capture_stack_trace(static_save_buffer);
        strncpyz(session_status.fatal.stack_trace, buffer_tostring(static_save_buffer), sizeof(session_status.fatal.stack_trace) - 1);
        daemon_status_file_save(static_save_buffer, &session_status, false);
    }
}

bool daemon_status_file_has_last_crashed(void) {
    return last_session_status.status != DAEMON_STATUS_EXITED || !is_exit_reason_normal(last_session_status.exit_reason);
}

bool daemon_status_file_was_incomplete_shutdown(void) {
    return last_session_status.status == DAEMON_STATUS_EXITING;
}

// --------------------------------------------------------------------------------------------------------------------
// startup and shutdown steps

void daemon_status_file_startup_step(const char *step) {
    if(session_status.fatal.filename[0])
        // we have a fatal logged
        return;

    if(step != NULL)
        strncpyz(session_status.fatal.function, step, sizeof(session_status.fatal.function) - 1);
    else
        session_status.fatal.function[0] = '\0';

    daemon_status_file_update_status(DAEMON_STATUS_NONE);
}

void daemon_status_file_shutdown_step(const char *step) {
    if(session_status.fatal.filename[0])
        // we have a fatal logged
        return;

    if(step != NULL)
        snprintfz(session_status.fatal.function, sizeof(session_status.fatal.function), "shutdown(%s)", step);
    else
        session_status.fatal.function[0] = '\0';

    daemon_status_file_update_status(DAEMON_STATUS_NONE);
}
