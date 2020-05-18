// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_GLOBAL_UUID_MAP_H
#define NETDATA_GLOBAL_UUID_MAP_H

#include "../libnetdata.h"
#include <Judy.h>

extern int guid_store(uuid_t uuid, char *object);
extern int guid_find(uuid_t uuid, char *object, size_t max_bytes);
extern int find_guid_by_object(char *object, uuid_t *uuid);

#endif //NETDATA_GLOBAL_UUID_MAP_H
