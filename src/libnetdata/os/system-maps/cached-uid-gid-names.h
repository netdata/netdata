// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CACHED_UID_GID_NAMES_H
#define NETDATA_CACHED_UID_GID_NAMES_H

#include "libnetdata/libnetdata.h"

struct netdata_string;

// uid -> username

typedef struct {
    uint32_t version;
    uid_t uid;
    struct netdata_string *username;
} CACHED_USERNAME;

void cached_username_populate_by_uid(uid_t uid, const char *username, uint32_t version);
CACHED_USERNAME cached_username_get_by_uid(uid_t uid);
void cached_username_release(CACHED_USERNAME cu);

void cached_usernames_init(void);
void cached_usernames_destroy(void);
void cached_usernames_delete_old_versions(uint32_t version);

// gid -> groupname

typedef struct {
    uint32_t version;
    gid_t gid;
    struct netdata_string *groupname;
} CACHED_GROUPNAME;

void cached_groupname_populate_by_gid(gid_t gid, const char *groupname, uint32_t version);
CACHED_GROUPNAME cached_groupname_get_by_gid(gid_t gid);
void cached_groupname_release(CACHED_GROUPNAME cg);

void cached_groupnames_init(void);
void cached_groupnames_destroy(void);
void cached_groupnames_delete_old_versions(uint32_t version);

// refresh the caches from {host prefix}/etc/passwd and {host prefix}/etc/group

void update_cached_host_users(void);
void update_cached_host_groups(void);

#endif //NETDATA_CACHED_UID_GID_NAMES_H
