// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SANITIZERS_FUNCTIONS_H
#define NETDATA_SANITIZERS_FUNCTIONS_H

#include "../libnetdata.h"

size_t rrd_functions_sanitize(char *dst, const char *src, size_t dst_len);

#endif //NETDATA_SANITIZERS_FUNCTIONS_H
