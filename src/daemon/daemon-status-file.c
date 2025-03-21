// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "daemon-status-file.h"
#include "buildinfo.h"

#include <curl/curl.h>
#include <openssl/evp.h>
#include <openssl/pem.h>
#include <openssl/err.h>

#define REPORT_EVENTS_EVERY (86400 - 3600) // -1 hour to tolerate cron randomness

#ifdef ENABLE_SENTRY
#include "sentry-native/sentry-native.h"
#endif

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
};

static DAEMON_STATUS_FILE session_status = {
    .v = STATUS_FILE_VERSION,
    .spinlock = SPINLOCK_INITIALIZER,
    .fatal = {
        .spinlock = SPINLOCK_INITIALIZER,
    },
};

static void daemon_status_file_out_of_memory(void);

// these are used instead of locks when locks cannot be used (signal handler, out of memory, etc)
#define dsf_acquire(ds) __atomic_load_n(&(ds).v, __ATOMIC_ACQUIRE)
#define dsf_release(ds) __atomic_store_n(&(ds).v, (ds).v, __ATOMIC_RELEASE)

static void copy_and_clean_thread_name_if_empty(DAEMON_STATUS_FILE *ds, const char *name) {
    if(ds->fatal.thread[0] && strcmp(ds->fatal.thread, "NO_NAME") != 0)
        return;

    if(!name || !*name) name = "NO_NAME";

    strncpyz(ds->fatal.thread, name, sizeof(ds->fatal.thread) - 1);

    // remove the variable part from the thread by removing [XXX] from it
    unsigned char *p = (unsigned char *)strchr(ds->fatal.thread, '[');
    if(p && isdigit(p[1]) && (isdigit(p[2]) || p[2] == ']'))
        *p = '\0';
}

#define STACK_TRACE_INFO_PREFIX "info: "
static bool stack_trace_is_empty(DAEMON_STATUS_FILE *ds) {
    return !ds->fatal.stack_trace[0] || strncmp(ds->fatal.stack_trace, STACK_TRACE_INFO_PREFIX, strlen(STACK_TRACE_INFO_PREFIX)) == 0;
}

static void set_stack_trace_message_if_empty(DAEMON_STATUS_FILE *ds, const char *msg) {
    if(stack_trace_is_empty(ds))
        strncpyz(ds->fatal.stack_trace, msg, sizeof(ds->fatal.stack_trace) - 1);
}

// --------------------------------------------------------------------------------------------------------------------
// json generation

static uint64_t daemon_status_file_hash(DAEMON_STATUS_FILE *ds, const char *msg, const char *cause) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    struct {
        uint32_t v;
        DAEMON_STATUS status;
        SIGNAL_CODE signal_code;
        ND_PROFILE profile;
        EXIT_REASON exit_reason;
        RRD_DB_MODE db_mode;
        uint8_t db_tiers;
        bool kubernetes;
        bool sentry_available;
        bool sentry_fatal;
        ND_UUID host_id;
        ND_UUID machine_id;
        long line;
        char version[sizeof(ds->version)];
        char filename[sizeof(ds->fatal.filename)];
        char function[sizeof(ds->fatal.function)];
        char stack_trace[sizeof(ds->fatal.stack_trace)];
        char thread[sizeof(ds->fatal.thread)];
        char msg[128];
        char cause[32];
    } to_hash;

    // this is important to remove any random bytes from the structure
    memset(&to_hash, 0, sizeof(to_hash));

    dsf_acquire(*ds);

    to_hash.v = ds->v,
    to_hash.status = ds->status,
    to_hash.signal_code = ds->fatal.signal_code,
    to_hash.profile = ds->profile,
    to_hash.exit_reason = ds->exit_reason,
    to_hash.db_mode = ds->db_mode,
    to_hash.db_tiers = ds->db_tiers,
    to_hash.kubernetes = ds->kubernetes,
    to_hash.sentry_available = ds->sentry_available,
    to_hash.sentry_fatal = ds->fatal.sentry,
    to_hash.host_id = ds->host_id,
    to_hash.machine_id = ds->machine_id,

    strncpyz(to_hash.version, ds->version, sizeof(to_hash.version) - 1);
    strncpyz(to_hash.filename, ds->fatal.filename, sizeof(to_hash.filename) - 1);
    strncpyz(to_hash.filename, ds->fatal.function, sizeof(to_hash.function) - 1);
    strncpyz(to_hash.stack_trace, ds->fatal.stack_trace, sizeof(to_hash.stack_trace) - 1);
    strncpyz(to_hash.thread, ds->fatal.thread, sizeof(to_hash.thread) - 1);

    if(msg)
        strncpyz(to_hash.msg, msg, sizeof(to_hash.msg) - 1);

    if(cause)
        strncpyz(to_hash.cause, cause, sizeof(to_hash.cause) - 1);

    uint64_t hash = fnv1a_hash_bin64(&to_hash, sizeof(to_hash));

    dsf_release(*ds);
    return hash;
}

