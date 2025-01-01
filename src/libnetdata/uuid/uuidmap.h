// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UUIDMAP_H
#define NETDATA_UUIDMAP_H

#include "libnetdata/libnetdata.h"

typedef uint32_t uuidmap_t;

// returns ID, or zero on error
uuidmap_t uuidmap_create(const nd_uuid_t uuid);

// delete a uuid from the map
void uuidmap_free(uuidmap_t id);

// returns true if found, false if not found
// UUID is copied to out_uuid if found
bool uuidmap_uuid(uuidmap_t id, nd_uuid_t out_uuid);

nd_uuid_t *uuidmap_uuid_ptr(uuidmap_t id);

ND_UUID uuidmap_get(uuidmap_t id);

struct aral_statistics *uuidmap_aral_statistics(void);

#endif //NETDATA_UUIDMAP_H
