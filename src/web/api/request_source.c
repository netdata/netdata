// SPDX-License-Identifier: GPL-3.0-or-later

#include "request_source.h"

bool request_source_is_cloud(const char *source) {
    return source && *source && strstartswith(source, "method=NC,");
}

void web_client_api_request_vX_source_to_buffer(struct web_client *w, BUFFER *source) {
    if(web_client_flag_check(w, WEB_CLIENT_FLAG_AUTH_CLOUD))
        buffer_sprintf(source, "method=NC");
    else if(web_client_flag_check(w, WEB_CLIENT_FLAG_AUTH_BEARER))
        buffer_sprintf(source, "method=api-bearer");
    else
        buffer_sprintf(source, "method=api");

    if(web_client_flag_check(w, WEB_CLIENT_FLAG_AUTH_GOD))
        buffer_strcat(source, ",role=god");
    else
        buffer_sprintf(source, ",role=%s", http_id2user_role(w->user_role));

    buffer_sprintf(source, ",permissions="HTTP_ACCESS_FORMAT, (HTTP_ACCESS_FORMAT_CAST)w->access);

    if(w->auth.client_name[0])
        buffer_sprintf(source, ",user=%s", w->auth.client_name);

    if(!uuid_is_null(w->auth.cloud_account_id)) {
        char uuid_str[UUID_COMPACT_STR_LEN];
        uuid_unparse_lower_compact(w->auth.cloud_account_id, uuid_str);
        buffer_sprintf(source, ",account=%s", uuid_str);
    }

    if(w->client_ip[0])
        buffer_sprintf(source, ",ip=%s", w->client_ip);

    if(w->forwarded_for)
        buffer_sprintf(source, ",forwarded_for=%s", w->forwarded_for);
}

bool parse_request_source(const char *src, PARSED_REQUEST_SOURCE *parsed) {
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
                if (strcmp(value, "NC") == 0)
                    parsed->auth |= WEB_CLIENT_FLAG_AUTH_CLOUD;
                else if (strcmp(value, "api-bearer") == 0)
                    parsed->auth |= WEB_CLIENT_FLAG_AUTH_BEARER;
                // "api" does not add an extra flag.
            }
            else if (strcmp(key, "role") == 0) {
                if (strcmp(value, "god") == 0)
                    parsed->auth |= WEB_CLIENT_FLAG_AUTH_GOD;
                else
                    parsed->user_role = http_user_role2id(value);
            }
            else if (strcmp(key, "permissions") == 0)
                parsed->access = http_access_from_hex_str(value);

            else if (strcmp(key, "user") == 0) {
                strncpy(parsed->client_name, value, CLOUD_CLIENT_NAME_LENGTH - 1);
                parsed->client_name[CLOUD_CLIENT_NAME_LENGTH - 1] = '\0';
            }
            else if (strcmp(key, "account") == 0) {
                if (uuid_parse(value, parsed->cloud_account_id.uuid) != 0)
                    parsed->cloud_account_id = UUID_ZERO;
            }
            else if (strcmp(key, "ip") == 0) {
                strncpy(parsed->client_ip, value, INET6_ADDRSTRLEN - 1);
                parsed->client_ip[INET6_ADDRSTRLEN - 1] = '\0';
            }
            else if (strcmp(key, "forwarded_for") == 0) {
                strncpy(parsed->forwarded_for, value, INET6_ADDRSTRLEN - 1);
                parsed->forwarded_for[INET6_ADDRSTRLEN - 1] = '\0';
            }
        }
        token = strtok_r(NULL, ",", &saveptr);
    }

    freez(buf);
    return true;
}