static void daemon_status_file_to_json(BUFFER *wb, DAEMON_STATUS_FILE *ds) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    dsf_acquire(*ds);

    buffer_json_member_add_datetime_rfc3339(wb, "@timestamp", ds->timestamp_ut, true);
    buffer_json_member_add_uint64(wb, "version", STATUS_FILE_VERSION);

    buffer_json_member_add_object(wb, "agent");
    {
        buffer_json_member_add_uuid(wb, "id", ds->host_id.uuid);
        buffer_json_member_add_uuid_compact(wb, "ephemeral_id", ds->invocation.uuid);
        buffer_json_member_add_string(wb, "version", ds->version);

        buffer_json_member_add_time_t(wb, "uptime", ds->uptime);

        buffer_json_member_add_uuid(wb, "node_id", ds->node_id.uuid);
        buffer_json_member_add_uuid(wb, "claim_id", ds->claim_id.uuid);
        buffer_json_member_add_uint64(wb, "restarts", ds->restarts);

        ND_PROFILE_2json(wb, "profile", ds->profile);
        buffer_json_member_add_string(wb, "status", DAEMON_STATUS_2str(ds->status));
        EXIT_REASON_2json(wb, "exit_reason", ds->exit_reason);

        buffer_json_member_add_string_or_empty(wb, "install_type", ds->install_type);

        if(ds->v >= 14) {
            buffer_json_member_add_string(wb, "db_mode", rrd_memory_mode_name(ds->db_mode));
            buffer_json_member_add_uint64(wb, "db_tiers", ds->db_tiers);
            buffer_json_member_add_boolean(wb, "kubernetes", ds->kubernetes);
        }

        if(ds->v >= 16)
            buffer_json_member_add_boolean(wb, "sentry_available", ds->sentry_available);

        if(ds->v >= 18) {
            buffer_json_member_add_int64(wb, "reliability", ds->reliability);
            buffer_json_member_add_string(wb, "stack_traces", ds->stack_traces);
        }

        buffer_json_member_add_object(wb, "timings");
        {
            buffer_json_member_add_time_t(wb, "init", ds->timings.init);
            buffer_json_member_add_time_t(wb, "exit", ds->timings.exit);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "host");
    {
        buffer_json_member_add_uuid_compact(wb, "id", ds->machine_id.uuid);
        buffer_json_member_add_string_or_empty(wb, "architecture", ds->architecture);
        buffer_json_member_add_string_or_empty(wb, "virtualization", ds->virtualization);
        buffer_json_member_add_string_or_empty(wb, "container", ds->container);
        buffer_json_member_add_time_t(wb, "uptime", ds->boottime);

        if(ds->v >= 20) {
            buffer_json_member_add_string_or_empty(wb, "timezone", ds->timezone);
            buffer_json_member_add_string_or_empty(wb, "cloud_provider", ds->cloud_provider_type);
            buffer_json_member_add_string_or_empty(wb, "cloud_instance", ds->cloud_instance_type);
            buffer_json_member_add_string_or_empty(wb, "cloud_region", ds->cloud_instance_region);
        }

        buffer_json_member_add_object(wb, "boot");
        {
            buffer_json_member_add_uuid_compact(wb, "id", ds->boot_id.uuid);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "memory");
        if(OS_SYSTEM_MEMORY_OK(ds->memory)) {
            buffer_json_member_add_uint64(wb, "total", ds->memory.ram_total_bytes);
            buffer_json_member_add_uint64(wb, "free", ds->memory.ram_available_bytes);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "disk");
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

    buffer_json_member_add_object(wb, "os");
    {
        buffer_json_member_add_string(wb, "type", DAEMON_OS_TYPE_2str(ds->os_type));
        buffer_json_member_add_string_or_empty(wb, "kernel", ds->kernel_version);
        buffer_json_member_add_string_or_empty(wb, "name", ds->os_name);
        buffer_json_member_add_string_or_empty(wb, "version", ds->os_version);
        buffer_json_member_add_string_or_empty(wb, "family", ds->os_id);
        buffer_json_member_add_string_or_empty(wb, "platform", ds->os_id_like);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "fatal");
    {
        buffer_json_member_add_uint64(wb, "line", ds->fatal.line);
        buffer_json_member_add_string_or_empty(wb, "filename", ds->fatal.filename);
        buffer_json_member_add_string_or_empty(wb, "function", ds->fatal.function);
        buffer_json_member_add_string_or_empty(wb, "message", ds->fatal.message);
        buffer_json_member_add_string_or_empty(wb, "errno", ds->fatal.errno_str);
        buffer_json_member_add_string_or_empty(wb, "thread", ds->fatal.thread);
        buffer_json_member_add_uint64(wb, "thread_id", ds->fatal.thread_id);
        buffer_json_member_add_string_or_empty(wb, "stack_trace", ds->fatal.stack_trace);

        if(ds->v >= 16) {
            char signal_code[UINT64_MAX_LENGTH];
            SIGNAL_CODE_2str_h(ds->fatal.signal_code, signal_code, sizeof(signal_code));
            buffer_json_member_add_string_or_empty(wb, "signal_code", signal_code);
        }

        if(ds->v >= 17)
            buffer_json_member_add_boolean(wb, "sentry", ds->fatal.sentry);

        if(ds->v >= 18) {
            char buf[UINT64_HEX_MAX_LENGTH];
            print_uint64_hex(buf, ds->fatal.fault_address);
            buffer_json_member_add_string(wb, "fault_address", buf);
        }
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_array(wb, "dedup");
    {
        for(size_t i = 0; i < _countof(ds->dedup.slot); i++) {
            if (ds->dedup.slot[i].timestamp_ut == 0)
                continue;

            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_datetime_rfc3339(wb, "@timestamp", ds->dedup.slot[i].timestamp_ut, true);
                buffer_json_member_add_uint64(wb, "hash", ds->dedup.slot[i].hash);
                buffer_json_member_add_boolean(wb, "sentry", ds->dedup.slot[i].sentry);
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
    bool required_v10 = version >= 10 ? strict : false;
    bool required_v14 = version >= 14 ? strict : false;
    bool required_v16 = version >= 16 ? strict : false;
    bool required_v17 = version >= 17 ? strict : false;
    bool required_v18 = version >= 18 ? strict : false;
    bool required_v20 = version >= 20 ? strict : false;

    // Parse timestamp
    JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "@timestamp", datetime, error, required_v1);
    if(datetime[0])
        ds->timestamp_ut = rfc3339_parse_ut(datetime, NULL);

    const char *profile_key = version >= 18 ? "profile" : "ND_profile";
    const char *status_key = version >= 18 ? "status" : "ND_status";
    const char *exit_reason_key = version >= 18 ? "exit_reason" : "ND_exit_reason";
    const char *node_id_key = version >= 18 ? "node_id" : "ND_node_id";
    const char *claim_id_key = version >= 18 ? "claim_id" : "ND_claim_id";
    const char *install_type_key = version >= 18 ? "install_type" : "ND_install_type";
    const char *timings_key = version >= 18 ? "timings" : "ND_timings";
    const char *restarts_key = version >= 18 ? "restarts" : "ND_restarts";
    const char *db_mode_key = version >= 18 ? "db_mode" : "ND_db_mode";
    const char *db_tiers_key = version >= 18 ? "db_tiers" : "ND_db_tiers";
    const char *kubernetes_key = version >= 18 ? "kubernetes" : "ND_kubernetes";
    const char *sentry_available_key = version >= 18 ? "sentry_available" : "ND_sentry_available";

    // Parse agent object
    JSONC_PARSE_SUBOBJECT(jobj, path, "agent", error, required_v1, {
        JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "id", ds->host_id.uuid, error, required_v1);
        JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "ephemeral_id", ds->invocation.uuid, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "version", ds->version, error, required_v1);
        JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "uptime", ds->uptime, error, required_v1);

        JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, profile_key, ND_PROFILE_2id_one, ds->profile, error, required_v1);
        JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, status_key, DAEMON_STATUS_2id, ds->status, error, required_v1);
        JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, exit_reason_key, EXIT_REASON_2id_one, ds->exit_reason, error, required_v1);
        JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, node_id_key, ds->node_id.uuid, error, required_v1);
        JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, claim_id_key, ds->claim_id.uuid, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, install_type_key, ds->install_type, error, required_v3);

        JSONC_PARSE_SUBOBJECT(jobj, path, timings_key, error, required_v1, {
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "init", ds->timings.init, error, required_v1);
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "exit", ds->timings.exit, error, required_v1);
        });

        if(version >= 4)
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, restarts_key, ds->restarts, error, required_v4);

        if(version >= 14) {
            JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, db_mode_key, rrd_memory_mode_id, ds->db_mode, error, required_v14);
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, db_tiers_key, ds->db_tiers, error, required_v14);
            JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, kubernetes_key, ds->kubernetes, error, required_v14);
        }
        else {
            ds->db_mode = default_rrd_memory_mode;
            ds->db_tiers = nd_profile.storage_tiers;
            ds->kubernetes = false;
        }

        if(version >= 17)
            JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, sentry_available_key, ds->sentry_available, error, required_v17);
        else if(version == 16)
            JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, "ND_sentry", ds->sentry_available, error, required_v16);
        if(version >= 18) {
            JSONC_PARSE_INT64_OR_ERROR_AND_RETURN(jobj, path, "reliability", ds->reliability, error, required_v18);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "stack_traces", ds->stack_traces, error, required_v18);

            char buf[UINT64_HEX_MAX_LENGTH];
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "fault_address", buf, error, required_v18);
            ds->fatal.fault_address = str2ull_encoded(buf);
        }
    });

    // Parse host object
    JSONC_PARSE_SUBOBJECT(jobj, path, "host", error, required_v1, {
        JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "id", ds->machine_id.uuid, error, required_v10);
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

        if(version >= 20) {
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "timezone", ds->timezone, error, required_v20);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "cloud_provider", ds->cloud_provider_type, error, required_v20);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "cloud_instance", ds->cloud_instance_type, error, required_v20);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "cloud_region", ds->cloud_instance_region, error, required_v20);
        }
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

        if(version >= 16)
            JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "signal_code", SIGNAL_CODE_2id_h, ds->fatal.signal_code, error, required_v16);

        if(version >= 17)
            JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "sentry", SIGNAL_CODE_2id_h, ds->fatal.sentry, error, required_v17);

        if(version >= 18)
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "thread_id", ds->fatal.thread_id, error, required_v18);
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

                    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "hash", ds->dedup.slot[i].hash, error, required_v4);
                    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, "sentry", ds->dedup.slot[i].sentry, error, required_v17);
                }
            });
        });
    }

    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// get the current status

