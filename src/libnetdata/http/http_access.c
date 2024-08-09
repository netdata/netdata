// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

static struct {
    HTTP_USER_ROLE role;
    const char *name;
} user_roles[] = {
    { .role = HTTP_USER_ROLE_NONE, .name = "none" },
    { .role = HTTP_USER_ROLE_ADMIN, .name = "admin" },
    { .role = HTTP_USER_ROLE_MANAGER, .name = "manager" },
    { .role = HTTP_USER_ROLE_TROUBLESHOOTER, .name = "troubleshooter" },
    { .role = HTTP_USER_ROLE_OBSERVER, .name = "observer" },
    { .role = HTTP_USER_ROLE_MEMBER, .name = "member" },
    { .role = HTTP_USER_ROLE_BILLING, .name = "billing" },
    { .role = HTTP_USER_ROLE_ANY, .name = "any" },

    { .role = HTTP_USER_ROLE_MEMBER, .name = "members" },
    { .role = HTTP_USER_ROLE_ADMIN, .name = "admins" },
    { .role = HTTP_USER_ROLE_ANY, .name = "all" },

    // terminator
    { .role = 0, .name = NULL },
};

HTTP_USER_ROLE http_user_role2id(const char *role) {
    if(!role || !*role)
        return HTTP_USER_ROLE_MEMBER;

    for(size_t i = 0; user_roles[i].name ;i++) {
        if(strcmp(user_roles[i].name, role) == 0)
            return user_roles[i].role;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING, "HTTP user role '%s' is not valid", role);
    return HTTP_USER_ROLE_NONE;
}

const char *http_id2user_role(HTTP_USER_ROLE role) {
    for(size_t i = 0; user_roles[i].name ;i++) {
        if(role == user_roles[i].role)
            return user_roles[i].name;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING, "HTTP user role %d is not valid", role);
    return "none";
}

// --------------------------------------------------------------------------------------------------------------------

static struct {
    const char *name;
    uint32_t hash;
    HTTP_ACCESS value;
} http_accesses[] = {
      {"none"                       , 0    , HTTP_ACCESS_NONE}
    , {"signed-in"                  , 0    , HTTP_ACCESS_SIGNED_ID}
    , {"same-space"                 , 0    , HTTP_ACCESS_SAME_SPACE}
    , {"commercial"                 , 0    , HTTP_ACCESS_COMMERCIAL_SPACE}
    , {"anonymous-data"             , 0    , HTTP_ACCESS_ANONYMOUS_DATA}
    , {"sensitive-data"             , 0    , HTTP_ACCESS_SENSITIVE_DATA}
    , {"view-config"                , 0    , HTTP_ACCESS_VIEW_AGENT_CONFIG}
    , {"edit-config"                , 0    , HTTP_ACCESS_EDIT_AGENT_CONFIG}
    , {"view-notifications-config"  , 0    , HTTP_ACCESS_VIEW_NOTIFICATIONS_CONFIG}
    , {"edit-notifications-config"  , 0    , HTTP_ACCESS_EDIT_NOTIFICATIONS_CONFIG}
    , {"view-alerts-silencing"      , 0    , HTTP_ACCESS_VIEW_ALERTS_SILENCING}
    , {"edit-alerts-silencing"     , 0    , HTTP_ACCESS_EDIT_ALERTS_SILENCING}

    , {NULL                , 0    , 0}
};

inline HTTP_ACCESS http_access2id_one(const char *str) {
    HTTP_ACCESS ret = 0;

    if(!str || !*str) return ret;

    uint32_t hash = simple_hash(str);
    int i;
    for(i = 0; http_accesses[i].name ; i++) {
        if(unlikely(!http_accesses[i].hash))
            http_accesses[i].hash = simple_hash(http_accesses[i].name);

        if (unlikely(hash == http_accesses[i].hash && !strcmp(str, http_accesses[i].name))) {
            ret |= http_accesses[i].value;
            break;
        }
    }

    return ret;
}

inline HTTP_ACCESS http_access2id(char *str) {
    HTTP_ACCESS ret = 0;
    char *tok;

    while(str && *str && (tok = strsep_skip_consecutive_separators(&str, ", |"))) {
        if(!*tok) continue;
        ret |= http_access2id_one(tok);
    }

    return ret;
}

void http_access2buffer_json_array(BUFFER *wb, const char *key, HTTP_ACCESS access) {
    buffer_json_member_add_array(wb, key);

    HTTP_ACCESS used = 0; // to prevent adding duplicates
    for(int i = 0; http_accesses[i].name ; i++) {
        if (unlikely((http_accesses[i].value & access) && !(http_accesses[i].value & used))) {
            const char *name = http_accesses[i].name;
            used |= http_accesses[i].value;

            buffer_json_add_array_item_string(wb, name);
        }
    }

    buffer_json_array_close(wb);
}

void http_access2txt(char *buf, size_t size, const char *separator, HTTP_ACCESS access) {
    char *write = buf;
    char *end = &buf[size - 1];

    HTTP_ACCESS used = 0; // to prevent adding duplicates
    int added = 0;
    for(int i = 0; http_accesses[i].name ; i++) {
        if (unlikely((http_accesses[i].value & access) && !(http_accesses[i].value & used))) {
            const char *name = http_accesses[i].name;
            used |= http_accesses[i].value;

            if(added && write < end) {
                const char *s = separator;
                while(*s && write < end)
                    *write++ = *s++;
            }

            while(*name && write < end)
                *write++ = *name++;

            added++;
        }
    }
    *write = *end = '\0';
}

HTTP_ACCESS http_access_from_hex_mapping_old_roles(const char *str) {
    if(!str || !*str)
        return HTTP_ACCESS_NONE;

    if(strcmp(str, "any") == 0 || strcmp(str, "all") == 0)
        return HTTP_ACCESS_MAP_OLD_ANY;

    if(strcmp(str, "member") == 0 || strcmp(str, "members") == 0)
        return HTTP_ACCESS_MAP_OLD_MEMBER;

    else if(strcmp(str, "admin") == 0 || strcmp(str, "admins") == 0)
        return HTTP_ACCESS_MAP_OLD_ADMIN;

    return (HTTP_ACCESS)strtoull(str, NULL, 16) & HTTP_ACCESS_ALL;
}

HTTP_ACCESS http_access_from_hex(const char *str) {
    if(!str || !*str)
        return HTTP_ACCESS_NONE;

    return (HTTP_ACCESS)strtoull(str, NULL, 16) & HTTP_ACCESS_ALL;
}

HTTP_ACCESS http_access_from_source(const char *str) {
    if(!str || !*str)
        return HTTP_ACCESS_NONE;

    HTTP_ACCESS access = HTTP_ACCESS_NONE;

    const char *permissions = strstr(str, "permissions=");
    if(permissions)
        access = (HTTP_ACCESS)strtoull(permissions + 12, NULL, 16) & HTTP_ACCESS_ALL;

    return access;
}

bool log_cb_http_access_to_hex(BUFFER *wb, void *data) {
    HTTP_ACCESS access = *((HTTP_ACCESS *)data);
    buffer_sprintf(wb, HTTP_ACCESS_FORMAT, (HTTP_ACCESS_FORMAT_CAST)access);
    return true;
}
