// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_GLOBAL_UUID_MAP_H
#define NETDATA_GLOBAL_UUID_MAP_H

#include "libnetdata/libnetdata.h"
#include <Judy.h>
#include "../../rrd.h"

typedef enum guid_type {
    GUID_TYPE_CHAR,
    GUID_TYPE_HOST,
    GUID_TYPE_CHART,
    GUID_TYPE_DIMENSION,
    GUID_TYPE_NOTFOUND,
    GUID_TYPE_NOSPACE
} GUID_TYPE;

extern GUID_TYPE find_object_by_guid(uuid_t *uuid, char *object, size_t max_bytes);
extern int find_guid_by_object(char *object, uuid_t *uuid, GUID_TYPE);
extern void init_global_guid_map();
extern int find_or_generate_guid(void *object, uuid_t *uuid, GUID_TYPE object_type, int replace_instead_of_generate);
extern void free_uuid(uuid_t *uuid);
extern void free_global_guid_map();
#endif //NETDATA_GLOBAL_UUID_MAP_H
