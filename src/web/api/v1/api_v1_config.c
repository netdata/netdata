// SPDX-License-Identifier: GPL-3.0-or-later

#include "web/api/v2/api_v2_calls.h"

int api_v1_config(RRDHOST *host, struct web_client *w, char *url __maybe_unused) {
    char *action = "tree";
    char *path = "/";
    char *id = NULL;
    char *add_name = NULL;
    int timeout = 120;

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "action"))
            action = value;
        else if(!strcmp(name, "path"))
            path = value;
        else if(!strcmp(name, "id"))
            id = value;
        else if(!strcmp(name, "name"))
            add_name = value;
        else if(!strcmp(name, "timeout")) {
            timeout = (int)strtol(value, NULL, 10);
            if(timeout < 10)
                timeout = 10;
        }
    }

    char transaction[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(w->transaction, transaction);

    size_t len = strlen(action) + (id ? strlen(id) : 0) + strlen(path) + (add_name ? strlen(add_name) : 0) + 100;

    char cmd[len];
    if(strcmp(action, "tree") == 0)
        snprintfz(cmd, sizeof(cmd), PLUGINSD_FUNCTION_CONFIG " tree '%s' '%s'", path, id?id:"");
    else {
        DYNCFG_CMDS c = dyncfg_cmds2id(action);
        if(!id || !*id || !dyncfg_is_valid_id(id)) {
            rrd_call_function_error(w->response.data, "Invalid id", HTTP_RESP_BAD_REQUEST);
            return HTTP_RESP_BAD_REQUEST;
        }

        if(c == DYNCFG_CMD_NONE) {
            rrd_call_function_error(w->response.data, "Invalid action", HTTP_RESP_BAD_REQUEST);
            return HTTP_RESP_BAD_REQUEST;
        }

        if(c == DYNCFG_CMD_ADD || c == DYNCFG_CMD_USERCONFIG || c == DYNCFG_CMD_TEST) {
            if(c == DYNCFG_CMD_TEST && (!add_name || !*add_name)) {
                // backwards compatibility for TEST without a name
                char *colon = strrchr(id, ':');
                if(colon) {
                    *colon = '\0';
                    add_name = ++colon;
                }
                else
                    add_name = "test";
            }

            if(!add_name || !*add_name || !dyncfg_is_valid_id(add_name)) {
                rrd_call_function_error(w->response.data, "Invalid name", HTTP_RESP_BAD_REQUEST);
                return HTTP_RESP_BAD_REQUEST;
            }
            snprintfz(cmd, sizeof(cmd), PLUGINSD_FUNCTION_CONFIG " %s %s %s", id, dyncfg_id2cmd_one(c), add_name);
        }
        else
            snprintfz(cmd, sizeof(cmd), PLUGINSD_FUNCTION_CONFIG " %s %s", id, dyncfg_id2cmd_one(c));
    }

    CLEAN_BUFFER *source = buffer_create(100, NULL);
    web_client_api_request_vX_source_to_buffer(w, source);

    buffer_flush(w->response.data);
    int code = rrd_function_run(host, w->response.data, timeout, w->access, cmd,
                                true, transaction,
                                NULL, NULL,
                                web_client_progress_functions_update, w,
                                web_client_interrupt_callback, w,
                                w->payload, buffer_tostring(source), false);

    return code;
}
