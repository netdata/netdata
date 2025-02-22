// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "daemon-status-file.h"

#include <curl/curl.h>
#include <openssl/evp.h>
#include <openssl/pem.h>
#include <openssl/err.h>

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

static void daemon_status_file_to_json(BUFFER *wb, DAEMON_STATUS_FILE *ds) {
    buffer_json_member_add_string(wb, "version", ds->version);
    buffer_json_member_add_string(wb, "status", DAEMON_STATUS_2str(ds->status));
    EXIT_REASON_2json(wb, "reason", ds->reason);
    ND_PROFILE_2json(wb, "profile", ds->profile);
    buffer_json_member_add_string(wb, "os_type", DAEMON_OS_TYPE_2str(ds->os_type));

    buffer_json_member_add_time_t(wb, "boottime", ds->boottime);
    buffer_json_member_add_time_t(wb, "uptime", ds->uptime);
    buffer_json_member_add_datetime_rfc3339(wb, "wallclock", ds->timestamp_ut, true);
    buffer_json_member_add_uuid_compact(wb, "invocation", ds->invocation.uuid);
    buffer_json_member_add_uuid(wb, "boot_id", ds->boot_id.uuid);
    buffer_json_member_add_uuid(wb, "host_id", ds->host_id.uuid);
    buffer_json_member_add_uuid(wb, "node_id", ds->node_id.uuid);
    buffer_json_member_add_uuid(wb, "claim_id", ds->claim_id.uuid);

    buffer_json_member_add_object(wb, "timings");
    {
        buffer_json_member_add_time_t(wb, "init", ds->timings.init);
        buffer_json_member_add_time_t(wb, "exit", ds->timings.exit);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "ram");
    if(OS_SYSTEM_MEMORY_OK(ds->memory)) {
        buffer_json_member_add_uint64(wb, "total", ds->memory.ram_total_bytes);
        buffer_json_member_add_uint64(wb, "free", ds->memory.ram_available_bytes);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "db_disk");
    if(OS_SYSTEM_DISK_SPACE_OK(ds->var_cache)) {
        buffer_json_member_add_uint64(wb, "total", ds->var_cache.total_bytes);
        buffer_json_member_add_uint64(wb, "free", ds->var_cache.free_bytes);
        buffer_json_member_add_uint64(wb, "inodes_total", ds->var_cache.total_inodes);
        buffer_json_member_add_uint64(wb, "inodes_free", ds->var_cache.free_inodes);
        buffer_json_member_add_boolean(wb, "read_only", ds->var_cache.is_read_only);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "fatal");
    {
        buffer_json_member_add_uint64(wb, "line", ds->fatal.line);
        buffer_json_member_add_string_or_empty(wb, "filename", ds->fatal.filename);
        buffer_json_member_add_string_or_empty(wb, "function", ds->fatal.function);
        buffer_json_member_add_string_or_empty(wb, "message", ds->fatal.message);
        buffer_json_member_add_string_or_empty(wb, "stack_trace", ds->fatal.stack_trace);
    }
    buffer_json_object_close(wb);
}

static bool daemon_status_file_from_json_cache_dir(json_object *jobj, const char *path, DAEMON_STATUS_FILE *ds, BUFFER *error, bool required __maybe_unused) {
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "total", ds->var_cache.total_bytes, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "free", ds->var_cache.free_bytes, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "inodes_total", ds->var_cache.total_inodes, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "inodes_free", ds->var_cache.free_inodes, error, false);
    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, "read_only", ds->var_cache.is_read_only, error, false);
    if(!OS_SYSTEM_DISK_SPACE_OK(ds->var_cache))
        ds->var_cache = OS_SYSTEM_DISK_SPACE_EMPTY;
    return true;
}

static bool daemon_status_file_from_json_ram(json_object *jobj, const char *path, DAEMON_STATUS_FILE *ds, BUFFER *error, bool required __maybe_unused) {
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "total", ds->memory.ram_total_bytes, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "free", ds->memory.ram_available_bytes, error, false);
    if(!OS_SYSTEM_MEMORY_OK(ds->memory))
        ds->memory = OS_SYSTEM_MEMORY_EMPTY;
    return true;
}

