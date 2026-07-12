// SPDX-License-Identifier: GPL-3.0-or-later

#include "http_auth.h"
#include "web/api/mcp_auth.h"

#define BEARER_TOKEN_EXPIRATION (86400 * 1)

bool netdata_is_protected_by_bearer = false;
static DICTIONARY *netdata_authorized_bearers = NULL;

struct bearer_token {
    nd_uuid_t cloud_account_id;
    char client_name[CLOUD_CLIENT_NAME_LENGTH];
    HTTP_ACCESS access;
    HTTP_USER_ROLE user_role;
    time_t created_s;
    time_t expires_s;
};

static void bearer_token_insert_callback(
    const DICTIONARY_ITEM *item __maybe_unused, void *value __maybe_unused, void *data) {
    bool *inserted = data;
    if(inserted)
        *inserted = true;
}

static DICTIONARY *bearer_tokens_dictionary_create(void) {
    DICTIONARY *dict = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
        NULL, sizeof(struct bearer_token));
    dictionary_register_insert_callback(dict, bearer_token_insert_callback, NULL);
    return dict;
}

static void bearer_tokens_path(char out[FILENAME_MAX]) {
    filename_from_path_entry(out, netdata_configured_varlib_dir, "bearer_tokens", NULL);
}

static void bearer_token_filename(char out[FILENAME_MAX], nd_uuid_t uuid) {
    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower(uuid, uuid_str);

    char path[FILENAME_MAX];
    bearer_tokens_path(path);
    filename_from_path_entry(out, path, uuid_str, NULL);
}

static inline bool bearer_tokens_ensure_path_exists(void) {
    char path[FILENAME_MAX];
    bearer_tokens_path(path);
    return filename_is_dir(path, true);
}

static void bearer_token_delete_from_disk(nd_uuid_t *token) {
    char filename[FILENAME_MAX];
    bearer_token_filename(filename, *token);
    if(unlink(filename) != 0)
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to unlink() file '%s'", filename);
}

static void bearer_token_cleanup(bool force) {
    static uint32_t cleanup_attempts = 0;

    uint32_t attempts = __atomic_add_fetch(&cleanup_attempts, 1, __ATOMIC_RELAXED);
    if(attempts % 1000 != 0 && !force)
        return;

    time_t now_s = now_realtime_sec();

    struct bearer_token *z;
    dfe_start_write(netdata_authorized_bearers, z) {
        if(z->expires_s < now_s) {
            nd_uuid_t uuid;
            if(uuid_parse_flexi(z_dfe.name, uuid) == 0)
                bearer_token_delete_from_disk(&uuid);

            dictionary_del(netdata_authorized_bearers, z_dfe.name);
        }
    }
    dfe_done(z);

    dictionary_garbage_collect(netdata_authorized_bearers);
}

static uint64_t bearer_token_signature(nd_uuid_t token, struct bearer_token *bt) {
    // we use a custom structure to make sure that changes in the other code will not affect the signature

    struct {
        nd_uuid_t host_uuid;
        nd_uuid_t token;
        nd_uuid_t cloud_account_id;
        char client_name[CLOUD_CLIENT_NAME_LENGTH];
        HTTP_ACCESS access;
        HTTP_USER_ROLE user_role;
        time_t created_s;
        time_t expires_s;
    } signature_payload = {
        .access = bt->access,
        .user_role = bt->user_role,
        .created_s = bt->created_s,
        .expires_s = bt->expires_s,
    };
    uuid_copy(signature_payload.host_uuid, localhost->host_id.uuid);
    uuid_copy(signature_payload.token, token);
    uuid_copy(signature_payload.cloud_account_id, bt->cloud_account_id);
    memset(signature_payload.client_name, 0, sizeof(signature_payload.client_name));
    strncpyz(signature_payload.client_name, bt->client_name, sizeof(signature_payload.client_name) - 1);

    return XXH3_64bits(&signature_payload, sizeof(signature_payload));
}

static bool bearer_token_save_to_file(nd_uuid_t token, struct bearer_token *bt) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_uint64(wb, "version", 1);
    buffer_json_member_add_uuid(wb, "host_uuid", localhost->host_id.uuid);
    buffer_json_member_add_uuid(wb, "token", token);
    buffer_json_member_add_uuid(wb, "cloud_account_id", bt->cloud_account_id);
    buffer_json_member_add_string(wb, "client_name", bt->client_name);
    http_access2buffer_json_array(wb, "access", bt->access);
    buffer_json_member_add_string(wb, "user_role", http_id2user_role(bt->user_role));
    buffer_json_member_add_uint64(wb, "created_s", bt->created_s);
    buffer_json_member_add_uint64(wb, "expires_s", bt->expires_s);
    buffer_json_member_add_uint64(wb, "signature", bearer_token_signature(token, bt));
    buffer_json_finalize(wb);

    char filename[FILENAME_MAX];
    bearer_token_filename(filename, token);

    FILE *fp = fopen(filename, "w");
    if(!fp) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot create file '%s'", filename);
        return false;
    }

    size_t len = buffer_strlen(wb);
    size_t written = fwrite(buffer_tostring(wb), 1, len, fp);
    int saved_errno = errno;
    if(fclose(fp) != 0 || written != len) {
        if(written == len)
            saved_errno = errno;

        unlink(filename);
        errno = saved_errno;
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot save file '%s'", filename);
        return false;
    }

    return true;
}

