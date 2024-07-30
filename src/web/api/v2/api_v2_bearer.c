// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v2_calls.h"

static bool verify_agent_uuids(const char *machine_guid, const char *node_id, const char *claim_id) {
    if(!machine_guid || !node_id || !claim_id)
        return false;

    if(strcmp(machine_guid, localhost->machine_guid) != 0)
        return false;

    char *agent_claim_id = aclk_get_claimed_id();

    bool not_verified = (!agent_claim_id || strcmp(claim_id, agent_claim_id) != 0);
    freez(agent_claim_id);

    if(not_verified || uuid_is_null(localhost->node_id))
        return false;

    char buf[UUID_STR_LEN];
    uuid_unparse_lower(localhost->node_id, buf);

    if(strcmp(node_id, buf) != 0)
        return false;

    return true;
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

    if(!verify_agent_uuids(machine_guid, node_id, claim_id)) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "The request is missing or not matching local UUIDs");
        return HTTP_RESP_BAD_REQUEST;
    }

    netdata_is_protected_by_bearer = protection;

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_boolean(wb, "bearer_protection", netdata_is_protected_by_bearer);
    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}

int api_v2_bearer_token(RRDHOST *host __maybe_unused, struct web_client *w __maybe_unused, char *url __maybe_unused) {
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

    if(!verify_agent_uuids(machine_guid, node_id, claim_id)) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "The request is missing or not matching local UUIDs");
        return HTTP_RESP_BAD_REQUEST;
    }

    nd_uuid_t uuid;
    time_t expires_s = bearer_create_token(&uuid, w);

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_string(wb, "mg", localhost->machine_guid);
    buffer_json_member_add_boolean(wb, "bearer_protection", netdata_is_protected_by_bearer);
    buffer_json_member_add_uuid(wb, "token", &uuid);
    buffer_json_member_add_time_t(wb, "expiration", expires_s);
    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}