static bool daemon_status_file_from_json_fatal(json_object *jobj, const char *path, DAEMON_STATUS_FILE *ds, BUFFER *error, bool required __maybe_unused) {
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "filename", ds->fatal.filename, error, false);
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "function", ds->fatal.function, error, false);
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "message", ds->fatal.message, error, false);
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "stack_trace", ds->fatal.stack_trace, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "line", ds->fatal.line, error, false);
    return true;
}

static bool daemon_status_file_from_json_timings(json_object *jobj, const char *path, DAEMON_STATUS_FILE *ds, BUFFER *error, bool required __maybe_unused) {
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "init", ds->timings.init, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "exit", ds->timings.exit, error, false);
    return true;
}

static bool daemon_status_file_from_json(json_object *jobj, const char *path, void *data, BUFFER *error) {
    char datetime[RFC3339_MAX_LENGTH]; datetime[0] = '\0';

    DAEMON_STATUS_FILE *ds = data;
    JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "version", ds->version, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "boottime", ds->boottime, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "uptime", ds->uptime, error, false);

    JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "wallclock", datetime, error, false);
    if(datetime[0])
        ds->timestamp_ut = rfc3339_parse_ut(datetime, NULL);

    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "invocation", ds->invocation.uuid, error, false);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "boot_id", ds->boot_id.uuid, error, false);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "host_id", ds->host_id.uuid, error, false);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "node_id", ds->node_id.uuid, error, false);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "claim_id", ds->claim_id.uuid, error, false);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "status", DAEMON_STATUS_2id, ds->status, error, false);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "os_type", DAEMON_OS_TYPE_2id, ds->os_type, error, false);
    JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, "reason", EXIT_REASON_2id_one, ds->reason, error, false);
    JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, "profile", ND_PROFILE_2id_one, ds->profile, error, false);

    JSONC_PARSE_SUBOBJECT(jobj, path, "timings", ds, daemon_status_file_from_json_timings, error, false);
    JSONC_PARSE_SUBOBJECT(jobj, path, "db_disk", ds, daemon_status_file_from_json_cache_dir, error, false);
    JSONC_PARSE_SUBOBJECT(jobj, path, "ram", ds, daemon_status_file_from_json_ram, error, false);
    JSONC_PARSE_SUBOBJECT(jobj, path, "fatal", ds, daemon_status_file_from_json_fatal, error, false);
    return true;
}

static DAEMON_STATUS_FILE daemon_status_file_get(DAEMON_STATUS status) {
    usec_t now_ut = now_realtime_usec();

#if defined(OS_LINUX)
    session_status.os_type = DAEMON_OS_TYPE_LINUX;
#elif defined(OS_FREEBSD)
    session_status.built_for = DAEMON_OS_TYPE_FREEBSD;
#elif defined(OS_MACOS)
    session_status.built_for = DAEMON_OS_TYPE_MACOS;
#elif defined(OS_WINDOWS)
    session_status.built_for = DAEMON_OS_TYPE_WINDOWS;
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
        const char *machine_guid = registry_get_this_machine_guid();
        if(machine_guid && *machine_guid) {
            if (uuid_parse_flexi(machine_guid, session_status.host_id.uuid) != 0)
                session_status.host_id = UUID_ZERO;
        }
        else
            session_status.host_id = UUID_ZERO;
    }

    if(UUIDiszero(session_status.claim_id))
        session_status.claim_id = last_session_status.claim_id;
    if(UUIDiszero(session_status.node_id))
        session_status.node_id = last_session_status.node_id;
    if(UUIDiszero(session_status.host_id))
        session_status.host_id = last_session_status.host_id;

    session_status.reason = exit_initiated;
    session_status.profile = nd_profile_detect_and_configure(false);
    session_status.status = status;

    session_status.memory = os_system_memory(true);
    session_status.var_cache = os_disk_space(netdata_configured_cache_dir);

    return session_status;
}

