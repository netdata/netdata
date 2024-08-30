// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SETRESUID_H
#define NETDATA_SETRESUID_H

#include <unistd.h>

int os_setresuid(uid_t uid, uid_t euid, uid_t suid);

#endif //NETDATA_SETRESUID_H