static const DICTIONARY_ITEM *bearer_token_set_and_acquire(
    nd_uuid_t token, HTTP_USER_ROLE user_role, HTTP_ACCESS access,
    nd_uuid_t cloud_account_id, const char *client_name,
    time_t created_s, time_t expires_s, bool *inserted) {
    char uuid_str[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(token, uuid_str);

    struct bearer_token candidate = {
        .access = access,
        .user_role = user_role,
        .created_s = created_s,
        .expires_s = expires_s,
    };
    uuid_copy(candidate.cloud_account_id, cloud_account_id);
    strncpyz(candidate.client_name, client_name, sizeof(candidate.client_name) - 1);

    *inserted = false;
    return dictionary_set_and_acquire_item_advanced(
        netdata_authorized_bearers, uuid_str, -1,
        &candidate, sizeof(candidate), inserted);
}

static time_t bearer_create_token_internal(nd_uuid_t token, HTTP_USER_ROLE user_role, HTTP_ACCESS access, nd_uuid_t cloud_account_id, const char *client_name, time_t created_s, time_t expires_s, bool save) {
    bool inserted;
    const DICTIONARY_ITEM *item = bearer_token_set_and_acquire(
        token, user_role, access, cloud_account_id, client_name,
        created_s, expires_s, &inserted);
    struct bearer_token *bt = dictionary_acquired_item_value(item);

    if(inserted && save)
        bearer_token_save_to_file(token, bt);

    time_t expiration = bt->expires_s;

    dictionary_acquired_item_release(netdata_authorized_bearers, item);

    return expiration;
}

time_t bearer_create_token(nd_uuid_t *uuid, HTTP_USER_ROLE user_role, HTTP_ACCESS access, nd_uuid_t cloud_account_id, const char *client_name) {
    time_t now_s = now_realtime_sec();
    time_t expires_s = 0;

    struct bearer_token *bt;
    dfe_start_read(netdata_authorized_bearers, bt) {
        if(bt->expires_s > now_s + 3600 * 2 &&                                          // expires in more than 2 hours
            user_role == bt->user_role &&                                               // the user_role matches
            access == bt->access &&                                                     // the access matches
            uuid_eq(cloud_account_id, bt->cloud_account_id) &&                          // the cloud_account_id matches
            strncmp(client_name, bt->client_name, sizeof(bt->client_name) - 1) == 0 &&  // the client_name matches
            uuid_parse_flexi(bt_dfe.name, *uuid) == 0)                               // the token can be parsed
            return bt->expires_s; /* dfe will cleanup automatically */
    }
    dfe_done(bt);

    uuid_generate_random(*uuid);
    expires_s = bearer_create_token_internal(
        *uuid, user_role, access, cloud_account_id, client_name,
        now_s, now_s + BEARER_TOKEN_EXPIRATION, true);

    bearer_token_cleanup(false);

    return expires_s;
}

static bool bearer_token_parse_json(nd_uuid_t token, struct json_object *jobj, BUFFER *error) {
    int64_t version;
    nd_uuid_t token_in_file, cloud_account_id, host_uuid;
    CLEAN_STRING *client_name = NULL;
    HTTP_USER_ROLE user_role = HTTP_USER_ROLE_NONE;
    HTTP_ACCESS access = HTTP_ACCESS_NONE;
    time_t created_s = 0, expires_s = 0;
    uint64_t signature = 0;

    JSONC_PARSE_INT64_OR_ERROR_AND_RETURN(jobj, ".", "version", version, error, JSONC_REQUIRED);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, ".", "host_uuid", host_uuid, error, JSONC_REQUIRED);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, ".", "token", token_in_file, error, JSONC_REQUIRED);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, ".", "cloud_account_id", cloud_account_id, error, JSONC_REQUIRED);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, ".", "client_name", client_name, error, JSONC_REQUIRED);
    JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, ".", "access", http_access2id_one, access, error, JSONC_REQUIRED);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, ".", "user_role", http_user_role2id, user_role, error, JSONC_REQUIRED);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, ".", "created_s", created_s, error, JSONC_REQUIRED);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, ".", "expires_s", expires_s, error, JSONC_REQUIRED);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, ".", "signature", signature, error, JSONC_REQUIRED);

    if(uuid_compare(token, token_in_file) != 0) {
        buffer_flush(error);
        buffer_strcat(error, "token in JSON file does not match the filename");
        return false;
    }

    if(uuid_compare(host_uuid, localhost->host_id.uuid) != 0) {
        buffer_flush(error);
        buffer_strcat(error, "Host UUID in JSON file does not match our host UUID");
        return false;
    }

    if(!created_s || !expires_s || created_s >= expires_s) {
        buffer_flush(error);
        buffer_strcat(error, "bearer token has invalid dates");
        return false;
    }

    struct bearer_token bt = {
        .access = access,
        .user_role = user_role,
        .created_s = created_s,
        .expires_s = expires_s,
    };
    uuid_copy(bt.cloud_account_id, cloud_account_id);
    strncpyz(bt.client_name, string2str(client_name), sizeof(bt.client_name) - 1);

    if(signature != bearer_token_signature(token_in_file, &bt)) {
        buffer_flush(error);
        buffer_strcat(error, "bearer token has invalid signature");
        return false;
    }

    bearer_create_token_internal(token, user_role, access,
                                 cloud_account_id, string2str(client_name),
                                 created_s, expires_s, false);

    return true;
}

