// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "daemon-status-file.h"
#include "buildinfo.h"

#include <curl/curl.h>
#include <openssl/evp.h>
#include <openssl/pem.h>
#include <openssl/err.h>

#define STATUS_FILE_VERSION 3

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

static DAEMON_STATUS_FILE last_session_status = { 0 };
static DAEMON_STATUS_FILE session_status = { 0 };
static SPINLOCK dsf_spinlock = SPINLOCK_INITIALIZER;

// --------------------------------------------------------------------------------------------------------------------
// json generation

static XXH64_hash_t daemon_status_file_hash(DAEMON_STATUS_FILE *ds, const char *msg, const char *cause) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_uint64(wb, "version", STATUS_FILE_VERSION);
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
    buffer_json_finalize(wb);
    XXH64_hash_t hash = XXH3_64bits((const void *)buffer_tostring(wb), buffer_strlen(wb));
    return hash;
}

static void daemon_status_file_to_json(BUFFER *wb, DAEMON_STATUS_FILE *ds) {
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
        buffer_json_member_add_string_or_empty(wb, "stack_trace", ds->fatal.stack_trace);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "dedup"); // custom
    {
        buffer_json_member_add_datetime_rfc3339(wb, "@timestamp", ds->dedup.timestamp_ut, true); // custom
        buffer_json_member_add_uint64(wb, "hash", ds->dedup.hash); // custom
        buffer_json_member_add_uint64(wb, "restarts", ds->dedup.restarts); // custom
    }
    buffer_json_object_close(wb);
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

    bool strict = false; // allow missing fields and values
    bool required_v1 = version >= 1 ? strict : false;
    bool required_v3 = version >= 3 ? strict : false;

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
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "ND_install_type", ds->install_type, error, required_v3);

        JSONC_PARSE_SUBOBJECT(jobj, path, "ND_timings", error, required_v1, {
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "init", ds->timings.init, error, required_v1);
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "exit", ds->timings.exit, error, required_v1);
        });
    });

    // Parse host object
    JSONC_PARSE_SUBOBJECT(jobj, path, "host", error, required_v1, {
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "architecture", ds->architecture, error, required_v1);
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "virtualization", ds->virtualization, error, required_v1);
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "container", ds->container, error, required_v1);
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
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "kernel", ds->kernel_version, error, required_v1);
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "name", ds->os_name, error, required_v1);
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "version", ds->os_version, error, required_v1);
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "family", ds->os_id, error, required_v1);
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "platform", ds->os_id_like, error, required_v1);
    });

    // Parse fatal object
    JSONC_PARSE_SUBOBJECT(jobj, path, "fatal", error, required_v1, {
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "filename", ds->fatal.filename, error, required_v1);
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "function", ds->fatal.function, error, required_v1);
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "message", ds->fatal.message, error, required_v1);
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "errno", ds->fatal.errno_str, error, required_v3);
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "stack_trace", ds->fatal.stack_trace, error, required_v1);
        JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "line", ds->fatal.line, error, required_v1);
    });

    // Parse the last posted object
    JSONC_PARSE_SUBOBJECT(jobj, path, "dedup", error, required_v3, {
        datetime[0] = '\0';
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "@timestamp", datetime, error, required_v1);
        if(datetime[0])
            ds->dedup.timestamp_ut = rfc3339_parse_ut(datetime, NULL);

        JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "hash", ds->dedup.hash, error, required_v3);
        JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "restarts", ds->dedup.restarts, error, required_v3);
    });

    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// get the current status

