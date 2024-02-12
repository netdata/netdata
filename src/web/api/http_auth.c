// SPDX-License-Identifier: GPL-3.0-or-later

#include "http_auth.h"

#define BEARER_TOKEN_EXPIRATION 86400

bool netdata_is_protected_by_bearer = false; // this is controlled by cloud, at the point the agent logs in - this should also be saved to /var/lib/netdata
static DICTIONARY *netdata_authorized_bearers = NULL;

struct bearer_token {
    uuid_t cloud_account_id;
    char cloud_user_name[CLOUD_USER_NAME_LENGTH];
    HTTP_ACCESS access;
    HTTP_USER_ROLE user_role;
    time_t created_s;
    time_t expires_s;
};

bool web_client_bearer_token_auth(struct web_client *w, const char *v) {
    if(!uuid_parse_flexi(v, w->auth.bearer_token)) {
        char uuid_str[UUID_COMPACT_STR_LEN];
        uuid_unparse_lower_compact(w->auth.bearer_token, uuid_str);

        struct bearer_token *z = dictionary_get(netdata_authorized_bearers, uuid_str);
        if (z && z->expires_s > now_monotonic_sec()) {
            strncpyz(w->auth.client_name, z->cloud_user_name, sizeof(w->auth.client_name) - 1);
            uuid_copy(w->auth.cloud_account_id, z->cloud_account_id);
            web_client_set_permissions(w, z->access, z->user_role, WEB_CLIENT_FLAG_AUTH_BEARER);
            return true;
        }
    }
    else
        nd_log(NDLS_DAEMON, NDLP_NOTICE, "Invalid bearer token '%s' received.", v);

    return false;
}

static void bearer_token_cleanup(void) {
    static time_t attempts = 0;

    if(++attempts % 1000 != 0)
        return;

    time_t now_s = now_monotonic_sec();

    struct bearer_token *z;
    dfe_start_read(netdata_authorized_bearers, z) {
        if(z->expires_s < now_s)
            dictionary_del(netdata_authorized_bearers, z_dfe.name);
    }
    dfe_done(z);

    dictionary_garbage_collect(netdata_authorized_bearers);
}

void bearer_tokens_init(void) {
    netdata_authorized_bearers = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
        NULL, sizeof(struct bearer_token));
}

time_t bearer_create_token(uuid_t *uuid, struct web_client *w) {
    char uuid_str[UUID_COMPACT_STR_LEN];

    uuid_generate_random(*uuid);
    uuid_unparse_lower_compact(*uuid, uuid_str);

    struct bearer_token t = { 0 }, *z;
    z = dictionary_set(netdata_authorized_bearers, uuid_str, &t, sizeof(t));
    if(!z->created_s) {
        z->created_s = now_monotonic_sec();
        z->expires_s = z->created_s + BEARER_TOKEN_EXPIRATION;
        z->user_role = w->user_role;
        z->access = w->access;
        uuid_copy(z->cloud_account_id, w->auth.cloud_account_id);
        strncpyz(z->cloud_user_name, w->auth.client_name, sizeof(z->cloud_account_id) - 1);
    }

    bearer_token_cleanup();

    return now_realtime_sec() + BEARER_TOKEN_EXPIRATION;
}

bool extract_bearer_token_from_request(struct web_client *w, char *dst, size_t dst_len) {
    if(!web_client_flag_check(w, WEB_CLIENT_FLAG_AUTH_BEARER) || dst_len != UUID_STR_LEN)
        return false;

    uuid_unparse_lower(w->auth.bearer_token, dst);
    return true;
}
