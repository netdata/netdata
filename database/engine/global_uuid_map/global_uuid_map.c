// SPDX-License-Identifier: GPL-3.0-or-later

#include "global_uuid_map.h"

static Pvoid_t JGUID_map = (Pvoid_t) NULL;
static Pvoid_t JGUID_object_map = (Pvoid_t) NULL;
static uv_rwlock_t guid_lock;
static uv_rwlock_t object_lock;
static uv_rwlock_t global_lock;


void guid_bulk_load_lock()
{
    uv_rwlock_wrlock(&global_lock);

}

void guid_bulk_load_unlock()
{
    uv_rwlock_wrlock(&global_lock);
}


static inline int guid_store_internal(uuid_t uuid, char *object, int lock)
{
    if (unlikely(!object) || uuid == NULL)
        return 0;

    Pvoid_t *PValue;

    if (likely(lock))
        uv_rwlock_wrlock(&global_lock);
    PValue = JudyHSIns(&JGUID_map, (void *) uuid, (Word_t) sizeof(uuid_t), PJE0);
    if (PPJERR == PValue)
        fatal("JudyHSIns() fatal error.");
    if (*PValue)
        return 1;

    *PValue = (Pvoid_t *) strdupz(object);

    PValue = JudyHSIns(&JGUID_object_map, (void *)object, (Word_t)strlen(object), PJE0);
    if (PPJERR == PValue)
        fatal("JudyHSIns() fatal error.");
    if (*PValue == NULL) {
        uuid_t *value = (uuid_t *) mallocz(sizeof(uuid_t));
        uuid_copy(*value, uuid);
        *PValue = value;
    }
    if (likely(lock))
        uv_rwlock_wrunlock(&global_lock);

#ifdef NETDATA_INTERNAL_CHECKS
    static uint32_t count = 0;
    count++;
    char uuid_s[36 + 1];
    uuid_unparse(uuid, uuid_s);
    info("GUID Added item %"PRIu32" [%s] on [%s]", count, uuid_s, object);
#endif
    return 0;
}


inline int guid_store(uuid_t uuid, char *object)
{
    return guid_store_internal(uuid, object, 1);
}

/*
 * This can be used to bulk load entries into the global map
 *
 * A lock must be aquired since it will call guid_store_internal
 * with a "no lock" parameter.
 */
int guid_bulk_load(char *uuid, char *object)
{
    uuid_t target_uuid;
    if (likely(!uuid_parse(uuid, target_uuid))) {
#ifdef NETDATA_INTERNAL_CHECKS
        info("Mapping GUID [%s] on [%s]", uuid, object);
#endif
        return guid_store_internal(target_uuid, object, 0);
    }
    return 1;
}

/*
 * Given a GUID, find if an object is stored
 *   - Optionally return the object
 */

int guid_find(uuid_t uuid, char *object, size_t max_bytes)
{
    Pvoid_t *PValue;

    PValue = JudyHSGet(JGUID_map, (void *) uuid, (Word_t) sizeof(uuid_t));
    if (unlikely(!PValue))
        return 1;

    if (likely(object && max_bytes))
        strncpyz(object, (char *)*PValue, max_bytes - 1);

    return 0;
}

/*
 * Find a GUID of an object
 *   - Optionally return the GUID
 *
 */

int find_guid_by_object(char *object, uuid_t *uuid)
{
    Pvoid_t *PValue;

    PValue = JudyHSGet(JGUID_object_map, (void *) object, (Word_t) strlen(object));
    if (unlikely(!PValue))
        return 1;

    if (likely(uuid))
        uuid_copy(*uuid, *PValue);

    return 0;
}

int find_or_generate_guid(char *object, uuid_t *uuid)
{
    int rc = find_guid_by_object(object, uuid);
    if (rc) {
        uuid_generate(*uuid);
        return (guid_store_internal(*uuid, object, 1));
    }
#ifdef NETDATA_INTERNAL_CHECKS
    char uuid_s[36 + 1];
    uuid_unparse(*uuid, uuid_s);
    info("Found [%s] on GUID %s", object, uuid_s);
#endif
    return 0;
}

void init_global_guid_map()
{
    static int init = 0;

    if (init)
        return;

    init = 1;
    info("Configuring locking mechanism for global GUID map");
    assert(0 == uv_rwlock_init(&guid_lock));
    assert(0 == uv_rwlock_init(&object_lock));
    assert(0 == uv_rwlock_init(&global_lock));

    int rc = guid_bulk_load("6fc56a64-05d7-47a7-bc82-7f3235d8cbda","d6b37186-74db-11ea-88b2-0bf5095b1f9e/cgroup_qemu_ubuntu18.04.cpu_per_core/cpu3");
    rc = guid_bulk_load("75c6fa02-97cc-40c1-aacd-a0132190472e","d6b37186-74db-11ea-88b2-0bf5095b1f9e/services.throttle_io_ops_write/system.slice_setvtrgb.service");
    if (rc == 0)
        info("BULK GUID load successful");

    return;
}