static void daemon_status_file_refresh(DAEMON_STATUS status) {
    spinlock_lock(&dsf_spinlock);

    usec_t now_ut = now_realtime_usec();

#if defined(OS_LINUX)
    session_status.os_type = DAEMON_OS_TYPE_LINUX;
#elif defined(OS_FREEBSD)
    session_status.os_type = DAEMON_OS_TYPE_FREEBSD;
#elif defined(OS_MACOS)
    session_status.os_type = DAEMON_OS_TYPE_MACOS;
#elif defined(OS_WINDOWS)
    session_status.os_type = DAEMON_OS_TYPE_WINDOWS;
#endif

    if(session_status.status == DAEMON_STATUS_INITIALIZING && status == DAEMON_STATUS_RUNNING)
        session_status.timings.init = (time_t)((now_ut - session_status.timestamp_ut + USEC_PER_SEC/2) / USEC_PER_SEC);

    if(session_status.status == DAEMON_STATUS_EXITING && status == DAEMON_STATUS_EXITED)
        session_status.timings.exit = (time_t)((now_ut - session_status.timestamp_ut + USEC_PER_SEC/2) / USEC_PER_SEC);

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
    if(!session_status.architecture && last_session_status.architecture)
        session_status.architecture = strdupz(last_session_status.architecture);
    if(!session_status.virtualization && last_session_status.virtualization)
        session_status.virtualization = strdupz(last_session_status.virtualization);
    if(!session_status.container && last_session_status.container)
        session_status.container = strdupz(last_session_status.container);
    if(!session_status.kernel_version && last_session_status.kernel_version)
        session_status.kernel_version = strdupz(last_session_status.kernel_version);
    if(!session_status.os_name && last_session_status.os_name)
        session_status.os_name = strdupz(last_session_status.os_name);
    if(!session_status.os_version && last_session_status.os_version)
        session_status.os_version = strdupz(last_session_status.os_version);
    if(!session_status.os_id && last_session_status.os_id)
        session_status.os_id = strdupz(last_session_status.os_id);
    if(!session_status.os_id_like && last_session_status.os_id_like)
        session_status.os_id_like = strdupz(last_session_status.os_id_like);
    if(!session_status.dedup.restarts)
        session_status.dedup.restarts = last_session_status.dedup.restarts + 1;
    if(!session_status.dedup.timestamp_ut || !session_status.dedup.hash) {
        session_status.dedup.timestamp_ut = last_session_status.dedup.timestamp_ut;
        session_status.dedup.hash = last_session_status.dedup.hash;
    }

    if(!session_status.install_type) {
        char *install_type = NULL, *prebuilt_arch = NULL, *prebuilt_dist = NULL;
        get_install_type_internal(&install_type, &prebuilt_arch, &prebuilt_dist);
        freez(prebuilt_arch);
        freez(prebuilt_dist);
        session_status.install_type = install_type;
    }

    get_daemon_status_fields_from_system_info(&session_status);

    session_status.exit_reason = exit_initiated;
    session_status.profile = nd_profile_detect_and_configure(false);

    if(status != DAEMON_STATUS_NONE)
        session_status.status = status;

    session_status.memory = os_system_memory(true);
    session_status.var_cache = os_disk_space(netdata_configured_cache_dir);

    spinlock_unlock(&dsf_spinlock);
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
    if (!OS_FILE_METADATA_OK(metadata))
        return false;

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

DAEMON_STATUS_FILE daemon_status_file_load(void) {
    DAEMON_STATUS_FILE status = {0};
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
        if(!load_status_file(newest_filename, &status))
            nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to load newest status file: %s", newest_filename);
    }
    else
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot find a status file in any location");

    return status;
}

// --------------------------------------------------------------------------------------------------------------------
// save the current status

static bool save_status_file(const char *directory, const char *content, size_t content_size) {
    if(!directory || !*directory)
        return false;

    char filename[FILENAME_MAX];
    char temp_filename[FILENAME_MAX];

    snprintfz(filename, sizeof(filename), "%s/%s", directory, STATUS_FILENAME);
    snprintfz(temp_filename, sizeof(temp_filename), "%s/%s-%08x", directory, STATUS_FILENAME, (unsigned)gettid_cached());

    FILE *fp = fopen(temp_filename, "w");
    if (!fp)
        return false;

    bool ok = fwrite(content, 1, content_size, fp) == content_size;
    fclose(fp);

    if (!ok) {
        unlink(temp_filename);
        return false;
    }

    if (chmod(temp_filename, 0664) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot set permissions on status file '%s'", temp_filename);
        unlink(temp_filename);
        return false;
    }

    if (rename(temp_filename, filename) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot rename status file '%s' to '%s'", temp_filename, filename);
        unlink(temp_filename);
        return false;
    }

    return true;
}

