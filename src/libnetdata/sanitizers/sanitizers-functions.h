// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SANITIZERS_FUNCTIONS_H
#define NETDATA_SANITIZERS_FUNCTIONS_H

#include "../libnetdata.h"

size_t nrpc_sanitize_name(char *dst, const char *src, size_t dst_len);

#endif //NETDATA_SANITIZERS_FUNCTIONS_H
