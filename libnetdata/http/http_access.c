// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

static struct {
    HTTP_ACCESS access;
    const char *name;
} rrd_function_access_levels[] = {
        { .access = HTTP_ACCESS_NONE, .name = "none" },
        { .access = HTTP_ACCESS_MEMBERS, .name = "members" },
        { .access = HTTP_ACCESS_ADMINS, .name = "admins" },
        { .access = HTTP_ACCESS_ANY, .name = "any" },
};

HTTP_ACCESS http_access2id(const char *access) {
    if(!access || !*access)
        return HTTP_ACCESS_MEMBERS;

    size_t entries = sizeof(rrd_function_access_levels) / sizeof(rrd_function_access_levels[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(rrd_function_access_levels[i].name, access) == 0)
            return rrd_function_access_levels[i].access;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING, "HTTP access level '%s' is not valid", access);
    return HTTP_ACCESS_MEMBERS;
}

const char *http_id2access(HTTP_ACCESS access) {
    size_t entries = sizeof(rrd_function_access_levels) / sizeof(rrd_function_access_levels[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(access == rrd_function_access_levels[i].access)
            return rrd_function_access_levels[i].name;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING, "HTTP access level %d is not valid", access);
    return "members";
}
