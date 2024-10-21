// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CACHED_UID_USERNAME_H
#define NETDATA_CACHED_UID_USERNAME_H

#include "libnetdata/libnetdata.h"

struct netdata_string;

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

#endif //NETDATA_CACHED_UID_USERNAME_H
