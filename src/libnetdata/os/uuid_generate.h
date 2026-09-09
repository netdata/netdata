// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UUID_GENERATE_H
#define NETDATA_UUID_GENERATE_H

#include "libnetdata/uuid/uuid.h"

void os_uuid_generate(nd_uuid_t out);
void os_uuid_generate_random(nd_uuid_t out);
void os_uuid_generate_time(nd_uuid_t out);

#endif //NETDATA_UUID_GENERATE_H
