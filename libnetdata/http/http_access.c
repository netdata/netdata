// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

static struct {
    HTTP_ACCESS access;
    const char *name;
} access_levels[] = {
    { .access = HTTP_ACCESS_NONE, .name = "none" },
    { .access = HTTP_ACCESS_MEMBER, .name = "member" },
    { .access = HTTP_ACCESS_ADMIN, .name = "admin" },
    { .access = HTTP_ACCESS_ANY, .name = "any" },

    { .access = HTTP_ACCESS_MEMBER, .name = "members" },
    { .access = HTTP_ACCESS_ADMIN, .name = "admins" },
    { .access = HTTP_ACCESS_ANY, .name = "all" },

    // terminator
    { .access = 0, .name = NULL },
};

HTTP_ACCESS http_access2id(const char *access) {
    if(!access || !*access)
        return HTTP_ACCESS_MEMBER;

    for(size_t i = 0; access_levels[i].name ;i++) {
        if(strcmp(access_levels[i].name, access) == 0)
            return access_levels[i].access;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING, "HTTP access level '%s' is not valid", access);
    return HTTP_ACCESS_NONE;
}

const char *http_id2access(HTTP_ACCESS access) {
    for(size_t i = 0; access_levels[i].name ;i++) {
        if(access == access_levels[i].access)
            return access_levels[i].name;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING, "HTTP access level %d is not valid", access);
    return "none";
}
