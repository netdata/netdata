// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "status-file.h"
#include "buildinfo.h"

#include <curl/curl.h>
#include <openssl/evp.h>
#include <openssl/pem.h>
#include <openssl/err.h>
#include "status-file-dmi.h"
#include "status-file-product.h"

#ifdef ENABLE_SENTRY
#include "sentry-native/sentry-native.h"
#endif

#include "status-file-dedup.h"
#include "status-file-io.h"

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
    .fatal = {
        .spinlock = SPINLOCK_INITIALIZER,
    },
};

static DAEMON_STATUS_FILE session_status = {
    .v = STATUS_FILE_VERSION,
    .fatal = {
        .spinlock = SPINLOCK_INITIALIZER,
    },
};

static void daemon_status_file_out_of_memory(void);

static void copy_and_clean_thread_name_if_empty(DAEMON_STATUS_FILE *ds, const char *name) {
    if(ds->fatal.thread[0] && strcmp(ds->fatal.thread, "NO_NAME") != 0)
        return;

    if(!name || !*name) name = "NO_NAME";

    safecpy(ds->fatal.thread, name);

    // remove the variable part from the thread by removing [XXX] from it
    unsigned char *p = (unsigned char *)strchr(ds->fatal.thread, '[');
    if(p && isdigit(p[1]) && (isdigit(p[2]) || p[2] == ']'))
        *p = '\0';
}

static bool stack_trace_is_empty(DAEMON_STATUS_FILE *ds) {
    return !ds->fatal.stack_trace[0] || strncmp(ds->fatal.stack_trace, STACK_TRACE_INFO_PREFIX, strlen(STACK_TRACE_INFO_PREFIX)) == 0;
}

static void set_stack_trace_message_if_empty(DAEMON_STATUS_FILE *ds, const char *msg) {
    if(stack_trace_is_empty(ds))
        safecpy(ds->fatal.stack_trace, msg);
}

static void fill_dmi_info(DAEMON_STATUS_FILE *ds) {
    if(!ds) return;

    dmi_info_init(&ds->hw);
    os_dmi_info_get(&ds->hw);
    product_name_vendor_type(ds);
}

// --------------------------------------------------------------------------------------------------------------------
// json generation