static void daemon_status_file_migrate_once(void) {
    FUNCTION_RUN_ONCE();

    dsf_acquire(last_session_status);
    dsf_acquire(session_status);

    strncpyz(session_status.version, NETDATA_VERSION, sizeof(session_status.version) - 1);
    session_status.machine_id = os_machine_id();

    {
        char *install_type = NULL, *prebuilt_arch = NULL, *prebuilt_dist = NULL;
        get_install_type_internal(&install_type, &prebuilt_arch, &prebuilt_dist);

        if(install_type)
            strncpyz(session_status.install_type, install_type, sizeof(session_status.install_type) - 1);

        freez(prebuilt_arch);
        freez(prebuilt_dist);
        freez(install_type);
    }

#if defined(ENABLE_SENTRY)
    session_status.sentry_available = true;
#else
    session_status.sentry_available = false;
#endif

    session_status.boot_id = os_boot_id();
    if(!UUIDeq(session_status.boot_id, last_session_status.boot_id) && os_boot_ids_match(session_status.boot_id, last_session_status.boot_id)) {
        // there is a slight difference in boot_id, but it is still the same boot
        // copy the last boot_id
        session_status.boot_id = last_session_status.boot_id;
    }

    session_status.claim_id = last_session_status.claim_id;
    session_status.node_id = last_session_status.node_id;
    session_status.host_id = last_session_status.host_id;
    if(UUIDiszero(session_status.host_id)) {
        const char *machine_guid = registry_get_this_machine_guid(false);
        if(machine_guid && *machine_guid) {
            if (uuid_parse_flexi(machine_guid, session_status.host_id.uuid) != 0)
                session_status.host_id = UUID_ZERO;
        }
    }

    strncpyz(session_status.architecture, last_session_status.architecture, sizeof(session_status.architecture) - 1);
    strncpyz(session_status.virtualization, last_session_status.virtualization, sizeof(session_status.virtualization) - 1);
    strncpyz(session_status.container, last_session_status.container, sizeof(session_status.container) - 1);
    strncpyz(session_status.kernel_version, last_session_status.kernel_version, sizeof(session_status.kernel_version) - 1);
    strncpyz(session_status.os_name, last_session_status.os_name, sizeof(session_status.os_name) - 1);
    strncpyz(session_status.os_version, last_session_status.os_version, sizeof(session_status.os_version) - 1);
    strncpyz(session_status.os_id, last_session_status.os_id, sizeof(session_status.os_id) - 1);
    strncpyz(session_status.os_id_like, last_session_status.os_id_like, sizeof(session_status.os_id_like) - 1);
    strncpyz(session_status.timezone, last_session_status.timezone, sizeof(session_status.timezone) - 1);
    strncpyz(session_status.cloud_provider_type, last_session_status.cloud_provider_type, sizeof(session_status.cloud_provider_type) - 1);
    strncpyz(session_status.cloud_instance_type, last_session_status.cloud_instance_type, sizeof(session_status.cloud_instance_type) - 1);
    strncpyz(session_status.cloud_instance_region, last_session_status.cloud_instance_region, sizeof(session_status.cloud_instance_region) - 1);

    session_status.restarts = last_session_status.restarts + 1;
    session_status.reliability = last_session_status.reliability;

    if(daemon_status_file_has_last_crashed())  {
        if(session_status.reliability > 0) session_status.reliability = 0;
        session_status.reliability--;
    }
    else {
        if(session_status.reliability < 0) session_status.reliability = 0;
        session_status.reliability++;
    }

    if(last_session_status.v == STATUS_FILE_VERSION) {
        for (size_t i = 0; i < _countof(session_status.dedup.slot); i++)
            session_status.dedup.slot[i] = last_session_status.dedup.slot[i];
    }

    strncpyz(session_status.stack_traces, capture_stack_trace_backend(), sizeof(session_status.stack_traces) - 1);

    dsf_release(last_session_status);
    dsf_release(session_status);
}

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

    session_status.boottime = now_boottime_sec();
    session_status.uptime = now_realtime_sec() - netdata_start_time;
    session_status.timestamp_ut = now_ut;
    session_status.invocation = nd_log_get_invocation_id();
    session_status.db_mode = default_rrd_memory_mode;
    session_status.db_tiers = nd_profile.storage_tiers;

    session_status.claim_id = claim_id_get_uuid();

    if(localhost) {
        if(!UUIDiszero(localhost->host_id))
            session_status.host_id = localhost->host_id;

        if(!UUIDiszero(localhost->node_id))
            session_status.node_id = localhost->node_id;
    }

    get_daemon_status_fields_from_system_info(&session_status);

    if(netdata_configured_timezone)
        strncpyz(session_status.timezone, netdata_configured_timezone, sizeof(session_status.timezone) - 1);

    session_status.exit_reason = exit_initiated_get();
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
    CACHE_DIR,
    "/tmp",
    "/run",
    "/var/run",
    ".",
};

