// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "daemon-status-file.h"

ENUM_STR_MAP_DEFINE(DAEMON_STATUS) = {
    { DAEMON_STATUS_NONE, "none"},
    { DAEMON_STATUS_INITIALIZING, "initializing"},
    { DAEMON_STATUS_RUNNING, "running"},
    { DAEMON_STATUS_EXITING, "exiting"},
    { DAEMON_STATUS_EXITED, "exited"},

    // terminator
    { 0, NULL },
};

static DAEMON_STATUS_FILE last_session_status = { 0 };
static DAEMON_STATUS_FILE session_status = { 0 };

ENUM_STR_DEFINE_FUNCTIONS(DAEMON_STATUS, DAEMON_STATUS_NONE, "none");

static void daemon_status_file_to_json(BUFFER *wb, DAEMON_STATUS_FILE *ds) {
    buffer_json_member_add_string(wb, "version", ds->version);
    buffer_json_member_add_time_t(wb, "boottime", ds->boottime);
    buffer_json_member_add_time_t(wb, "uptime", ds->uptime);
    buffer_json_member_add_time_t(wb, "timestamp", ds->timestamp);
    buffer_json_member_add_uuid_compact(wb, "invocation", ds->invocation.uuid);
    buffer_json_member_add_uuid(wb, "machine_guid", ds->machine_guid.uuid);
    buffer_json_member_add_string(wb, "status", DAEMON_STATUS_2str(ds->status));
    EXIT_REASON_2json(wb, "reason", ds->reason);

    buffer_json_member_add_time_t(wb, "init_dt", ds->init_dt);
    buffer_json_member_add_time_t(wb, "exit_dt", ds->exit_dt);
    buffer_json_member_add_uint64(wb, "ram_total", ds->memory.ram_total_bytes);
    buffer_json_member_add_uint64(wb, "ram_available", ds->memory.ram_available_bytes);

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

static bool daemon_status_file_from_json_fatal(json_object *jobj, const char *path, DAEMON_STATUS_FILE *ds, BUFFER *error, bool required __maybe_unused) {
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "filename", ds->fatal.filename, error, false);
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "function", ds->fatal.function, error, false);
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "message", ds->fatal.message, error, false);
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "stack_trace", ds->fatal.stack_trace, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "line", ds->fatal.line, error, false);
    return true;
}

static bool daemon_status_file_from_json(json_object *jobj, const char *path, void *data, BUFFER *error) {
    DAEMON_STATUS_FILE *ds = data;
    JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "version", ds->version, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "boottime", ds->boottime, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "uptime", ds->uptime, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "timestamp", ds->timestamp, error, false);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "invocation", ds->invocation.uuid, error, false);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "machine_guid", ds->machine_guid.uuid, error, false);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "status", DAEMON_STATUS_2id, ds->status, error, false);
    JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, "reason", EXIT_REASON_2id_one, ds->reason, error, false);

    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "init_dt", ds->init_dt, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "exit_dt", ds->exit_dt, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "ram_total", ds->memory.ram_total_bytes, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "ram_available", ds->memory.ram_available_bytes, error, false);

    JSONC_PARSE_SUBOBJECT(jobj, path, "fatal", ds, daemon_status_file_from_json_fatal, error, false);
    return true;
}

static DAEMON_STATUS_FILE daemon_status_file_get(DAEMON_STATUS status) {
    static time_t netdata_boottime_time = 0;
    if (!netdata_boottime_time)
        netdata_boottime_time = now_boottime_sec();

    time_t now = now_realtime_sec();

    if(session_status.status == DAEMON_STATUS_INITIALIZING && status == DAEMON_STATUS_RUNNING)
        session_status.init_dt = now - session_status.timestamp;

    if(session_status.status == DAEMON_STATUS_EXITING && status == DAEMON_STATUS_EXITED)
        session_status.exit_dt = now - session_status.timestamp;

    strncpyz(session_status.version, NETDATA_VERSION, sizeof(session_status.version) - 1);
    session_status.boottime = now_boottime_sec();
    session_status.uptime = now_boottime_sec() - netdata_boottime_time;
    session_status.timestamp = now;
    session_status.invocation = nd_log_get_invocation_id();

    if(localhost)
        session_status.machine_guid = localhost->host_id;
    else if(!UUIDiszero(last_session_status.machine_guid))
        session_status.machine_guid = last_session_status.machine_guid;
    else {
        const char *machine_guid = registry_get_this_machine_guid();
        if(machine_guid && *machine_guid)
            uuid_parse_flexi(machine_guid, session_status.machine_guid.uuid);
        else
            session_status.machine_guid = UUID_ZERO;
    }

    session_status.reason = exit_initiated;
    session_status.status = status;
    session_status.memory = os_system_memory(true);

    return session_status;
}

DAEMON_STATUS_FILE daemon_status_file_load(void) {
    DAEMON_STATUS_FILE status = {
        .boottime = 0,
        .uptime = 0,
        .timestamp = 0,
        .invocation = UUID_ZERO,
        .reason = EXIT_REASON_NONE,
        .status = DAEMON_STATUS_NONE,
    };

    char filename[FILENAME_MAX];
    snprintfz(filename, sizeof(filename), "%s/.status.%s", netdata_configured_varlib_dir, program_name);
    FILE *fp = fopen(filename, "r");
    if (fp) {
        fseek(fp, 0, SEEK_END);
        long file_size = ftell(fp);
        fseek(fp, 0, SEEK_SET);

        CLEAN_BUFFER *wb = buffer_create(file_size + 1, NULL);
        buffer_need_bytes(wb, file_size + 1);

        size_t read_bytes = fread(wb->buffer, 1, file_size, fp);
        fclose(fp);

        wb->buffer[read_bytes] = '\0';
        wb->len = read_bytes;

        CLEAN_BUFFER *error = buffer_create(0, NULL);
        json_parse_payload_or_error(wb, error, daemon_status_file_from_json, &status);
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

    char temp_filename[FILENAME_MAX];
    snprintfz(temp_filename, sizeof(temp_filename), "%s/.status.%s.tmp", netdata_configured_varlib_dir, program_name);

    FILE *fp = fopen(temp_filename, "w");
    if (fp) {
        bool ok = fwrite(buffer_tostring(wb), 1, buffer_strlen(wb), fp) == buffer_strlen(wb);
        fclose(fp);

        if(ok) {
            char filename[FILENAME_MAX];
            snprintfz(filename, sizeof(filename), "%s/.status.%s", netdata_configured_varlib_dir, program_name);
            rename(temp_filename, filename);
        }
    }

    spinlock_unlock(&spinlock);
}

void daemon_status_file_check_crash(void) {
    last_session_status = daemon_status_file_load();

    bool log = false;
    switch(last_session_status.status) {
        case DAEMON_STATUS_NONE:
            // probably a previous version of netdata was running
            break;

        case DAEMON_STATUS_EXITED:
            // netdata exited gracefully
            if(!is_exit_reason_normal(last_session_status.reason)) {
                // it did not exit normally
                log = true;
            }
            break;

        case DAEMON_STATUS_INITIALIZING:
        case DAEMON_STATUS_EXITING:
        case DAEMON_STATUS_RUNNING:
            // this is a crash!
            log = true;
            break;
    }

    if(log) {
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
        daemon_status_file_to_json(wb, &last_session_status);
        buffer_json_finalize(wb);

        nd_log(NDLS_DAEMON, NDLP_ERR,
               "LAST SESSION CRASH: Netdata crashed the last time it was running!\n\n%s",
               buffer_tostring(wb));
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