static void daemon_status_file_to_json(BUFFER *wb, DAEMON_STATUS_FILE *ds) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

    dsf_acquire(*ds);

    buffer_json_member_add_string(wb, "@timestamp", ds->timestamp_ut_rfc3339);
    buffer_json_member_add_uint64(wb, "version", STATUS_FILE_VERSION);

    buffer_json_member_add_object(wb, "agent");
    {
        buffer_json_member_add_uuid(wb, "id", ds->host_id.uuid.uuid);

        if(ds->v >= 24 && ds->host_id.last_modified_ut)
            buffer_json_member_add_string(wb, "since", ds->host_id.last_modified_ut_rfc3339);

        buffer_json_member_add_uuid_compact(wb, "ephemeral_id", ds->invocation.uuid);
        buffer_json_member_add_string(wb, "version", ds->version);

        buffer_json_member_add_time_t(wb, "uptime", ds->uptime);

        buffer_json_member_add_uuid(wb, "node_id", ds->node_id.uuid);
        buffer_json_member_add_uuid(wb, "claim_id", ds->claim_id.uuid);
        buffer_json_member_add_uint64(wb, "restarts", ds->restarts);

        if(ds->v >= 24)
            buffer_json_member_add_uint64(wb, "crashes", ds->crashes);
            
        // Only include PID if we're at version 27 or later
        if(ds->v >= 27)
            buffer_json_member_add_uint64(wb, "pid", (uint64_t)ds->pid);

        if(ds->v >= 22) {
            buffer_json_member_add_uint64(wb, "posts", ds->posts);
            buffer_json_member_add_string(wb, "aclk", CLOUD_STATUS_2str(ds->cloud_status));
        }

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

    // Add metrics stats as a top-level node
    buffer_json_member_add_object(wb, "metrics");
    {
        buffer_json_member_add_object(wb, "nodes");
        {
            buffer_json_member_add_uint64(wb, "total", ds->metrics_metadata.nodes.total);
            buffer_json_member_add_uint64(wb, "receiving", ds->metrics_metadata.nodes.receiving);
            buffer_json_member_add_uint64(wb, "sending", ds->metrics_metadata.nodes.sending);
            buffer_json_member_add_uint64(wb, "archived", ds->metrics_metadata.nodes.archived);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "metrics");
        {
            buffer_json_member_add_uint64(wb, "collected", ds->metrics_metadata.metrics.collected);
            buffer_json_member_add_uint64(wb, "available", ds->metrics_metadata.metrics.available);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "instances");
        {
            buffer_json_member_add_uint64(wb, "collected", ds->metrics_metadata.instances.collected);
            buffer_json_member_add_uint64(wb, "available", ds->metrics_metadata.instances.available);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "contexts");
        {
            buffer_json_member_add_uint64(wb, "collected", ds->metrics_metadata.contexts.collected);
            buffer_json_member_add_uint64(wb, "available", ds->metrics_metadata.contexts.available);
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
        
        buffer_json_member_add_uint64(wb, "system_cpus", ds->system_cpus);

        buffer_json_member_add_object(wb, "boot");
        {
            buffer_json_member_add_uuid_compact(wb, "id", ds->boot_id.uuid);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "memory");
        if(OS_SYSTEM_MEMORY_OK(ds->memory)) {
            buffer_json_member_add_uint64(wb, "total", ds->memory.ram_total_bytes);
            buffer_json_member_add_uint64(wb, "free", ds->memory.ram_available_bytes);

            if(ds->v >= 21) {
                buffer_json_member_add_uint64(wb, "netdata", ds->netdata_max_rss);
                buffer_json_member_add_uint64(wb, "oom_protection", ds->oom_protection);
            }
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
            
            buffer_json_member_add_object(wb, "netdata");
            buffer_json_member_add_uint64(wb, "dbengine", ds->disk_footprint.dbengine);
            buffer_json_member_add_uint64(wb, "sqlite", ds->disk_footprint.sqlite);
            buffer_json_member_add_uint64(wb, "other", ds->disk_footprint.other);
            buffer_json_member_add_string(wb, "last_updated", ds->disk_footprint.last_updated_ut_rfc3339);
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

    buffer_json_member_add_object(wb, "hw");
    {
        buffer_json_member_add_object(wb, "sys");
        {
            buffer_json_member_add_string(wb, "vendor", ds->hw.sys.vendor);
            buffer_json_member_add_string(wb, "uuid", ds->hw.sys.uuid);
            // buffer_json_member_add_string(wb, "serial", ds->hw.sys.serial);
            // buffer_json_member_add_string(wb, "asset_tag", ds->hw.sys.asset_tag);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "product");
        {
            buffer_json_member_add_string(wb, "name", ds->hw.product.name);
            buffer_json_member_add_string(wb, "version", ds->hw.product.version);
            buffer_json_member_add_string(wb, "sku", ds->hw.product.sku);
            buffer_json_member_add_string(wb, "family", ds->hw.product.family);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "board");
        {
            buffer_json_member_add_string(wb, "name", ds->hw.board.name);
            buffer_json_member_add_string(wb, "version", ds->hw.board.version);
            buffer_json_member_add_string(wb, "vendor", ds->hw.board.vendor);
            // buffer_json_member_add_string(wb, "serial", ds->hw.board.serial);
            // buffer_json_member_add_string(wb, "asset_tag", ds->hw.board.asset_tag);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "chassis");
        {
            buffer_json_member_add_string(wb, "type", ds->hw.chassis.type);
            buffer_json_member_add_string(wb, "vendor", ds->hw.chassis.vendor);
            buffer_json_member_add_string(wb, "version", ds->hw.chassis.version);
            // buffer_json_member_add_string(wb, "serial", ds->hw.chassis.serial);
            // buffer_json_member_add_string(wb, "asset_tag", ds->hw.chassis.asset_tag);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "bios");
        {
            buffer_json_member_add_string(wb, "date", ds->hw.bios.date);
            buffer_json_member_add_string(wb, "release", ds->hw.bios.release);
            buffer_json_member_add_string(wb, "version", ds->hw.bios.version);
            buffer_json_member_add_string(wb, "vendor", ds->hw.bios.vendor);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "product");
    {
        buffer_json_member_add_string(wb, "vendor", ds->product.vendor);
        buffer_json_member_add_string(wb, "name", ds->product.name);
        buffer_json_member_add_string(wb, "type", ds->product.type);
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

            if(ds->fatal.signal_code)
                print_uint64_hex(buf, ds->fatal.fault_address);
            else
                buf[0] = '\0';

            buffer_json_member_add_string(wb, "fault_address", buf);
        }

        if(ds->v >= 23)
            buffer_json_member_add_uint64(wb, "worker_job_id", ds->fatal.worker_job_id);
    }
    buffer_json_object_close(wb);

    dsf_release(*ds);
}

// --------------------------------------------------------------------------------------------------------------------
// json parsing

static bool daemon_status_file_from_json(json_object *jobj, void *data, BUFFER *error) {
    char path[1024]; path[0] = '\0';

    DAEMON_STATUS_FILE *ds = data;

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
    bool required_v21 = version >= 21 ? strict : false;
    bool required_v22 = version >= 22 ? strict : false;
    bool required_v23 = version >= 23 ? strict : false;
    bool required_v24 = version >= 24 ? strict : false;
    bool required_v25 = version >= 25 ? strict : false;
    bool required_v26 = version >= 26 ? strict : false;
    bool required_v27 = version >= 27 ? strict : false;

    // Parse timestamp
    JSONC_PARSE_TXT2RFC3339_USEC_OR_ERROR_AND_RETURN(jobj, path, "@timestamp", ds->timestamp_ut, error, required_v1);

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
        JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "id", ds->host_id.uuid.uuid, error, required_v1);

        if(version >= 24)
            JSONC_PARSE_TXT2RFC3339_USEC_OR_ERROR_AND_RETURN(jobj, path, "since", ds->host_id.last_modified_ut, error, required_v24);

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

        if(version >= 24)
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "crashes", ds->crashes, error, required_v24);
            
        // Only try to parse PID if we're at version 27 or later
        if(version >= 27)
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "pid", ds->pid, error, false);

        if(version >= 22) {
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "posts", ds->posts, error, required_v22);
            JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "aclk", CLOUD_STATUS_2id, ds->cloud_status, error, required_v22);
        }

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
        }
    });

    // Parse host object
    JSONC_PARSE_SUBOBJECT(jobj, path, "host", error, required_v1, {
        JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "id", ds->machine_id.uuid, error, required_v10);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "architecture", ds->architecture, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "virtualization", ds->virtualization, error, required_v1);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "container", ds->container, error, required_v1);
        JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "uptime", ds->boottime, error, required_v1);
        JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "system_cpus", ds->system_cpus, error, required_v27);

        JSONC_PARSE_SUBOBJECT(jobj, path, "boot", error, required_v1, {
            JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "id", ds->boot_id.uuid, error, required_v1);
        });

        JSONC_PARSE_SUBOBJECT(jobj, path, "memory", error, required_v1, {
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "total", ds->memory.ram_total_bytes, error, false);
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "free", ds->memory.ram_available_bytes, error, false);
            if(!OS_SYSTEM_MEMORY_OK(ds->memory))
                ds->memory = OS_SYSTEM_MEMORY_EMPTY;

            if(version >= 21) {
                JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "netdata", ds->netdata_max_rss, error, required_v21);
                JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "oom_protection", ds->oom_protection, error, required_v21);
            }
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
            
            JSONC_PARSE_SUBOBJECT(jobj, path, "netdata", error, required_v27, {
                JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "dbengine", ds->disk_footprint.dbengine, error, required_v27);
                JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "sqlite", ds->disk_footprint.sqlite, error, required_v27);
                JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "other", ds->disk_footprint.other, error, required_v27);
                JSONC_PARSE_TXT2RFC3339_USEC_OR_ERROR_AND_RETURN(jobj, path, "last_updated", ds->disk_footprint.last_updated_ut, error, required_v27);
                // Don't reset if not OK since this is a new field
            });
        });

        if(version >= 20) {
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "timezone", ds->timezone, error, required_v20);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "cloud_provider", ds->cloud_provider_type, error, required_v20);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "cloud_instance", ds->cloud_instance_type, error, required_v20);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "cloud_region", ds->cloud_instance_region, error, required_v20);
        }
    });
    
    // Parse metrics metadata
    JSONC_PARSE_SUBOBJECT(jobj, path, "metrics", error, required_v27, {
        JSONC_PARSE_SUBOBJECT(jobj, path, "nodes", error, required_v27, {
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "total", ds->metrics_metadata.nodes.total, error, required_v27);
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "receiving", ds->metrics_metadata.nodes.receiving, error, required_v27);
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "sending", ds->metrics_metadata.nodes.sending, error, required_v27);
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "archived", ds->metrics_metadata.nodes.archived, error, required_v27);
        });
        
        JSONC_PARSE_SUBOBJECT(jobj, path, "metrics", error, required_v27, {
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "collected", ds->metrics_metadata.metrics.collected, error, required_v27);
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "available", ds->metrics_metadata.metrics.available, error, required_v27);
        });
        
        JSONC_PARSE_SUBOBJECT(jobj, path, "instances", error, required_v27, {
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "collected", ds->metrics_metadata.instances.collected, error, required_v27);
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "available", ds->metrics_metadata.instances.available, error, required_v27);
        });
        
        JSONC_PARSE_SUBOBJECT(jobj, path, "contexts", error, required_v27, {
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "collected", ds->metrics_metadata.contexts.collected, error, required_v27);
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "available", ds->metrics_metadata.contexts.available, error, required_v27);
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

    // Parse the hw object
    JSONC_PARSE_SUBOBJECT(jobj, path, "hw", error, required_v25, {
        JSONC_PARSE_SUBOBJECT(jobj, path, "sys", error, required_v25, {
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "vendor", ds->hw.sys.vendor, error, required_v25);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "uuid", ds->hw.sys.uuid, error, required_v27);
            // JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "serial", ds->hw.sys.serial, error, required_v26);
            // JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "asset_tag", ds->hw.sys.asset_tag, error, required_v27);
        });

        JSONC_PARSE_SUBOBJECT(jobj, path, "product", error, required_v25, {
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "name", ds->hw.product.name, error, required_v25);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "version", ds->hw.product.version, error, required_v25);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "sku", ds->hw.product.sku, error, required_v25);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "family", ds->hw.product.family, error, required_v25);
        });

        JSONC_PARSE_SUBOBJECT(jobj, path, "board", error, required_v25, {
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "name", ds->hw.board.name, error, required_v25);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "version", ds->hw.board.version, error, required_v25);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "vendor", ds->hw.board.vendor, error, required_v25);
            // JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "serial", ds->hw.board.serial, error, required_v26);
            // JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "asset_tag", ds->hw.board.asset_tag, error, required_v27);
        });

        JSONC_PARSE_SUBOBJECT(jobj, path, "chassis", error, required_v25, {
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "type", ds->hw.chassis.type, error, required_v25);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "vendor", ds->hw.chassis.vendor, error, required_v25);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "version", ds->hw.chassis.version, error, required_v25);
            // JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "serial", ds->hw.chassis.serial, error, required_v26);
            // JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "asset_tag", ds->hw.chassis.asset_tag, error, required_v27);
        });

        JSONC_PARSE_SUBOBJECT(jobj, path, "bios", error, required_v25, {
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "date", ds->hw.bios.date, error, required_v25);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "release", ds->hw.bios.release, error, required_v25);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "version", ds->hw.bios.version, error, required_v25);
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "vendor", ds->hw.bios.vendor, error, required_v25);
        });
    });

    // Parse product object
    JSONC_PARSE_SUBOBJECT(jobj, path, "product", error, required_v26, {
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "vendor", ds->product.vendor, error, required_v26);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "name", ds->product.name, error, required_v26);
        JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "type", ds->product.type, error, required_v26);
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

        if(version >= 18) {
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "thread_id", ds->fatal.thread_id, error, required_v18);

            char buf[UINT64_HEX_MAX_LENGTH];
            JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "fault_address", buf, error, required_v18);
            if(buf[0])
                ds->fatal.fault_address = str2ull_encoded(buf);
            else
                ds->fatal.fault_address = 0;
        }

        if(version >= 23)
            JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "worker_job_id", ds->fatal.worker_job_id, error, required_v23);
    });

    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// get the current status