static void set_dynamic_fallbacks(void) {
    status_file_fallbacks[0] = netdata_configured_cache_dir;
}

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
    ssize_t read_bytes = fread(wb->buffer, 1, file_size, fp);
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
    if(check_status_file(netdata_configured_varlib_dir, current_filename, sizeof(current_filename), &current_mtime)) {
        strncpyz(newest_filename, current_filename, sizeof(newest_filename) - 1);
        newest_mtime = current_mtime;
    }

    // Check each fallback location
    set_dynamic_fallbacks();
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
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    // Linux: https://man7.org/linux/man-pages/man7/signal-safety.7.html
    // memcpy(), strlen(), open(), write(), fsync(), close(), fchmod(), rename(), unlink()

    // MacOS: https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/sigaction.2.html#//apple_ref/doc/man/2/sigaction
    // open(), write(), fsync(), close(), rename(), unlink()
    // does not explicitly mention fchmod, memcpy(), and strlen(), but they are safe

    if(!directory || !*directory)
        return false;

    static uint64_t tmp_attempt_counter = 0;

    char filename[FILENAME_MAX];
    char temp_filename[FILENAME_MAX];
    char tid_str[UINT64_MAX_LENGTH];

    print_uint64(tid_str, __atomic_add_fetch(&tmp_attempt_counter, 1, __ATOMIC_RELAXED));
    size_t dir_len = strlen(directory);
    size_t fil_len = strlen(STATUS_FILENAME);
    size_t tid_len = strlen(tid_str);

    if (dir_len + 1 + fil_len + 1 + tid_len + 1 >= sizeof(filename))
        return false; // cannot fit the filename

    // create the filename
    size_t pos = 0;
    memcpy(&filename[pos], directory, dir_len); pos += dir_len;
    filename[pos] = '/'; pos++;
    memcpy(&filename[pos], STATUS_FILENAME, fil_len); pos += fil_len;
    filename[pos] = '\0';

    // create the temp filename
    memcpy(temp_filename, filename, pos);
    temp_filename[pos] = '-'; pos++;
    memcpy(&temp_filename[pos], tid_str, tid_len); pos += tid_len;
    temp_filename[pos] = '\0';

    // Open file with O_WRONLY, O_CREAT, and O_TRUNC flags
    int fd = open(temp_filename, O_WRONLY | O_CREAT | O_TRUNC, 0664);
    if (fd == -1)
        return false;

    /* Write content to file using write() */
    size_t total_written = 0;

    while (total_written < content_size) {
        ssize_t bytes_written = write(fd, content + total_written, content_size - total_written);

        if (bytes_written <= 0) {
            if (errno == EINTR)
                continue; /* Retry if interrupted by signal */

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

    /* Set permissions using chmod() */
    if (fchmod(fd, 0664) != 0) {
        close(fd);
        unlink(temp_filename);
        return false;
    }

    /* Close file */
    if (close(fd) == -1) {
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

static void remove_old_status_files(const char *protected_dir) {
    FUNCTION_RUN_ONCE();
    
    char filename[FILENAME_MAX];

    set_dynamic_fallbacks();
    for(size_t i = 0; i < _countof(status_file_fallbacks); i++) {
        if(strcmp(status_file_fallbacks[i], protected_dir) == 0)
            continue;

        snprintfz(filename, sizeof(filename), "%s/%s", status_file_fallbacks[i], STATUS_FILENAME);
        unlink(filename);
    }

    errno_clear();
}

static bool daemon_status_file_saved = false;
static void daemon_status_file_save(BUFFER *wb, DAEMON_STATUS_FILE *ds, bool log) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    // wb should have enough space to hold the JSON content, to avoid any allocations

    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    daemon_status_file_to_json(wb, ds);
    buffer_json_finalize(wb);

    const char *content = buffer_tostring(wb);
    size_t content_size = buffer_strlen(wb);

    // Try primary directory first
    bool saved = false;
    if (save_status_file(netdata_configured_varlib_dir, content, content_size)) {
        remove_old_status_files(netdata_configured_varlib_dir);
        saved = true;
    }
    else {
        if(log)
            nd_log(NDLS_DAEMON, NDLP_DEBUG, "Failed to save status file in primary directory %s",
                   netdata_configured_varlib_dir);

        // Try each fallback directory until successful
        set_dynamic_fallbacks();
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

    if (saved)
        daemon_status_file_saved = true;
}

// --------------------------------------------------------------------------------------------------------------------
// deduplication hashes management

static bool dedup_already_posted(DAEMON_STATUS_FILE *ds, uint64_t hash, bool sentry) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    usec_t now_ut = now_realtime_usec();

    for(size_t i = 0; i < _countof(ds->dedup.slot); i++) {
        if(ds->dedup.slot[i].timestamp_ut == 0)
            continue;

        if(hash == ds->dedup.slot[i].hash &&
            sentry == ds->dedup.slot[i].sentry &&
            now_ut - ds->dedup.slot[i].timestamp_ut < REPORT_EVENTS_EVERY * USEC_PER_SEC) {
            // we have already posted this crash
            return true;
        }
    }

    return false;
}

static void dedup_keep_hash(DAEMON_STATUS_FILE *ds, uint64_t hash, bool sentry) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    // find the same hash
    for(size_t i = 0; i < _countof(ds->dedup.slot); i++) {
        if(ds->dedup.slot[i].hash == hash && ds->dedup.slot[i].sentry == sentry) {
            ds->dedup.slot[i].hash = hash;
            ds->dedup.slot[i].sentry = sentry;
            ds->dedup.slot[i].timestamp_ut = now_realtime_usec();
            return;
        }
    }

    // find an empty slot
    for(size_t i = 0; i < _countof(ds->dedup.slot); i++) {
        if(!ds->dedup.slot[i].hash) {
            ds->dedup.slot[i].hash = hash;
            ds->dedup.slot[i].sentry = sentry;
            ds->dedup.slot[i].timestamp_ut = now_realtime_usec();
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
    ds->dedup.slot[store_at_slot].sentry = sentry;
    ds->dedup.slot[store_at_slot].timestamp_ut = now_realtime_usec();
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
    buffer_json_member_add_string(wb, "exit_cause", d->cause);
    buffer_json_member_add_string(wb, "message", d->msg);
    buffer_json_member_add_uint64(wb, "priority", d->priority);
    buffer_json_member_add_uint64(wb, "version_saved", d->status->v);
    buffer_json_member_add_string(wb, "agent_version_now", NETDATA_VERSION);
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
    curl_easy_setopt(curl, CURLOPT_TIMEOUT, 10L);

    CURLcode rc = curl_easy_perform(curl);
    if(rc == CURLE_OK) {
        uint64_t hash = daemon_status_file_hash(d->status, d->msg, d->cause);
        dedup_keep_hash(&session_status, hash, false);
        daemon_status_file_save(wb, &session_status, true);
    }

    curl_easy_cleanup(curl);
    curl_slist_free_all(headers);
}

// --------------------------------------------------------------------------------------------------------------------
// check last status on startup and post-crash report

struct log_priority {
    ND_LOG_FIELD_PRIORITY user;
    ND_LOG_FIELD_PRIORITY post;
};

struct log_priority PRI_ALL_NORMAL      = { NDLP_NOTICE, NDLP_DEBUG };
struct log_priority PRI_USER_SHOULD_FIX = { NDLP_WARNING, NDLP_INFO };
struct log_priority PRI_FATAL           = { NDLP_ERR, NDLP_ERR };
struct log_priority PRI_DEADLY_SIGNAL   = { NDLP_CRIT, NDLP_CRIT };
struct log_priority PRI_KILLED_HARD     = { NDLP_ERR, NDLP_WARNING };

static bool is_ci(void) {
    const char *ci = getenv("CI");
    return ci && *ci && strcasecmp(ci, "true") == 0;
}

enum crash_report_t {
    DSF_REPORT_DISABLED = 0,
    DSF_REPORT_ALL,
    DSF_REPORT_CRASHES,
};

static enum crash_report_t check_crash_reports_config(void) {
    bool default_enabled = analytics_check_enabled() ||
                           !UUIDiszero(session_status.node_id) || !UUIDiszero(last_session_status.node_id) ||
                           !UUIDiszero(session_status.claim_id) || !UUIDiszero(last_session_status.claim_id);

    const char *t = inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "crash reports", default_enabled ? "all" : "off");

    enum crash_report_t rc;
    if(!t || !*t)
        rc = default_enabled ? DSF_REPORT_ALL : DSF_REPORT_DISABLED;
    else if(strcmp(t, "all") == 0)
        rc = DSF_REPORT_ALL;
    else if(strcmp(t, "crashes") == 0)
        rc = DSF_REPORT_CRASHES;
    else
        rc = DSF_REPORT_DISABLED;

    return rc;
}

void daemon_status_file_init(void) {
    static_save_buffer_init();
    mallocz_register_out_of_memory_cb(daemon_status_file_out_of_memory);
    daemon_status_file_load(&last_session_status);
    daemon_status_file_migrate_once();
}

void daemon_status_file_check_crash(void) {
    struct log_priority pri = PRI_ALL_NORMAL;

    bool new_version = strcmp(last_session_status.version, session_status.version) != 0;
    bool this_is_a_crash = false;
    bool no_previous_status = false;
    bool dump_json = true;
    const char *msg = "", *cause = "";
    switch(last_session_status.status) {
        default:
        case DAEMON_STATUS_NONE:
            // probably a previous version of netdata was running
            cause = "no last status";
            msg = "No status found for the previous Netdata session (new Netdata, or older version)";
            no_previous_status = true;
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
                pri = PRI_DEADLY_SIGNAL;
                this_is_a_crash = true;
            }
            else if(last_session_status.exit_reason != EXIT_REASON_NONE &&
                     !is_exit_reason_normal(last_session_status.exit_reason)) {
                cause = "fatal and exit";
                msg = "Netdata was last stopped gracefully after it encountered a fatal error";
                pri = PRI_FATAL;
                this_is_a_crash = true;
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
                pri = PRI_DEADLY_SIGNAL;
                this_is_a_crash = true;
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
                pri = PRI_FATAL;
            }
            else {
                cause = "killed hard on start";
                msg = "Netdata was last killed/crashed while starting";
                pri = PRI_KILLED_HARD;
            }
            this_is_a_crash = true;
            break;

        case DAEMON_STATUS_EXITING:
            if(is_deadly_signal(last_session_status.exit_reason)) {
                cause = "deadly signal on exit";
                msg = "Netdata was last crashed while exiting after receiving a deadly signal";
                pri = PRI_DEADLY_SIGNAL;
            }
            else if(last_session_status.exit_reason & EXIT_REASON_SHUTDOWN_TIMEOUT) {
                cause = "exit timeout";
                msg = "Netdata was last killed because it couldn't shutdown on time";
                pri = PRI_FATAL;
            }
            else if(last_session_status.exit_reason != EXIT_REASON_NONE &&
                !is_exit_reason_normal(last_session_status.exit_reason)) {
                cause = "fatal on exit";
                msg = "Netdata was last killed/crashed while exiting after encountering an error";
                pri = PRI_FATAL;
            }
            else if(last_session_status.exit_reason & EXIT_REASON_SYSTEM_SHUTDOWN) {
                cause = "killed hard on shutdown";
                msg = "Netdata was last killed/crashed while exiting due to system shutdown";
                pri = PRI_KILLED_HARD;
            }
            else if(new_version || (last_session_status.exit_reason & EXIT_REASON_UPDATE)) {
                cause = "killed hard on update";
                msg = "Netdata was last killed/crashed while exiting to update to a new version";
                pri = PRI_KILLED_HARD;
            }
            else {
                cause = "killed hard on exit";
                msg = "Netdata was last killed/crashed while it was instructed to exit";
                pri = PRI_KILLED_HARD;
            }
            this_is_a_crash = true;
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
                pri = PRI_DEADLY_SIGNAL;
                this_is_a_crash = true;
            }
            else if (last_session_status.exit_reason != EXIT_REASON_NONE &&
                     !is_exit_reason_normal(last_session_status.exit_reason)) {
                cause = "killed fatal";
                msg = "Netdata was last crashed due to a fatal error";
                pri = PRI_FATAL;
            }
            else {
                cause = "killed hard";
                msg = "Netdata was last killed/crashed while operating normally";
                pri = PRI_KILLED_HARD;
                this_is_a_crash = true;
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

    enum crash_report_t r = check_crash_reports_config();
    if( // must be first for netdata.conf option to be used
        (r == DSF_REPORT_ALL || (this_is_a_crash && r == DSF_REPORT_CRASHES)) &&

        // we have a previous status, or we managed to save the current one
        (!no_previous_status || daemon_status_file_saved) &&

        // we are not running in CI
        (last_session_status.restarts >= 10 || !is_ci()) &&

        // we have not already reported this
        !dedup_already_posted(&session_status, daemon_status_file_hash(&last_session_status, msg, cause), false)

        ) {
        netdata_conf_ssl();

        if(no_previous_status) {
            last_session_status = session_status;
            strncpyz(last_session_status.fatal.function, "no_status", sizeof(last_session_status.fatal.function) - 1);
        }

        struct post_status_file_thread_data d = {
            .cause = cause,
            .msg = msg,
            .status = &last_session_status,
            .priority = pri.post,
        };

        post_status_file(&d);

        // MacOS crashes when starting under launchctl, when we create a thread to post the status file,
        // so we post the status file synchronously, with a timeout of 10 seconds.
    }
}

static void daemon_status_file_save_twice_if_we_can_get_stack_trace(BUFFER *wb, DAEMON_STATUS_FILE *ds, bool force) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    if(capture_stack_trace_available())
        set_stack_trace_message_if_empty(&session_status, STACK_TRACE_INFO_PREFIX "will now attempt to get stack trace - if you see this message, we couldn't get it.");
    else
        set_stack_trace_message_if_empty(&session_status, STACK_TRACE_INFO_PREFIX "no stack trace backend available");

    // save it without a stack trace to be sure we will have the event
    daemon_status_file_save(wb, ds, false);

    if(!stack_trace_is_empty(ds) && !force)
        return;

    buffer_flush(wb);
    capture_stack_trace(wb);

    if(buffer_strlen(wb) > 0) {
        strncpyz(
            ds->fatal.stack_trace,
            buffer_tostring(wb),
            sizeof(ds->fatal.stack_trace) - 1);

        daemon_status_file_save(wb, ds, false);
    }
}

// --------------------------------------------------------------------------------------------------------------------
// ng_log() hook for receiving fatal message information

void daemon_status_file_register_fatal(const char *filename, const char *function, const char *message, const char *errno_str, const char *stack_trace, long line) {
    FUNCTION_RUN_ONCE();

    dsf_acquire(session_status);
    spinlock_lock(&session_status.fatal.spinlock);

    exit_initiated_add(EXIT_REASON_FATAL);
    session_status.exit_reason |= EXIT_REASON_FATAL;

    if(!session_status.fatal.thread_id)
        session_status.fatal.thread_id = gettid_cached();

    copy_and_clean_thread_name_if_empty(&session_status, nd_thread_tag());

    if(filename && *filename)
        strncpyz(session_status.fatal.filename, filename, sizeof(session_status.fatal.filename) - 1);

    if(function && *function)
        strncpyz(session_status.fatal.function, function, sizeof(session_status.fatal.function) - 1);

    if(message && *message)
        strncpyz(session_status.fatal.message, message, sizeof(session_status.fatal.message) - 1);

    if(errno_str && *errno_str)
        strncpyz(session_status.fatal.errno_str, errno_str, sizeof(session_status.fatal.errno_str) - 1);

    if(stack_trace && *stack_trace && stack_trace_is_empty(&session_status))
        strncpyz(session_status.fatal.stack_trace, stack_trace, sizeof(session_status.fatal.stack_trace) - 1);

    if(line)
        session_status.fatal.line = line;

    spinlock_unlock(&session_status.fatal.spinlock);
    dsf_release(session_status);

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    daemon_status_file_save_twice_if_we_can_get_stack_trace(wb, &session_status, false);

    freez((void *)filename);
    freez((void *)function);
    freez((void *)message);
    freez((void *)errno_str);
    freez((void *)stack_trace);

#ifdef ENABLE_SENTRY
    nd_sentry_add_fatal_message_as_breadcrumb();
#endif
}

// --------------------------------------------------------------------------------------------------------------------

void daemon_status_file_update_status(DAEMON_STATUS status) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    daemon_status_file_refresh(status);
    daemon_status_file_save(wb, &session_status, true);
}

