// SPDX-License-Identifier: GPL-3.0-or-later

#include "function-bearer_get_token.h"
#include "../v2/api_v2_calls.h"

struct bearer_token_request {
    nd_uuid_t claim_id;
    nd_uuid_t machine_guid;
    nd_uuid_t node_id;
    HTTP_USER_ROLE user_role;
    HTTP_ACCESS access;
    nd_uuid_t cloud_account_id;
    STRING *client_name;
};

static bool bearer_parse_json_payload(json_object *jobj, void *data, BUFFER *error) {
    const char *path = "";
    struct bearer_token_request *rq = data;
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "claim_id", rq->claim_id, error, true);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "machine_guid", rq->machine_guid, error, true);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "node_id", rq->node_id, error, true);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "user_role", http_user_role2id, rq->user_role, error, true);
    JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, "access", http_access2id_one, rq->access, error, true);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "cloud_account_id", rq->cloud_account_id, error, true);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "client_name", rq->client_name, error, true);
    return true;
}

int function_bearer_get_token(BUFFER *wb, const char *function __maybe_unused, BUFFER *payload, const char *source) {
    if(!request_source_is_cloud(source))
        return rrd_call_function_error(
            wb, "Bearer tokens can only be provided via NC.", HTTP_RESP_BAD_REQUEST);

    int code;
    struct bearer_token_request rq = { 0 };
    CLEAN_JSON_OBJECT *jobj = json_parse_function_payload_or_error(wb, payload, &code, bearer_parse_json_payload, &rq);
    if(!jobj || code != HTTP_RESP_OK) {
        string_freez(rq.client_name);
        return code;
    }

    char claim_id[UUID_STR_LEN];
    uuid_unparse_lower(rq.claim_id, claim_id);

    char machine_guid[UUID_STR_LEN];
    uuid_unparse_lower(rq.machine_guid, machine_guid);

    char node_id[UUID_STR_LEN];
    uuid_unparse_lower(rq.node_id, node_id);

    int rc = bearer_get_token_json_response(wb, localhost, claim_id, machine_guid, node_id,
                                            rq.user_role, rq.access, rq.cloud_account_id,
                                            string2str(rq.client_name));

    string_freez(rq.client_name);
    return rc;
}

int call_function_bearer_get_token(RRDHOST *host, struct web_client *w, const char *claim_id, const char *machine_guid, const char *node_id) {
    CLEAN_BUFFER *payload = buffer_create(0, NULL);
    buffer_json_initialize(payload, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_string(payload, "claim_id", claim_id);
    buffer_json_member_add_string(payload, "machine_guid", machine_guid);
    buffer_json_member_add_string(payload, "node_id", node_id);
    buffer_json_member_add_string(payload, "user_role", http_id2user_role(w->user_role));
    http_access2buffer_json_array(payload, "access", w->access);
    buffer_json_member_add_uuid(payload, "cloud_account_id", w->auth.cloud_account_id);
    buffer_json_member_add_string(payload, "client_name", w->auth.client_name);
    buffer_json_finalize(payload);

    CLEAN_BUFFER *source = buffer_create(0, NULL);
    web_client_api_request_vX_source_to_buffer(w, source);

    char transaction_str[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(w->transaction, transaction_str);
    return rrd_function_run(host, w->response.data, 10,
                            w->access, RRDFUNCTIONS_BEARER_GET_TOKEN, true,
                            transaction_str, NULL, NULL,
                            NULL, NULL,
                            NULL, NULL,
                            payload, buffer_tostring(source), true);
}
