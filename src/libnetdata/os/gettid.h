// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_GETTID_H
#define NETDATA_GETTID_H

#include <unistd.h>

pid_t os_gettid(void);
pid_t gettid_cached(void);
pid_t gettid_uncached(void);

#endif //NETDATA_GETTID_H
