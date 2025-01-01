// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UUIDMAP_H
#define NETDATA_UUIDMAP_H

#include "libnetdata/libnetdata.h"

typedef uint32_t UUIDMAP_ID;

// returns ID, or zero on error
UUIDMAP_ID uuidmap_create(const nd_uuid_t uuid);

// delete a uuid from the map
void uuidmap_free(UUIDMAP_ID id);

// returns true if found, false if not found
// UUID is copied to out_uuid if found
bool uuidmap_uuid(UUIDMAP_ID id, nd_uuid_t out_uuid);

nd_uuid_t *uuidmap_uuid_ptr(UUIDMAP_ID id);

ND_UUID uuidmap_get(UUIDMAP_ID id);

struct aral_statistics *uuidmap_aral_statistics(void);

#endif //NETDATA_UUIDMAP_H
