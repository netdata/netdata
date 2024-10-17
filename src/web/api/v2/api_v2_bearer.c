// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v2_calls.h"

static bool verify_host_uuids(RRDHOST *host, const char *machine_guid, const char *node_id) {
    if(!machine_guid || !node_id)
        return false;

    if(strcmp(machine_guid, host->machine_guid) != 0)
        return false;

    if(UUIDiszero(host->node_id))
        return false;

    char buf[UUID_STR_LEN];
    uuid_unparse_lower(host->node_id.uuid, buf);

    return strcmp(node_id, buf) == 0;
}

int api_v2_bearer_protection(RRDHOST *host __maybe_unused, struct web_client *w __maybe_unused, char *url) {
    char *machine_guid = NULL;
    char *claim_id = NULL;
    char *node_id = NULL;
    bool protection = netdata_is_protected_by_bearer;

    while (url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if (!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if (!name || !*name) continue;
        if (!value || !*value) continue;

        if(!strcmp(name, "bearer_protection")) {
            if(!strcmp(value, "on") || !strcmp(value, "true") || !strcmp(value, "yes"))
                protection = true;
            else
                protection = false;
        }
        else if(!strcmp(name, "machine_guid"))
            machine_guid = value;
        else if(!strcmp(name, "claim_id"))
            claim_id = value;
        else if(!strcmp(name, "node_id"))
            node_id = value;
    }

    if(!claim_id_matches(claim_id)) {
        buffer_reset(w->response.data);
        buffer_strcat(w->response.data, "The request is for a different claimed agent");
        return HTTP_RESP_BAD_REQUEST;
    }

    if(!verify_host_uuids(localhost, machine_guid, node_id)) {
        buffer_reset(w->response.data);
        buffer_strcat(w->response.data, "The request is missing or not matching local UUIDs");
        return HTTP_RESP_BAD_REQUEST;
    }

    netdata_is_protected_by_bearer = protection;

    BUFFER *wb = w->response.data;
    buffer_reset(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_boolean(wb, "bearer_protection", netdata_is_protected_by_bearer);
    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}

int bearer_get_token_json_response(BUFFER *wb, RRDHOST *host, const char *claim_id, const char *machine_guid, const char *node_id, HTTP_USER_ROLE user_role, HTTP_ACCESS access, nd_uuid_t cloud_account_id, const char *client_name) {
    if(!claim_id_matches_any(claim_id))
        return rrd_call_function_error(wb, "The request is for a different agent", HTTP_RESP_BAD_REQUEST);

    if(!verify_host_uuids(host, machine_guid, node_id))
        return rrd_call_function_error(wb, "The request is missing or not matching local node UUIDs", HTTP_RESP_BAD_REQUEST);

    nd_uuid_t uuid;
    time_t expires_s = bearer_create_token(&uuid, user_role, access, cloud_account_id, client_name);

    buffer_reset(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_int64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "mg", host->machine_guid);
    buffer_json_member_add_boolean(wb, "bearer_protection", netdata_is_protected_by_bearer);
    buffer_json_member_add_uuid(wb, "token", uuid);
    buffer_json_member_add_time_t(wb, "expiration", expires_s);
    buffer_json_finalize(wb);
    return HTTP_RESP_OK;
}

int api_v2_bearer_get_token(RRDHOST *host, struct web_client *w, char *url) {
    char *machine_guid = NULL;
    char *claim_id = NULL;
    char *node_id = NULL;

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if (!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if (!name || !*name) continue;
        if (!value || !*value) continue;

        if(!strcmp(name, "machine_guid"))
            machine_guid = value;
        else if(!strcmp(name, "claim_id"))
            claim_id = value;
        else if(!strcmp(name, "node_id"))
            node_id = value;
    }

    if(!claim_id_matches(claim_id)) {
        buffer_reset(w->response.data);
        buffer_strcat(w->response.data, "The request is for a different claimed agent");
        return HTTP_RESP_BAD_REQUEST;
    }

    if(!verify_host_uuids(host, machine_guid, node_id)) {
        buffer_reset(w->response.data);
        buffer_strcat(w->response.data, "The request is missing or not matching local UUIDs");
        return HTTP_RESP_BAD_REQUEST;
    }

     if(host != localhost)
        return call_function_bearer_get_token(host, w, claim_id, machine_guid, node_id);

    return bearer_get_token_json_response(
        w->response.data,
        host,
        claim_id,
        machine_guid,
        node_id,
        w->user_role,
        w->access,
        w->auth.cloud_account_id,
        w->auth.client_name);
}