static bool bearer_token_load_token(nd_uuid_t token) {
    char filename[FILENAME_MAX];
    bearer_token_filename(filename, token);

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    if(!read_txt_file_to_buffer(filename, wb, 1 * 1024 * 1024))
        return false;

    CLEAN_JSON_OBJECT *jobj = json_tokener_parse(buffer_tostring(wb));
    if (jobj == NULL) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot parse bearer token file '%s'", filename);
        return false;
    }

    CLEAN_BUFFER *error = buffer_create(0, NULL);
    bool rc = bearer_token_parse_json(token, jobj, error);
    if(!rc) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to parse bearer token file '%s': %s", filename, buffer_tostring(error));
        unlink(filename);
        return false;
    }

    bearer_token_cleanup(true);

    return true;
}

static void bearer_tokens_load_from_disk(void) {
    bearer_tokens_ensure_path_exists();

    char path[FILENAME_MAX];
    bearer_tokens_path(path);

    DIR *dir = opendir(path);
    if(!dir) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot open directory '%s' to read saved bearer tokens", path);
        return;
    }

    struct dirent *de;
    while((de = readdir(dir))) {
        if (strcmp(de->d_name, ".") == 0 || strcmp(de->d_name, "..") == 0)
            continue;

        ND_UUID uuid = UUID_ZERO;
        if(uuid_parse_flexi(de->d_name, uuid.uuid) != 0 || UUIDiszero(uuid))
            continue;

        char filename[FILENAME_MAX];
        filename_from_path_entry(filename, path, de->d_name, NULL);

        if(de->d_type == DT_REG || (de->d_type == DT_LNK && filename_is_file(filename)))
            bearer_token_load_token(uuid.uuid);
    }

    closedir(dir);
}

bool web_client_bearer_token_auth(struct web_client *w, const char *v) {
    bool rc = false;

    // javascript may send "null" or "undefined"
    if(!v || !*v || strcmp(v, "null") == 0 || strcmp(v, "undefined") == 0)
        return rc;

#ifdef NETDATA_MCP_DEV_PREVIEW_API_KEY
    if (mcp_api_key_verify(v, true)) {  // silent=true for speculative check
        web_client_set_mcp_preview_key(w);
        return true;
    }
#endif

    if(!uuid_parse_flexi(v, w->auth.bearer_token)) {
        char uuid_str[UUID_COMPACT_STR_LEN];
        uuid_unparse_lower_compact(w->auth.bearer_token, uuid_str);

        const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(netdata_authorized_bearers, uuid_str);
        if(!item && bearer_token_load_token(w->auth.bearer_token))
            item = dictionary_get_and_acquire_item(netdata_authorized_bearers, uuid_str);

        if(item) {
            struct bearer_token *bt = dictionary_acquired_item_value(item);
            if (bt->expires_s > now_realtime_sec()) {
                strncpyz(w->user_auth.client_name, bt->client_name, sizeof(w->user_auth.client_name) - 1);
                uuid_copy(w->user_auth.cloud_account_id.uuid, bt->cloud_account_id);
                web_client_set_permissions(w, bt->access, bt->user_role, USER_AUTH_METHOD_BEARER);
                rc = true;
            }

            dictionary_acquired_item_release(netdata_authorized_bearers, item);
        }
    }
    else
        nd_log(NDLS_DAEMON, NDLP_NOTICE, "Invalid bearer token '%s' received.", v);

    return rc;
}

void bearer_tokens_init(void) {
    netdata_bearer_protection_set_enabled(inicfg_get_boolean(
        &netdata_config,
        CONFIG_SECTION_WEB,
        "bearer token protection",
        netdata_bearer_protection_is_enabled()));

    netdata_authorized_bearers = bearer_tokens_dictionary_create();

    bearer_tokens_load_from_disk();
}

void bearer_tokens_destroy(void) {
    dictionary_destroy(netdata_authorized_bearers);
    netdata_authorized_bearers = NULL;
}

bool extract_bearer_token_from_request(struct web_client *w, char *dst, size_t dst_len) {
    if(w->user_auth.method != USER_AUTH_METHOD_BEARER || dst_len != UUID_STR_LEN)
        return false;

    uuid_unparse_lower(w->auth.bearer_token, dst);
    return true;
}
