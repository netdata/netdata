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

ENUM_STR_DEFINE_FUNCTIONS(DAEMON_STATUS, DAEMON_STATUS_NONE, "none");

bool json_parse_daemon_status_file_payload(json_object *jobj, const char *path, void *data, BUFFER *error) {
    DAEMON_STATUS_FILE *ds = data;
    JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, "version", ds->version, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "boottime", ds->boottime, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "uptime", ds->uptime, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "timestamp", ds->timestamp, error, false);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "invocation", ds->invocation.uuid, error, false);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "status", DAEMON_STATUS_2id, ds->status, error, false);
    JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, "reason", EXIT_REASON_2id_one, ds->reason, error, false);
    return true;
}

DAEMON_STATUS_FILE daemon_status_file_load(void) {
    DAEMON_STATUS_FILE status = {
        .boottime = 0,
        .uptime = 0,
        .timestamp = 0,
        .invocation = {0},
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
        json_parse_payload_or_error(wb, error, json_parse_daemon_status_file_payload, &status);
    }

    return status;
}

DAEMON_STATUS_FILE daemon_status_file_get(DAEMON_STATUS status) {
    static time_t netdata_boottime_time = 0;
    if (!netdata_boottime_time)
        netdata_boottime_time = now_boottime_sec();

    DAEMON_STATUS_FILE rc = {
        .version = NETDATA_VERSION,
        .boottime = now_boottime_sec(),
        .uptime = now_boottime_sec() - netdata_boottime_time,
        .timestamp = now_realtime_sec(),
        .invocation = nd_log_get_invocation_id(),
        .reason = exit_initiated,
        .status = status,
    };

    return rc;
}

void daemon_status_file_save(DAEMON_STATUS status) {
    DAEMON_STATUS_FILE ds = daemon_status_file_get(status);

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    {
        buffer_json_member_add_string(wb, "version", ds.version);
        buffer_json_member_add_time_t(wb, "boottime", ds.boottime);
        buffer_json_member_add_time_t(wb, "uptime", ds.uptime);
        buffer_json_member_add_time_t(wb, "timestamp", ds.timestamp);
        buffer_json_member_add_uuid_compact(wb, "invocation", ds.invocation.uuid);
        buffer_json_member_add_string(wb, "status", DAEMON_STATUS_2str(ds.status));
        EXIT_REASON_2json(wb, "reason", ds.reason);
    }
    buffer_json_finalize(wb);

    char filename[FILENAME_MAX];
    snprintfz(filename, sizeof(filename), "%s/.status.%s", netdata_configured_varlib_dir, program_name);

    FILE *fp = fopen(filename, "w");
    if (fp) {
        fwrite(buffer_tostring(wb), 1, buffer_strlen(wb), fp);
        fclose(fp);
    }
}

static DAEMON_STATUS_FILE last_session_status = { 0 };

void daemon_status_file_check_crash(void) {
    last_session_status = daemon_status_file_load();;

    switch(last_session_status.status) {
        case DAEMON_STATUS_NONE:
            // probably a previous version of netdata was running
            break;

        case DAEMON_STATUS_EXITED:
            // netdata exited gracefully
            if(!is_exit_reason_normal(last_session_status.reason)) {
                // it did not exit normally
            }
            break;

        case DAEMON_STATUS_INITIALIZING:
        case DAEMON_STATUS_EXITING:
        case DAEMON_STATUS_RUNNING:
            // this is a crash!
            break;
    }
}

bool daemon_status_file_has_last_crashed(void) {
    return last_session_status.status != DAEMON_STATUS_EXITED || !is_exit_reason_normal(last_session_status.reason);
}

bool daemon_status_file_was_incomplete_shutdown(void) {
    return last_session_status.status == DAEMON_STATUS_EXITING;
}