static void daemon_status_file_migrate_once(void) {
    FUNCTION_RUN_ONCE();

    dsf_acquire(last_session_status);
    dsf_acquire(session_status);

    safecpy(session_status.version, NETDATA_VERSION);
    session_status.machine_id = os_machine_id();

    {
        char *install_type = NULL, *prebuilt_arch = NULL, *prebuilt_dist = NULL;
        get_install_type_internal(&install_type, &prebuilt_arch, &prebuilt_dist);

        if(install_type)
            safecpy(session_status.install_type, install_type);

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
    session_status.host_id = *machine_guid_get();

    safecpy(session_status.architecture, last_session_status.architecture);
    safecpy(session_status.virtualization, last_session_status.virtualization);
    safecpy(session_status.container, last_session_status.container);
    safecpy(session_status.kernel_version, last_session_status.kernel_version);
    safecpy(session_status.os_name, last_session_status.os_name);
    safecpy(session_status.os_version, last_session_status.os_version);
    safecpy(session_status.os_id, last_session_status.os_id);
    safecpy(session_status.os_id_like, last_session_status.os_id_like);
    safecpy(session_status.timezone, last_session_status.timezone);
    safecpy(session_status.cloud_provider_type, last_session_status.cloud_provider_type);
    safecpy(session_status.cloud_instance_type, last_session_status.cloud_instance_type);
    safecpy(session_status.cloud_instance_region, last_session_status.cloud_instance_region);

    session_status.posts = last_session_status.posts;
    session_status.restarts = last_session_status.restarts + 1;
    session_status.crashes = last_session_status.crashes;
    session_status.reliability = last_session_status.reliability;

    if(daemon_status_file_has_last_crashed(&last_session_status))  {
        session_status.crashes++;
        if(session_status.reliability > 0) session_status.reliability = 0;
        session_status.reliability--;
    }
    else {
        if(session_status.reliability < 0) session_status.reliability = 0;
        session_status.reliability++;
    }

#ifdef HAVE_LIBBACKTRACE
    safecpy(session_status.stack_traces, stacktrace_backend());
#endif

    fill_dmi_info(&session_status);

    dsf_release(last_session_status);
    dsf_release(session_status);
}

static void daemon_status_file_refresh(DAEMON_STATUS status) {
    usec_t now_ut = now_realtime_usec();

    dsf_acquire(session_status);

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

    session_status.host_id = *machine_guid_get();
    session_status.boottime = now_boottime_sec();
    session_status.uptime = now_realtime_sec() - netdata_start_time;
    session_status.timestamp_ut = now_ut;
    rfc3339_datetime_ut(session_status.timestamp_ut_rfc3339, sizeof(session_status.timestamp_ut_rfc3339),
                        session_status.timestamp_ut, 2, true);
    session_status.invocation = nd_log_get_invocation_id();
    session_status.db_mode = default_rrd_memory_mode;
    session_status.db_tiers = nd_profile.storage_tiers;
    session_status.pid = getpid();

    // we keep the highest cloud status, to know how the agent gets connected to netdata.cloud
    CLOUD_STATUS cs = cloud_status();
    if(!session_status.cloud_status ||                              // it is ok to overwrite this
        session_status.cloud_status == CLOUD_STATUS_AVAILABLE ||    // it is ok to overwrite this
        session_status.cloud_status == CLOUD_STATUS_OFFLINE ||      // it is ok to overwrite this
        cs == CLOUD_STATUS_BANNED ||                                // this is a final state
        cs == CLOUD_STATUS_ONLINE ||                                // this is a final state
        cs == CLOUD_STATUS_INDIRECT)                                // this is a final state
        session_status.cloud_status = cs;


#ifdef ENABLE_DBENGINE
    session_status.oom_protection = dbengine_out_of_memory_protection;
#else
    session_status.oom_protection = 0;
#endif
    
    OS_PROCESS_MEMORY proc_mem = os_process_memory(0);
    if(OS_PROCESS_MEMORY_OK(proc_mem))
        session_status.netdata_max_rss = proc_mem.max_rss;

    session_status.claim_id = claim_id_get_uuid();

    if(localhost) {
        if(!UUIDiszero(localhost->host_id))
            session_status.host_id.uuid = localhost->host_id;

        if(!UUIDiszero(localhost->node_id))
            session_status.node_id = localhost->node_id;
    }

    if(get_daemon_status_fields_from_system_info(&session_status))
        product_name_vendor_type(&session_status);

    if(netdata_configured_timezone)
        safecpy(session_status.timezone, netdata_configured_timezone);

    session_status.exit_reason = exit_initiated_get();
    session_status.profile = nd_profile_detect_and_configure(false);

    if(status != DAEMON_STATUS_NONE)
        session_status.status = status;

    session_status.memory = os_system_memory(true);
    session_status.var_cache = os_disk_space(netdata_configured_cache_dir);
    session_status.system_cpus = os_get_system_cpus();
    
    // Collect metrics metadata statistics
    session_status.metrics_metadata = rrdstats_metadata_collect();
    
    // Update disk footprint at most once every 10 minutes (600 seconds)
    if ((now_ut - session_status.disk_footprint.last_updated_ut) >= 600 * USEC_PER_SEC ||
        session_status.disk_footprint.last_updated_ut == 0) {
        // Calculate disk footprint by categories
        const char *dirs_to_measure[] = {
            netdata_configured_varlib_dir,
            netdata_configured_cache_dir
        };
        
        // Create patterns for different file types
        SIMPLE_PATTERN *dbengine_pattern = simple_pattern_create("*dbengine*/*.ndf *dbengine*/*.njf*", " ", SIMPLE_PATTERN_EXACT, false);
        SIMPLE_PATTERN *sqlite_pattern = simple_pattern_create("*.db *.wal *.shm", " ", SIMPLE_PATTERN_EXACT, false);
        
        // Get total size first
        DIR_SIZE total_size = dir_size_multiple(dirs_to_measure, 2, NULL, 0);
        
        // Get DBEngine files size
        DIR_SIZE dbengine_size = dir_size_multiple(dirs_to_measure, 2, dbengine_pattern, 0);
        session_status.disk_footprint.dbengine = dbengine_size.bytes;
        
        // Get SQLite files size
        DIR_SIZE sqlite_size = dir_size_multiple(dirs_to_measure, 2, sqlite_pattern, 0);
        session_status.disk_footprint.sqlite = sqlite_size.bytes;
        
        // Calculate other files (total - dbengine - sqlite)
        session_status.disk_footprint.other = total_size.bytes - dbengine_size.bytes - sqlite_size.bytes;

        // Update last updated timestamp
        session_status.disk_footprint.last_updated_ut = now_ut;
        rfc3339_datetime_ut(session_status.disk_footprint.last_updated_ut_rfc3339,
                            sizeof(session_status.disk_footprint.last_updated_ut_rfc3339),
                            session_status.disk_footprint.last_updated_ut, 2, true);

        // Clean up patterns
        simple_pattern_free(dbengine_pattern);
        simple_pattern_free(sqlite_pattern);
    }

    dsf_release(session_status);
}

// --------------------------------------------------------------------------------------------------------------------
// load a saved status

static bool status_file_load_and_parse(const char *filename, void *data) {
    DAEMON_STATUS_FILE *status = data;

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    CLEAN_BUFFER *error = buffer_create(0, NULL);

    if(!read_txt_file_to_buffer(filename, wb, 65536))
        return false;

    // Parse the JSON
    return json_parse_payload_or_error(wb, error, daemon_status_file_from_json, status) == HTTP_RESP_OK;
}

// --------------------------------------------------------------------------------------------------------------------
// save the current status

static BUFFER *static_save_buffer = NULL;
static void static_save_buffer_init(void) {
    if (!static_save_buffer)
        static_save_buffer = buffer_create(16384, NULL);

    buffer_flush(static_save_buffer);
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

    if(status_file_io_save(STATUS_FILENAME, buffer_tostring(wb), buffer_strlen(wb), log))
        daemon_status_file_saved = true;
}

// --------------------------------------------------------------------------------------------------------------------
// POST the last status to agent-events

struct post_status_file_thread_data {
    const char *cause;
    const char *msg;
    ND_LOG_FIELD_PRIORITY priority;
    DAEMON_STATUS_FILE *status;
};

static const char *agent_health(DAEMON_STATUS_FILE *ds) {
    if(daemon_status_file_has_last_crashed(ds)) {
        // it crashed

        if(ds->restarts == 1)
            return "crash-first";
        else if(ds->reliability <= -2)
            return "crash-loop";
        else if(ds->reliability < 0)
            return "crash-repeated";
        else
            return "crash-entered";
    }

    // it didn't crash
    if(ds->restarts == 1)
        return "healthy-first";
    else if(ds->reliability >= 2)
        return "healthy-loop";
    else if(ds->reliability > 0)
        return "healthy-repeated";
    else
        return "healthy-recovered";
}

static void post_status_file(struct post_status_file_thread_data *d) {
    daemon_status_file_startup_step("startup(crash reports json)");

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_string(wb, "exit_cause", d->cause);
    buffer_json_member_add_string(wb, "message", d->msg);
    buffer_json_member_add_uint64(wb, "priority", d->priority);
    buffer_json_member_add_uint64(wb, "version_saved", d->status->v);
    buffer_json_member_add_string(wb, "agent_version_now", NETDATA_VERSION);
    buffer_json_member_add_uint64(wb, "agent_pid_now", getpid());
    buffer_json_member_add_boolean(wb, "host_memory_critical",
                                   OS_SYSTEM_MEMORY_OK(d->status->memory) && d->status->memory.ram_available_bytes <= d->status->oom_protection);
    buffer_json_member_add_uint64(wb, "host_memory_free_percent", (uint64_t)round(os_system_memory_available_percent(d->status->memory)));
    buffer_json_member_add_string(wb, "agent_health", agent_health(d->status));
    daemon_status_file_to_json(wb, d->status);
    buffer_json_finalize(wb);

    const char *json_data = buffer_tostring(wb);

    CURL *curl = curl_easy_init();
    if(!curl)
        return;

    daemon_status_file_startup_step("startup(crash reports curl)");

    curl_easy_setopt(curl, CURLOPT_URL, "https://agent-events.netdata.cloud/agent-events");
    curl_easy_setopt(curl, CURLOPT_POST, 1L);
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, json_data);
    struct curl_slist *headers = NULL;
    headers = curl_slist_append(headers, "Content-Type: application/json");
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
    curl_easy_setopt(curl, CURLOPT_TIMEOUT, 10L);

    CURLcode rc = curl_easy_perform(curl);
    if(rc == CURLE_OK) {
        daemon_status_file_startup_step("startup(crash reports dedup)");
        session_status.posts++;
        nd_log(NDLS_DAEMON, NDLP_INFO, "Posted last status to agent-events successfully.");
        uint64_t hash = daemon_status_file_hash(d->status, d->msg, d->cause);
        dedup_keep_hash(&session_status, hash, false);
        daemon_status_file_save(wb, &session_status, true);
    }
    else
        nd_log(NDLS_DAEMON, NDLP_INFO, "Failed to post last status to agent-events.");

    daemon_status_file_startup_step("startup(crash reports cleanup)");

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
    // List of known CI environment variables.
    const char *ci_vars[] = {
        "CI",                       // Generic CI flag
        "CONTINUOUS_INTEGRATION",   // Alternate generic flag
        "BUILD_NUMBER",             // Jenkins, TeamCity
        "RUN_ID",                   // AWS CodeBuild, some others
        "TRAVIS",                   // Travis CI
        "GITHUB_ACTIONS",           // GitHub Actions
        "GITHUB_TOKEN",             // GitHub Actions
        "GITLAB_CI",                // GitLab CI
        "CIRCLECI",                 // CircleCI
        "APPVEYOR",                 // AppVeyor
        "BITBUCKET_BUILD_NUMBER",   // Bitbucket Pipelines
        "SYSTEM_TEAMFOUNDATIONCOLLECTIONURI", // Azure DevOps
        "TF_BUILD",                 // Azure DevOps (alternate)
        "BAMBOO_BUILDKEY",          // Bamboo CI
        "GO_PIPELINE_NAME",         // GoCD
        "HUDSON_URL",               // Hudson CI
        "TEAMCITY_VERSION",         // TeamCity
        "CI_NAME",                  // Some environments (e.g., CodeShip)
        "CI_WORKER",                // AppVeyor (alternate)
        "CI_SERVER",                // Generic
        "HEROKU_TEST_RUN_ID",       // Heroku CI
        "BUILDKITE",                // Buildkite
        "DRONE",                    // Drone CI
        "SEMAPHORE",                // Semaphore CI
        "NETLIFY",                  // Netlify CI
        "NOW_BUILDER",              // Vercel (formerly Zeit Now)
        NULL
    };

    // Iterate over the CI environment variable names.
    for (const char **env = ci_vars; *env; env++) {
        if(getenv(*env))
            return true;
    }

    return false;
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

    status_file_io_load(STATUS_FILENAME, status_file_load_and_parse, &last_session_status);

    // fill missing information on older versions of the status file

    if(last_session_status.v <= 26)
        fill_dmi_info(&last_session_status);

    if(last_session_status.v <= 27)
        last_session_status.system_cpus = os_get_system_cpus();

    // Regenerate RFC3339 strings from loaded timestamps for async-signal-safe compatibility
    // (these fields don't exist in saved JSON, they're runtime-only for signal handlers)
    if(last_session_status.timestamp_ut)
        rfc3339_datetime_ut(last_session_status.timestamp_ut_rfc3339,
                            sizeof(last_session_status.timestamp_ut_rfc3339),
                            last_session_status.timestamp_ut, 2, true);

    if(last_session_status.host_id.last_modified_ut)
        rfc3339_datetime_ut(last_session_status.host_id.last_modified_ut_rfc3339,
                            sizeof(last_session_status.host_id.last_modified_ut_rfc3339),
                            last_session_status.host_id.last_modified_ut, 2, true);

    if(last_session_status.disk_footprint.last_updated_ut)
        rfc3339_datetime_ut(last_session_status.disk_footprint.last_updated_ut_rfc3339,
                            sizeof(last_session_status.disk_footprint.last_updated_ut_rfc3339),
                            last_session_status.disk_footprint.last_updated_ut, 2, true);

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
            else if (OS_SYSTEM_MEMORY_OK(last_session_status.memory) &&
                     last_session_status.memory.ram_available_bytes <= last_session_status.oom_protection) {
                cause = "killed hard low ram";
                msg = "Netdata was last killed/crashed while available memory was critically low";
                pri = PRI_KILLED_HARD;
                this_is_a_crash = true;
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

    daemon_status_file_startup_step("startup(crash reports check)");

    enum crash_report_t r = check_crash_reports_config();
    if( // must be first for netdata.conf option to be used
        (r == DSF_REPORT_ALL || (this_is_a_crash && r == DSF_REPORT_CRASHES)) &&

        // we have a previous status, or we managed to save the current one
        (!no_previous_status || daemon_status_file_saved) &&

        // we have more than 2 restarts, or this is not a CI run
        (last_session_status.restarts > 1 || !is_ci()) &&

        // we have not reported this
        !dedup_already_posted(&session_status, daemon_status_file_hash(&last_session_status, msg, cause), false)

        ) {
        daemon_status_file_startup_step("startup(crash reports prep)");

        netdata_conf_ssl();

        if(no_previous_status) {
            last_session_status = session_status;
            last_session_status.status = DAEMON_STATUS_NONE;
            last_session_status.exit_reason = 0;
            memset(&last_session_status.fatal, 0, sizeof(last_session_status.fatal));
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

NEVER_INLINE
static void daemon_status_file_save_twice_if_we_can_get_stack_trace(BUFFER *wb, DAEMON_STATUS_FILE *ds, bool force) {
    // IMPORTANT: NO LOCKS OR ALLOCATIONS HERE, THIS FUNCTION IS CALLED FROM SIGNAL HANDLERS
    // THIS FUNCTION MUST USE ONLY ASYNC-SIGNAL-SAFE OPERATIONS

#ifdef HAVE_LIBBACKTRACE
    if(stacktrace_available())
        set_stack_trace_message_if_empty(&session_status, STACK_TRACE_INFO_PREFIX "will now attempt to get stack trace - if you see this message, we couldn't get it.");
    else
#endif
        set_stack_trace_message_if_empty(&session_status, STACK_TRACE_INFO_PREFIX "no stack trace backend available");

    // save it without a stack trace to be sure we will have the event
    daemon_status_file_save(wb, ds, false);

    if(!stack_trace_is_empty(ds) && !force)
        return;

    buffer_flush(wb);

#ifdef HAVE_LIBBACKTRACE
    stacktrace_capture(wb);

    // Store the first netdata function from the stack trace if available
    const char *first_nd_fn = stacktrace_root_cause_function();
    if (first_nd_fn && *first_nd_fn &&
        (!ds->fatal.function[0] || strncmp(ds->fatal.function, "thread:", 7) == 0))
        safecpy(ds->fatal.function, first_nd_fn);
#endif

    if(buffer_strlen(wb) > 0) {
        safecpy(ds->fatal.stack_trace, buffer_tostring(wb));

        daemon_status_file_save(wb, ds, false);
    }

    errno_clear();
}

// --------------------------------------------------------------------------------------------------------------------
// ng_log() hook for receiving fatal message information

NEVER_INLINE
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
        safecpy(session_status.fatal.filename, filename);

    if(function && *function)
        safecpy(session_status.fatal.function, function);

    if(message && *message)
        safecpy(session_status.fatal.message, message);

    if(errno_str && *errno_str)
        safecpy(session_status.fatal.errno_str, errno_str);

    if(stack_trace && *stack_trace && stack_trace_is_empty(&session_status))
        safecpy(session_status.fatal.stack_trace, stack_trace);

    if(!session_status.fatal.worker_job_id)
        session_status.fatal.worker_job_id = workers_get_last_job_id();

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
    int saved_errno = errno;
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    daemon_status_file_refresh(status);
    daemon_status_file_save(wb, &session_status, true);
    errno = saved_errno;
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

NEVER_INLINE
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

    if(!session_status.fatal.worker_job_id)
        session_status.fatal.worker_job_id = workers_get_last_job_id();

    copy_and_clean_thread_name_if_empty(&session_status, nd_thread_tag_async_safe());

    if(!session_status.fatal.function[0] ||
        strncmp(session_status.fatal.function, "startup(", 8) == 0 ||
        strncmp(session_status.fatal.function, "shutdown(", 9) == 0) {
        size_t len = 0;
        len = strcatz(session_status.fatal.function, len, "thread:", sizeof(session_status.fatal.function));
        len = strcatz(session_status.fatal.function, len, session_status.fatal.thread, sizeof(session_status.fatal.function));
        if(session_status.fatal.worker_job_id <= WORKER_UTILIZATION_MAX_JOB_TYPES) {
            len = strcatz(session_status.fatal.function, len, ":", sizeof(session_status.fatal.function));
            len += print_uint64(&session_status.fatal.function[len], session_status.fatal.worker_job_id);
        }
    }

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

#ifdef HAVE_LIBBACKTRACE
#if defined(OS_WINDOWS)
    // THE FOLLOWING CODE IS NOT ASYNC-SIGNAL-SAFE on MSYS2 due to internal locking in the runtime.
    // This can cause a deadlock when a signal is received while the lock is held.
    // The code is commented out to prevent the deadlock, at the cost of not saving the status file on a crash.
#else
    bool safe_to_get_stack_trace = reason != EXIT_REASON_SIGABRT || stacktrace_capture_is_async_signal_safe();
    bool get_stack_trace = stacktrace_available() && safe_to_get_stack_trace && stack_trace_is_empty(&session_status);

    // save it
    if(get_stack_trace)
        daemon_status_file_save_twice_if_we_can_get_stack_trace(static_save_buffer, &session_status, true);
    else {
        if (!stacktrace_available())
            set_stack_trace_message_if_empty(&session_status, STACK_TRACE_INFO_PREFIX "no stack trace backend available");
        else
            set_stack_trace_message_if_empty(&session_status, STACK_TRACE_INFO_PREFIX "not safe to get a stack trace for this signal using this backend");

        daemon_status_file_save(static_save_buffer, &session_status, false);
    }
#endif // defined(OS_WINDOWS)
#endif // HAVE_LIBBACKTRACE

    return duplicate;
}

// --------------------------------------------------------------------------------------------------------------------
// shutdown related functions

static SPINLOCK shutdown_timeout_spinlock = SPINLOCK_INITIALIZER;

NEVER_INLINE
void daemon_status_file_shutdown_timeout(BUFFER *trace) {
    FUNCTION_RUN_ONCE();

    spinlock_lock(&shutdown_timeout_spinlock);

    dsf_acquire(session_status);
    exit_initiated_add(EXIT_REASON_SHUTDOWN_TIMEOUT);
    session_status.exit_reason |= EXIT_REASON_SHUTDOWN_TIMEOUT;
    if(trace && buffer_strlen(trace) && stack_trace_is_empty(&session_status))
        safecpy(session_status.fatal.stack_trace, buffer_tostring(trace));
    dsf_release(session_status);

    safecpy(session_status.fatal.function, "shutdown_timeout");

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    daemon_status_file_save(wb, &session_status, false);

    // keep the spinlock locked, to prevent further steps updating the status
}

void daemon_status_file_shutdown_step(const char *step, const char *step_timings) {
    if(session_status.fatal.filename[0] || !spinlock_trylock(&shutdown_timeout_spinlock))
        // we have a fatal logged
        return;

    if(step != NULL)
        snprintfz(session_status.fatal.function, sizeof(session_status.fatal.function), "shutdown(%s)", step);
    else
        session_status.fatal.function[0] = '\0';

    if(step_timings && *step_timings && stack_trace_is_empty(&session_status))
        safecpy(session_status.fatal.stack_trace, step_timings);

    daemon_status_file_update_status(DAEMON_STATUS_EXITING);

    spinlock_unlock(&shutdown_timeout_spinlock);
}

// --------------------------------------------------------------------------------------------------------------------

bool daemon_status_file_has_last_crashed(DAEMON_STATUS_FILE *ds) {
    if(!ds) ds = &last_session_status;

    return (ds->status != DAEMON_STATUS_NONE && ds->status != DAEMON_STATUS_EXITED) ||
           !is_exit_reason_normal(ds->exit_reason);
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
        safecpy(session_status.fatal.function, step);
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

const char *daemon_status_file_get_sys_vendor(void) {
    return session_status.product.vendor;
}

const char *daemon_status_file_get_product_name(void) {
    return session_status.product.name;
}

const char *daemon_status_file_get_product_type(void) {
    return session_status.product.type;
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

ND_MACHINE_GUID daemon_status_file_get_host_id(void) {
    return last_session_status.host_id;
}

size_t daemon_status_file_get_fatal_worker_job_id(void) {
    return session_status.fatal.worker_job_id;
}
