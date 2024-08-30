// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SETRESGID_H
#define NETDATA_SETRESGID_H

#include <unistd.h>
int os_setresgid(gid_t gid, gid_t egid, gid_t sgid);

#endif //NETDATA_SETRESGID_H
