// SPDX-License-Identifier: GPL-3.0-or-later

#include "user-auth.h"

// Define the mapping between enum values and strings
ENUM_STR_MAP_DEFINE(USER_AUTH_METHOD) = {
    {USER_AUTH_METHOD_NONE,   "none"},
    {USER_AUTH_METHOD_CLOUD,  "NC"},
    {USER_AUTH_METHOD_BEARER, "api-bearer"},
    {USER_AUTH_METHOD_GOD,    "god"},

    // terminator
    {0, NULL},
};

// Define the functions to convert between enum values and strings
ENUM_STR_DEFINE_FUNCTIONS(USER_AUTH_METHOD, USER_AUTH_METHOD_NONE, "none")

bool user_auth_source_is_cloud(const char *source) {
    return source && *source && strstartswith(source, "method=NC,");
}

void user_auth_to_source_buffer(USER_AUTH *user_auth, BUFFER *source) {
    buffer_reset(source);

    buffer_sprintf(source, "method=%s", USER_AUTH_METHOD_2str(user_auth->method));
    buffer_sprintf(source, ",role=%s", user_auth->method == USER_AUTH_METHOD_GOD ? "god" : http_id2user_role(user_auth->user_role));
    buffer_sprintf(source, ",permissions="HTTP_ACCESS_FORMAT, (HTTP_ACCESS_FORMAT_CAST)user_auth->access);

    if(user_auth->client_name[0])
        buffer_sprintf(source, ",user=%s", user_auth->client_name);

    if(!uuid_is_null(user_auth->cloud_account_id.uuid)) {
        char uuid_str[UUID_COMPACT_STR_LEN];
        uuid_unparse_lower_compact(user_auth->cloud_account_id.uuid, uuid_str);
        buffer_sprintf(source, ",account=%s", uuid_str);
    }

    if(user_auth->client_ip[0])
        buffer_sprintf(source, ",ip=%s", user_auth->client_ip);

    if(user_auth->forwarded_for[0])
        buffer_sprintf(source, ",forwarded_for=%s", user_auth->forwarded_for);
}

bool user_auth_from_source(const char *src, USER_AUTH *parsed) {
    char *buf, *token, *saveptr;

    if (!src || !parsed)
        return false;

    // Initialize the structure.
    memset(parsed, 0, sizeof(*parsed));

    // Duplicate the source string because strtok_r modifies it.
    buf = strdupz(src);
    token = strtok_r(buf, ",", &saveptr);
    while (token) {
        char *equal_sign = strchr(token, '=');
        if (equal_sign) {
            *equal_sign = '\0';
            const char *key = token;
            const char *value = equal_sign + 1;

            if (strcmp(key, "method") == 0) {
                parsed->method = USER_AUTH_METHOD_2id(value);
            }
            else if (strcmp(key, "role") == 0) {
                if (strcmp(value, "god") == 0)
                    parsed->method = USER_AUTH_METHOD_GOD;
                else
                    parsed->user_role = http_user_role2id(value);
            }
            else if (strcmp(key, "permissions") == 0)
                parsed->access = http_access_from_hex_str(value);

            else if (strcmp(key, "user") == 0) {
                strncpyz(parsed->client_name, value, CLOUD_CLIENT_NAME_LENGTH - 1);
            }
            else if (strcmp(key, "account") == 0) {
                if (uuid_parse(value, parsed->cloud_account_id.uuid) != 0)
                    parsed->cloud_account_id = UUID_ZERO;
            }
            else if (strcmp(key, "ip") == 0) {
                strncpyz(parsed->client_ip, value, INET6_ADDRSTRLEN - 1);
            }
            else if (strcmp(key, "forwarded_for") == 0) {
                strncpyz(parsed->forwarded_for, value, INET6_ADDRSTRLEN - 1);
            }
        }
        token = strtok_r(NULL, ",", &saveptr);
    }

    freez(buf);
    return true;
}
