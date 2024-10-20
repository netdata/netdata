// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CACHED_UID_GROUPNAME_H
#define NETDATA_CACHED_UID_GROUPNAME_H

#include "libnetdata/libnetdata.h"

struct netdata_string;

typedef struct {
    bool prepopulated;
    gid_t gid;
    struct netdata_string *groupname;
} CACHED_GROUPNAME;

void cached_groupname_populate_by_uid(gid_t gid, const char *groupname, bool overwrite);
CACHED_GROUPNAME cached_groupname_get_by_gid(gid_t gid);
void cached_groupname_release(CACHED_GROUPNAME cu);

void system_groupnames_cache_init(void);
void system_groupnames_cache_destroy(void);

#endif //NETDATA_CACHED_UID_GROUPNAME_H
