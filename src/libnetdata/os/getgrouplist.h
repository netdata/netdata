// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_GETGROUPLIST_H
#define NETDATA_GETGROUPLIST_H

#include <unistd.h>
int os_getgrouplist(const char *username, gid_t gid, gid_t *supplementary_groups, int *ngroups);

#endif //NETDATA_GETGROUPLIST_H