DAEMON_STATUS_FILE daemon_status_file_load(void) {
    DAEMON_STATUS_FILE status = {
        .boottime = 0,
        .uptime = 0,
        .timestamp_ut = 0,
        .invocation = UUID_ZERO,
        .reason = EXIT_REASON_NONE,
        .status = DAEMON_STATUS_NONE,
    };

    char filename[FILENAME_MAX];
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    CLEAN_BUFFER *error = buffer_create(0, NULL);

    for(size_t x = 0; x < 2 ;x++) {
        if(x == 0)
            snprintfz(filename, sizeof(filename), "%s/status-netdata.json", os_run_dir(true));
        else
            snprintfz(filename, sizeof(filename), "%s/status-netdata.json", netdata_configured_cache_dir);

        FILE *fp = fopen(filename, "r");
        if (fp) {
            fseek(fp, 0, SEEK_END);
            long file_size = ftell(fp);
            fseek(fp, 0, SEEK_SET);

            buffer_need_bytes(wb, file_size + 1);

            size_t read_bytes = fread(wb->buffer, 1, file_size, fp);
            fclose(fp);

            wb->buffer[read_bytes] = '\0';
            wb->len = read_bytes;

            buffer_flush(error);
            memset(&status, 0, sizeof(status));

            if(json_parse_payload_or_error(wb, error, daemon_status_file_from_json, &status) == HTTP_RESP_OK)
                break;
        }
    }

    return status;
}

void daemon_status_file_save(DAEMON_STATUS status) {
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    spinlock_lock(&spinlock);

    DAEMON_STATUS_FILE ds = daemon_status_file_get(status);

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    daemon_status_file_to_json(wb, &ds);
    buffer_json_finalize(wb);

    const char *run_dir = os_run_dir(true);

    char filename[FILENAME_MAX];
    char temp_filename[FILENAME_MAX];
    for (size_t x = 0; x < 2 ; x++) {
        if(x == 0) {
            snprintfz(filename, sizeof(filename), "%s/status-netdata.json", run_dir);
            snprintfz(temp_filename, sizeof(temp_filename), "%s/status-netdata.json.tmp", run_dir);
        }
        else {
            snprintfz(filename, sizeof(filename), "%s/status-netdata", netdata_configured_cache_dir);
            snprintfz(temp_filename, sizeof(temp_filename), "%s/status-netdata.tmp", netdata_configured_cache_dir);
        }

        FILE *fp = fopen(temp_filename, "w");
        if (fp) {
            bool ok = fwrite(buffer_tostring(wb), 1, buffer_strlen(wb), fp) == buffer_strlen(wb);
            fclose(fp);

            if (ok)
                (void)rename(temp_filename, filename);
            else {
                (void)unlink(filename);
                (void)unlink(temp_filename);
            }
        }
    }

    spinlock_unlock(&spinlock);
}

struct post_status_file_thread_data {
    const char *cause;
    const char *msg;
    ND_LOG_FIELD_PRIORITY priority;
    DAEMON_STATUS_FILE status;
};

void post_status_file(struct post_status_file_thread_data *d) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_string(wb, "cause", d->cause);
    buffer_json_member_add_string(wb, "message", d->msg);
    buffer_json_member_add_uint64(wb, "priority", d->priority);
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
    (void)rc;

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