static void daemon_status_file_out_of_memory(void) {
    FUNCTION_RUN_ONCE();

    // DO NOT ALLOCATE IN THIS FUNCTION - WE DON'T HAVE ANY MEMORY AVAILABLE!

    // the buffer should already be allocated, so this should normally do nothing
    static_save_buffer_init();

    dsf_acquire(session_status);
    exit_initiated_add(EXIT_REASON_OUT_OF_MEMORY);
    session_status.exit_reason |= EXIT_REASON_OUT_OF_MEMORY;
    dsf_release(session_status);

    daemon_status_file_save_twice_if_we_can_get_stack_trace(static_save_buffer, &session_status, true);
}

bool daemon_status_file_deadly_signal_received(EXIT_REASON reason, SIGNAL_CODE code, void *fault_address, bool chained_handler) {
    FUNCTION_RUN_ONCE_RET(true);

    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    dsf_acquire(session_status);

    exit_initiated_add(reason);
    session_status.exit_reason |= reason;
    session_status.fatal.sentry = chained_handler;

    if(code)
        session_status.fatal.signal_code = code;

    if(fault_address)
        session_status.fatal.fault_address = (uintptr_t)fault_address;

    if(!session_status.fatal.thread_id)
        session_status.fatal.thread_id = gettid_cached();

    copy_and_clean_thread_name_if_empty(&session_status, nd_thread_tag_async_safe());

    dsf_release(session_status);

    // the buffer should already be allocated, so this should normally do nothing
    static_save_buffer_init();

    // deduplicate the crash for sentry
    bool duplicate = false;
    if(chained_handler) {
        uint64_t hash = daemon_status_file_hash(&session_status, NULL, NULL);
        duplicate = dedup_already_posted(&session_status, hash, true);
        if (!duplicate) {
            // save this hash, so that we won't post it again to sentry
            dedup_keep_hash(&session_status, hash, true);
        }
    }

    bool safe_to_get_stack_trace = reason != EXIT_REASON_SIGABRT || capture_stack_trace_is_async_signal_safe();
    bool get_stack_trace = capture_stack_trace_available() && safe_to_get_stack_trace && stack_trace_is_empty(&session_status);

    // save it
    if(get_stack_trace)
        daemon_status_file_save_twice_if_we_can_get_stack_trace(static_save_buffer, &session_status, true);
    else {
        if (!capture_stack_trace_available())
            set_stack_trace_message_if_empty(&session_status, STACK_TRACE_INFO_PREFIX "no stack trace backend available");
        else
            set_stack_trace_message_if_empty(&session_status, STACK_TRACE_INFO_PREFIX "not safe to get a stack trace for this signal using this backend");

        daemon_status_file_save(static_save_buffer, &session_status, false);
    }

    return duplicate;
}

