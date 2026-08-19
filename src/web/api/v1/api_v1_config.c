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

    char *cmd = mallocz(len);
    if(strcmp(action, "tree") == 0)
        snprintfz(cmd, len, PLUGINSD_FUNCTION_CONFIG " tree '%s' '%s'", path, id?id:"");
    else {
        DYNCFG_CMDS c = dyncfg_cmds2id(action);
        if(!id || !*id || !dyncfg_is_valid_id(id)) {
            nrpc_call_error(w->response.data, "Invalid id", HTTP_RESP_BAD_REQUEST);
            freez(cmd);
            return HTTP_RESP_BAD_REQUEST;
        }

        if(c == DYNCFG_CMD_NONE) {
            nrpc_call_error(w->response.data, "Invalid action", HTTP_RESP_BAD_REQUEST);
            freez(cmd);
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
                nrpc_call_error(w->response.data, "Invalid name", HTTP_RESP_BAD_REQUEST);
                freez(cmd);
                return HTTP_RESP_BAD_REQUEST;
            }
            snprintfz(cmd, len, PLUGINSD_FUNCTION_CONFIG " %s %s %s", id, dyncfg_id2cmd_one(c), add_name);
        }
        else
            snprintfz(cmd, len, PLUGINSD_FUNCTION_CONFIG " %s %s", id, dyncfg_id2cmd_one(c));
    }

    CLEAN_BUFFER *source = buffer_create(100, NULL);
    user_auth_to_source_buffer(&w->user_auth, source);

    buffer_flush(w->response.data);
    int code = nrpc_call(&(struct nrpc_call_spec) {
        .owner = host ? rrdhost_nrpc_owner(host) : NRPC_OWNER_NONE,
        .result_wb = w->response.data,
        .cmd = cmd,
        .source = buffer_tostring(source),
        .user_access = w->user_auth.access,
        .timeout_s = timeout,
        .wait = true,
        .allow_restricted = false,
        .call_id = transaction,
        .payload = w->payload,
        .progress.cb = web_client_progress_functions_update,
        .is_cancelled.cb = web_client_interrupt_callback,
        .is_cancelled.data = w,
    });

    freez(cmd);
    return code;
}