void daemon_status_file_check_crash(void) {
    last_session_status = daemon_status_file_load();
    daemon_status_file_save(DAEMON_STATUS_INITIALIZING);
    ND_LOG_FIELD_PRIORITY pri = NDLP_NOTICE;

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
            if(last_session_status.reason == EXIT_REASON_NONE) {
                cause = "exit no reason";
                msg = "Netdata was last stopped gracefully (no exit reason set)";
                if(!last_session_status.timestamp_ut)
                    dump_json = false;
            }
            else if(!is_exit_reason_normal(last_session_status.reason)) {
                cause = "exit on fatal";
                msg = "Netdata was last stopped gracefully (encountered an error)";
                pri = NDLP_ERR;
                post_crash_report = true;
            }
            else if(last_session_status.reason & EXIT_REASON_SYSTEM_SHUTDOWN) {
                cause = "exit on system shutdown";
                msg = "Netdata has gracefully stopped due to system shutdown";
            }
            else if(last_session_status.reason & EXIT_REASON_UPDATE) {
                cause = "exit to update";
                msg = "Netdata has gracefully restarted to update to a new version";
            }
            else if(new_version) {
                cause = "exit and updated";
                msg = "Netdata has gracefully restarted and updated to a new version";
                last_session_status.reason |= EXIT_REASON_UPDATE;
            }
            else {
                cause = "exit instructed";
                msg = "Netdata was last stopped gracefully (instructed to do so)";
            }
            break;

        case DAEMON_STATUS_INITIALIZING:
            cause = "crashed on start";
            msg = "Netdata was last killed/crashed while starting";
            pri = NDLP_ERR;
            post_crash_report = true;
            break;

        case DAEMON_STATUS_EXITING:
            if(!is_exit_reason_normal(last_session_status.reason)) {
                cause = "crashed on fatal";
                msg = "Netdata was last killed/crashed while exiting after encountering an error";
            }
            else if(last_session_status.reason & EXIT_REASON_SYSTEM_SHUTDOWN) {
                cause = "crashed on system shutdown";
                msg = "Netdata was last killed/crashed while exiting due to system shutdown";
            }
            else if(new_version || (last_session_status.reason & EXIT_REASON_UPDATE)) {
                cause = "crashed on update";
                msg = "Netdata was last killed/crashed while exiting to update to a new version";
            }
            else {
                cause = "crashed on exit";
                msg = "Netdata was last killed/crashed while exiting (instructed to do so)";
            }
            pri = NDLP_ERR;
            post_crash_report = true;
            break;

        case DAEMON_STATUS_RUNNING: {
            if (!UUIDeq(session_status.boot_id, last_session_status.boot_id)) {
                cause = "abnormal power off";
                msg = "The system was abnormally powered off while Netdata was running";
                pri = NDLP_CRIT;
            }
            else {
                cause = "killed hard";
                msg = "Netdata was last killed/crashed while operating normally";
                pri = NDLP_CRIT;
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

    nd_log(NDLS_DAEMON, pri,
           "Netdata Agent version '%s' is starting...\n"
           "Last exit status: %s (%s):\n\n%s",
           NETDATA_VERSION, msg, cause, buffer_tostring(wb));

    if(!disable_crash_report && (analytics_check_enabled() || post_crash_report)) {
        netdata_conf_ssl();

        struct post_status_file_thread_data *d = calloc(1, sizeof(*d));
        d->cause = strdupz(cause);
        d->msg = strdupz(msg);
        d->status = last_session_status;
        d->priority = pri;
        nd_thread_create("post_status_file", NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_DEFAULT, post_status_file_thread, d);
    }
}

bool daemon_status_file_has_last_crashed(void) {
    return last_session_status.status != DAEMON_STATUS_EXITED || !is_exit_reason_normal(last_session_status.reason);
}

bool daemon_status_file_was_incomplete_shutdown(void) {
    return last_session_status.status == DAEMON_STATUS_EXITING;
}

void daemon_status_file_register_fatal(const char *filename, const char *function, const char *message, const char *stack_trace, long line) {
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    spinlock_lock(&spinlock);

    if(session_status.fatal.filename || session_status.fatal.function || session_status.fatal.message || session_status.fatal.stack_trace) {
        spinlock_unlock(&spinlock);
        freez((void *)filename);
        freez((void *)function);
        freez((void *)message);
        freez((void *)stack_trace);
        return;
    }

    session_status.fatal.filename = filename;
    session_status.fatal.function = function;
    session_status.fatal.message = message;
    session_status.fatal.stack_trace = stack_trace;
    session_status.fatal.line = line;

    spinlock_unlock(&spinlock);
}
