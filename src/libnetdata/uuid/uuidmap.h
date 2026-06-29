// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UUIDMAP_H
#define NETDATA_UUIDMAP_H

#include "libnetdata/libnetdata.h"

typedef uint32_t UUIDMAP_ID;

// 2^UUIDMAP_PARTITION_BITS partitions; the remaining bits of UUIDMAP_ID are
// the per-partition ID sequence space. IDs are never reused, so raising the
// partition count proportionally lowers the per-partition lifetime ID
// capacity - total lifetime capacity stays at 2^32 and random UUIDs
// distribute evenly across partitions. The partition is also reused by MRG
// to index its own partitions, so this knob spreads the lock words of both.
#define UUIDMAP_PARTITION_BITS 5
#define UUIDMAP_PARTITIONS (1u << UUIDMAP_PARTITION_BITS)
#define UUIDMAP_ID_SEQ_BITS (32 - UUIDMAP_PARTITION_BITS)
#define UUIDMAP_ID_SEQ_MASK ((1u << UUIDMAP_ID_SEQ_BITS) - 1)

#if UUIDMAP_PARTITION_BITS < 1 || UUIDMAP_PARTITION_BITS > 8
#error "UUIDMAP_PARTITION_BITS must be 1..8: the partition is derived from one uuid byte, is stored in uint8_t, and 0 bits would make the ID packing shifts undefined (shift by 32)"
#endif

static inline uint8_t uuid_to_uuidmap_partition(const nd_uuid_t uuid) {
    return uuid[15] & (UUIDMAP_PARTITIONS - 1);
}

static inline uint8_t uuidmap_id_to_partition(UUIDMAP_ID id) {
    return (uint8_t)(id >> UUIDMAP_ID_SEQ_BITS);
}

static inline UUIDMAP_ID uuidmap_make_id(uint8_t partition, uint32_t id) {
    return ((UUIDMAP_ID)partition << UUIDMAP_ID_SEQ_BITS) | (id & UUIDMAP_ID_SEQ_MASK);
}

// returns ID, or zero on error
UUIDMAP_ID uuidmap_create(const nd_uuid_t uuid);

// Like uuidmap_create() but never inserts: returns the existing ID (refcount
// incremented) or 0 if the UUID is not in the map. Caller must call
// uuidmap_free() on the returned ID.
UUIDMAP_ID uuidmap_acquire(const nd_uuid_t uuid);

// returns the number of entries still referenced (although freed)
size_t uuidmap_destroy(void);

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
