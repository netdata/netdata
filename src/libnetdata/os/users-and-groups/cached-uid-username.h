// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CACHED_UID_USERNAME_H
#define NETDATA_CACHED_UID_USERNAME_H

#include "libnetdata/libnetdata.h"

struct netdata_string;

typedef struct {
    bool prepopulated;
    uid_t uid;
    struct netdata_string *username;
} CACHED_USERNAME;

void cached_username_populate_by_uid(uid_t uid, const char *username, bool overwrite);
CACHED_USERNAME cached_username_get_by_uid(uid_t uid);
void cached_username_release(CACHED_USERNAME cu);

void system_usernames_cache_init(void);
void system_usernames_cache_destroy(void);

#endif //NETDATA_CACHED_UID_USERNAME_H