// --------------------------------------------------------------------------------------------------------------------
// shutdown related functions

static SPINLOCK shutdown_timeout_spinlock = SPINLOCK_INITIALIZER;

void daemon_status_file_shutdown_timeout(void) {
    FUNCTION_RUN_ONCE();

    spinlock_lock(&shutdown_timeout_spinlock);

    dsf_acquire(session_status);
    exit_initiated_add(EXIT_REASON_SHUTDOWN_TIMEOUT);
    session_status.exit_reason |= EXIT_REASON_SHUTDOWN_TIMEOUT;
    dsf_release(session_status);

    strncpyz(session_status.fatal.function, "shutdown_timeout", sizeof(session_status.fatal.function) - 1);

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    daemon_status_file_save(wb, &session_status, false);

    // keep the spinlock locked, to prevent further steps updating the status
}

void daemon_status_file_shutdown_step(const char *step) {
    if(session_status.fatal.filename[0] || !spinlock_trylock(&shutdown_timeout_spinlock))
        // we have a fatal logged
        return;

    if(step != NULL)
        snprintfz(session_status.fatal.function, sizeof(session_status.fatal.function), "shutdown(%s)", step);
    else
        session_status.fatal.function[0] = '\0';

    daemon_status_file_update_status(DAEMON_STATUS_EXITING);

    spinlock_unlock(&shutdown_timeout_spinlock);
}

