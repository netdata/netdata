// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MACHINE_GUID_H
#define NETDATA_MACHINE_GUID_H

#include "libnetdata/libnetdata.h"

typedef struct nd_machine_guid {
    char txt[UUID_STR_LEN];
    ND_UUID uuid;
    usec_t last_modified_ut;
} ND_MACHINE_GUID;

ND_MACHINE_GUID *machine_guid_get(void);
const char *machine_guid_get_txt(void);

#endif //NETDATA_MACHINE_GUID_H