static void daemon_status_file_save(DAEMON_STATUS_FILE *ds) {
    spinlock_lock(&dsf_spinlock);

    static BUFFER *wb = NULL;
    if (!wb)
        wb = buffer_create(16384, NULL);

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
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "Failed to save status file in primary directory %s",
               netdata_configured_cache_dir);

        // Try each fallback directory until successful
        for(size_t i = 0; i < _countof(status_file_fallbacks); i++) {
            if (save_status_file(status_file_fallbacks[i], content, content_size)) {
                nd_log(NDLS_DAEMON, NDLP_DEBUG, "Saved status file in fallback %s", status_file_fallbacks[i]);
                saved = true;
                break;
            }
        }
    }

    if (!saved)
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to save status file in any location");

    spinlock_unlock(&dsf_spinlock);
}

void daemon_status_file_update_status(DAEMON_STATUS status) {
    daemon_status_file_refresh(status);
    daemon_status_file_save(&session_status);
}

void daemon_status_file_exit_reason_save(EXIT_REASON reason) {
    spinlock_lock(&dsf_spinlock);
    session_status.exit_reason |= reason;
    spinlock_unlock(&dsf_spinlock);
    daemon_status_file_save(&session_status);
}

static void daemon_status_file_out_of_memory(void) {
    daemon_status_file_exit_reason_save(EXIT_REASON_OUT_OF_MEMORY);
}

// --------------------------------------------------------------------------------------------------------------------
// POST the last status to agent-events

struct post_status_file_thread_data {
    const char *cause;
    const char *msg;
    ND_LOG_FIELD_PRIORITY priority;
    DAEMON_STATUS_FILE status;
};