// --------------------------------------------------------------------------------------------------------------------

bool daemon_status_file_has_last_crashed(void) {
    return (last_session_status.status != DAEMON_STATUS_NONE && last_session_status.status != DAEMON_STATUS_EXITED) ||
           !is_exit_reason_normal(last_session_status.exit_reason);
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

    daemon_status_file_update_status(DAEMON_STATUS_INITIALIZING);
}

// --------------------------------------------------------------------------------------------------------------------
// public API to get values

const char *daemon_status_file_get_install_type(void) {
    return session_status.install_type;
}

const char *daemon_status_file_get_architecture(void) {
    return session_status.architecture;
}

const char *daemon_status_file_get_virtualization(void) {
    return session_status.virtualization;
}

const char *daemon_status_file_get_container(void) {
    return session_status.container;
}

const char *daemon_status_file_get_os_name(void) {
    return session_status.os_name;
}

const char *daemon_status_file_get_os_version(void) {
    return session_status.os_version;
}

const char *daemon_status_file_get_os_id(void) {
    return session_status.os_id;
}

const char *daemon_status_file_get_os_id_like(void) {
    return session_status.os_id_like;
}

const char *daemon_status_file_get_cloud_provider_type(void) {
    return session_status.cloud_provider_type;
}

const char *daemon_status_file_get_cloud_instance_type(void) {
    return session_status.cloud_instance_type;
}

