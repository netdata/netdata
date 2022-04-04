// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_CAPABILITY_LIST_H
#define ACLK_SCHEMA_CAPABILITY_LIST_H

#include <stdlib.h>

#include "database/rrd.h"

#ifdef __cplusplus
extern "C" {
#endif

struct capability {
    const char *name;
    uint32_t version;
    int enabled;
};

typedef void capabilities_list;

capabilities_list *new_capabilities_list();



#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_CAPABILITY_LIST_H */