void post_status_file(struct post_status_file_thread_data *d) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_string(wb, "exit_cause", d->cause); // custom
    buffer_json_member_add_string(wb, "message", d->msg); // ECS
    buffer_json_member_add_uint64(wb, "priority", d->priority); // custom
    daemon_status_file_to_json(wb, &d->status);
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
        XXH64_hash_t hash = daemon_status_file_hash(&d->status, d->msg, d->cause);
        spinlock_lock(&dsf_spinlock);
        session_status.dedup.timestamp_ut = now_realtime_usec();
        session_status.dedup.hash = hash;
        spinlock_unlock(&dsf_spinlock);
        daemon_status_file_save(&session_status);
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

    mallocz_register_out_of_memory_cb(daemon_status_file_out_of_memory);

    last_session_status = daemon_status_file_load();
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
                msg = "Netdata was last stopped gracefully (no exit reason set)";
                if(!last_session_status.timestamp_ut)
                    dump_json = false;
            }
            else if(!is_exit_reason_normal(last_session_status.exit_reason)) {
                cause = "exit on fatal";
                msg = "Netdata was last stopped gracefully (encountered an error)";
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
                msg = "Netdata was last stopped gracefully (instructed to do so)";
            }
            break;

        case DAEMON_STATUS_INITIALIZING:
            if (OS_SYSTEM_DISK_SPACE_OK(last_session_status.var_cache) &&
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
                     last_session_status.var_cache.free_bytes < 1 * 1024 * 1024) {
                cause = "disk almost full";
                msg = "Netdata couldn't start while the disk is almost full";
                pri = PRI_USER_SHOULD_FIX;
            }
            else if (last_session_status.exit_reason == EXIT_REASON_NONE &&
                !UUIDiszero(session_status.boot_id) &&
                !UUIDiszero(last_session_status.boot_id) &&
                !UUIDeq(session_status.boot_id, last_session_status.boot_id)) {
                cause = "abnormal power off";
                msg = "The system was abnormally powered off while Netdata was starting";
                pri = PRI_USER_SHOULD_FIX;
            }
            else if (last_session_status.exit_reason & EXIT_REASON_OUT_OF_MEMORY) {
                cause = "out of memory";
                msg = "Netdata was last crashed while starting, because it couldn't allocate memory";
                pri = PRI_USER_SHOULD_FIX;
            }
            else if (!is_exit_reason_normal(last_session_status.exit_reason)) {
                cause = "killed fatal";
                msg = "Netdata was last crashed while starting, because of a fatal error";
                pri = PRI_NETDATA_BUG;
            }
            else {
                cause = "crashed on start";
                msg = "Netdata was last killed/crashed while starting";
                pri = PRI_BAD_BUT_NO_REASON;
            }
            post_crash_report = true;

            break;

        case DAEMON_STATUS_EXITING:
            if(!is_exit_reason_normal(last_session_status.exit_reason)) {
                cause = "crashed on fatal";
                msg = "Netdata was last killed/crashed while exiting after encountering an error";
            }
            else if(last_session_status.exit_reason & EXIT_REASON_SYSTEM_SHUTDOWN) {
                cause = "crashed on system shutdown";
                msg = "Netdata was last killed/crashed while exiting due to system shutdown";
            }
            else if(new_version || (last_session_status.exit_reason & EXIT_REASON_UPDATE)) {
                cause = "crashed on update";
                msg = "Netdata was last killed/crashed while exiting to update to a new version";
            }
            else {
                cause = "crashed on exit";
                msg = "Netdata was last killed/crashed while exiting (instructed to do so)";
            }
            pri = PRI_NETDATA_BUG;
            post_crash_report = true;
            break;

        case DAEMON_STATUS_RUNNING: {
            if (last_session_status.exit_reason == EXIT_REASON_NONE &&
                !UUIDiszero(session_status.boot_id) &&
                !UUIDiszero(last_session_status.boot_id) &&
                !UUIDeq(session_status.boot_id, last_session_status.boot_id)) {
                cause = "abnormal power off";
                msg = "The system was abnormally powered off while Netdata was running";
                pri = PRI_USER_SHOULD_FIX;
            }
            else if (last_session_status.exit_reason & EXIT_REASON_OUT_OF_MEMORY) {
                cause = "out of memory";
                msg = "Netdata was last crashed because it couldn't allocate memory";
                pri = PRI_USER_SHOULD_FIX;
            }
            else if (!is_exit_reason_normal(last_session_status.exit_reason)) {
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

    if(last_session_status.dedup.timestamp_ut && last_session_status.dedup.hash) {
        XXH64_hash_t hash = daemon_status_file_hash(&last_session_status, msg, cause);
        if(hash == last_session_status.dedup.hash &&
            now_realtime_usec() - last_session_status.dedup.timestamp_ut < 86400 * USEC_PER_SEC) {
            // we have already posted this crash
            disable_crash_report = true;
        }
    }

    if(!disable_crash_report && (analytics_check_enabled() || post_crash_report)) {
        netdata_conf_ssl();

        struct post_status_file_thread_data *d = calloc(1, sizeof(*d));
        d->cause = strdupz(cause);
        d->msg = strdupz(msg);
        d->status = last_session_status;
        d->priority = pri.post;
        nd_thread_create("post_status_file", NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_DEFAULT, post_status_file_thread, d);
    }
}

bool daemon_status_file_has_last_crashed(void) {
    return last_session_status.status != DAEMON_STATUS_EXITED || !is_exit_reason_normal(last_session_status.exit_reason);
}

bool daemon_status_file_was_incomplete_shutdown(void) {
    return last_session_status.status == DAEMON_STATUS_EXITING;
}

void daemon_status_file_startup_step(const char *step) {
    freez((char *)session_status.fatal.function);
    session_status.fatal.function = step ? strdupz(step) : NULL;
    if(step != NULL)
        daemon_status_file_update_status(DAEMON_STATUS_NONE);
}

// --------------------------------------------------------------------------------------------------------------------
// ng_log() hook for receiving fatal message information

void daemon_status_file_register_fatal(const char *filename, const char *function, const char *message, const char *errno_str, const char *stack_trace, long line) {
    spinlock_lock(&dsf_spinlock);

    // do not check the function, because it may have a startup step in it
    if(session_status.fatal.filename || session_status.fatal.message || session_status.fatal.errno_str || session_status.fatal.stack_trace) {
        spinlock_unlock(&dsf_spinlock);
        freez((void *)filename);
        freez((void *)function);
        freez((void *)message);
        freez((void *)errno_str);
        freez((void *)stack_trace);
        return;
    }

    session_status.fatal.filename = filename;
    freez((char *)session_status.fatal.function); // it may have a startup step
    session_status.fatal.function = function;
    session_status.fatal.message = message;
    session_status.fatal.errno_str = errno_str;
    session_status.fatal.stack_trace = stack_trace;
    session_status.fatal.line = line;

    spinlock_unlock(&dsf_spinlock);

    exit_initiated |= EXIT_REASON_FATAL;
    daemon_status_file_save(&session_status);
}
