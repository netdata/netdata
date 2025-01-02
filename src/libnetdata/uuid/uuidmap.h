// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UUIDMAP_H
#define NETDATA_UUIDMAP_H

#include "libnetdata/libnetdata.h"

typedef uint32_t UUIDMAP_ID;

#define UUIDMAP_PARTITIONS 256

static inline uint8_t uuid_to_uuidmap_partition(const nd_uuid_t uuid) {
    return uuid[15];
}

static inline uint8_t uuidmap_id_to_partition(UUIDMAP_ID id) {
    return (uint8_t)(id >> 24);
}

// returns ID, or zero on error
UUIDMAP_ID uuidmap_create(const nd_uuid_t uuid);

// delete a uuid from the map
void uuidmap_free(UUIDMAP_ID id);

// returns true if found, false if not found
// UUID is copied to out_uuid if found
bool uuidmap_uuid(UUIDMAP_ID id, nd_uuid_t out_uuid);

nd_uuid_t *uuidmap_uuid_ptr(UUIDMAP_ID id);
nd_uuid_t *uuidmap_uuid_ptr_and_dup(UUIDMAP_ID id);

ND_UUID uuidmap_get(UUIDMAP_ID id);

size_t uuidmap_memory(void);
size_t uuidmap_free_bytes(void);
struct aral_statistics *uuidmap_aral_statistics(void);

UUIDMAP_ID uuidmap_dup(UUIDMAP_ID id);

int uuidmap_unittest(void);

#endif //NETDATA_UUIDMAP_H
