// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HOSTNAME_H
#define NETDATA_HOSTNAME_H

#include "libnetdata/libnetdata.h"

bool os_hostname(char *dst, size_t dst_size, const char *filesystem_root);

#endif //NETDATA_HOSTNAME_H
