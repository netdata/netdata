// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"

#define JOURNAL_DIRECTORIES_JSON_NODE "journalDirectories"

static int systemd_journal_directories_dyncfg_update(BUFFER *result, BUFFER *payload) {
    if(!payload || !buffer_strlen(payload))
        return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "empty payload received");

    CLEAN_JSON_OBJECT *jobj = json_tokener_parse(buffer_tostring(payload));
    if(!jobj)
        return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "cannot parse json payload");

    struct json_object *journalDirectories;
    json_object_object_get_ex(jobj, JOURNAL_DIRECTORIES_JSON_NODE, &journalDirectories);

    size_t n_directories = json_object_array_length(journalDirectories);

    size_t added = 0;
    for(size_t i = 0; i < n_directories; i++) {
        struct json_object *dir = json_object_array_get_idx(journalDirectories, i);
        const char *s = json_object_get_string(dir);
        if(s && *s) {
            string_freez(journal_directories[added].path);
            journal_directories[added++].path = string_strdupz(s);
        }
    }

    if(!added)
        return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "no directories in the payload");
    else {
        for(size_t i = added; i < MAX_JOURNAL_DIRECTORIES; i++) {
            string_freez(journal_directories[i].path);
            journal_directories[i].path = NULL;
        }
    }

    return dyncfg_default_response(result, HTTP_RESP_OK, "applied");
}

static int systemd_journal_directories_dyncfg_get(BUFFER *wb) {
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    buffer_json_member_add_array(wb, JOURNAL_DIRECTORIES_JSON_NODE);
    for(size_t i = 0; i < MAX_JOURNAL_DIRECTORIES ;i++) {
        if(!journal_directories[i].path)
            break;

        buffer_json_add_array_item_string(wb, string2str(journal_directories[i].path));
    }
    buffer_json_array_close(wb);

    buffer_json_finalize(wb);
    return HTTP_RESP_OK;
}

static int systemd_journal_directories_dyncfg_cb(const char *transaction,
                                                 const char *id,
                                                 DYNCFG_CMDS cmd,
                                                 const char *add_name __maybe_unused,
                                                 BUFFER *payload,
                                                 usec_t *stop_monotonic_ut __maybe_unused,
                                                 bool *cancelled __maybe_unused,
                                                 BUFFER *result,
                                                 HTTP_ACCESS access __maybe_unused,
                                                 const char *source __maybe_unused,
                                                 void *data __maybe_unused) {
    CLEAN_BUFFER *action = buffer_create(100, NULL);
    dyncfg_cmds2buffer(cmd, action);

    if(cmd == DYNCFG_CMD_GET)
        return systemd_journal_directories_dyncfg_get(result);

    if(cmd == DYNCFG_CMD_UPDATE)
        return systemd_journal_directories_dyncfg_update(result, payload);

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "DYNCFG: unhandled transaction '%s', id '%s' cmd '%s', payload: %s",
           transaction, id, buffer_tostring(action), payload ? buffer_tostring(payload) : "");

    return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "the command is not handled by this plugin");
}

// ----------------------------------------------------------------------------

void systemd_journal_dyncfg_init(struct functions_evloop_globals *wg) {
    functions_evloop_dyncfg_add(
        wg,
        "systemd-journal:monitored-directories",
        "/collectors/logs/systemd-journal",
        DYNCFG_STATUS_RUNNING,
        DYNCFG_TYPE_SINGLE,
        DYNCFG_SOURCE_TYPE_INTERNAL,
        "internal",
        DYNCFG_CMD_SCHEMA | DYNCFG_CMD_GET | DYNCFG_CMD_UPDATE,
        HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_VIEW_AGENT_CONFIG,
        HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_EDIT_AGENT_CONFIG,
        systemd_journal_directories_dyncfg_cb,
        NULL);
}
