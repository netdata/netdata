// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_GET_SYSTEM_PAGESIZE_H
#define NETDATA_GET_SYSTEM_PAGESIZE_H

#include "../common.h"

size_t os_get_system_page_size(void) __attribute__((const));

#endif //NETDATA_GET_SYSTEM_PAGESIZE_H