const char *daemon_status_file_get_cloud_instance_region(void) {
    return session_status.cloud_instance_region;
}

const char *daemon_status_file_get_timezone(void) {
    return session_status.timezone;
}

const char *daemon_status_file_get_fatal_filename(void) {
    return session_status.fatal.filename;
}

const char *daemon_status_file_get_fatal_function(void) {
    return session_status.fatal.function;
}

const char *daemon_status_file_get_fatal_message(void) {
    return session_status.fatal.message;
}

const char *daemon_status_file_get_fatal_errno(void) {
    return session_status.fatal.errno_str;
}

const char *daemon_status_file_get_fatal_stack_trace(void) {
    return session_status.fatal.stack_trace;
}

const char *daemon_status_file_get_stack_trace_backend(void) {
    return session_status.stack_traces;
}

const char *daemon_status_file_get_fatal_thread(void) {
    return session_status.fatal.thread;
}

pid_t daemon_status_file_get_fatal_thread_id(void) {
    return session_status.fatal.thread_id;
}

long daemon_status_file_get_fatal_line(void) {
    return session_status.fatal.line;
}

DAEMON_STATUS daemon_status_file_get_status(void) {
    return session_status.status;
}

size_t daemon_status_file_get_restarts(void) {
    return session_status.restarts;
}

ssize_t daemon_status_file_get_reliability(void) {
    return session_status.reliability;
}
