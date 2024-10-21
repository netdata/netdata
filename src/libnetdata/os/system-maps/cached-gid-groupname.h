// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CACHED_UID_GROUPNAME_H
#define NETDATA_CACHED_UID_GROUPNAME_H

#include "libnetdata/libnetdata.h"

struct netdata_string;

typedef struct {
    uint32_t version;
    gid_t gid;
    struct netdata_string *groupname;
} CACHED_GROUPNAME;

void cached_groupname_populate_by_gid(gid_t gid, const char *groupname, uint32_t version);
CACHED_GROUPNAME cached_groupname_get_by_gid(gid_t gid);
void cached_groupname_release(CACHED_GROUPNAME cg);
void cached_groupnames_delete_old_versions(uint32_t version);

void cached_groupnames_init(void);
void cached_groupnames_destroy(void);

#endif //NETDATA_CACHED_UID_GROUPNAME_H
