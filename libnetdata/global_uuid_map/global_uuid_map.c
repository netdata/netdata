// SPDX-License-Identifier: GPL-3.0-or-later

#include "global_uuid_map.h"

static int guid_object_compare(void* a, void* b) {
    return strcmp((char *) a, (char *) b);
};

Pvoid_t JGUID_map = (Pvoid_t) NULL;
Pvoid_t JGUID_object_map = (Pvoid_t) NULL;

int guid_store(uuid_t uuid, char *object)
{
#ifdef NETDATA_INTERNAL_CHECKS
    static uint32_t count = 0;
#endif
    if (unlikely(!object))
        return 0;

    Pvoid_t *PValue;

    PValue = JudyHSIns(&JGUID_map, (void *) uuid, (Word_t)sizeof(uuid_t), PJE0);
    if (*PValue) {
        //error("Failed to add GUID %s to %s --- contains [%s]", uuid_s, object, *PValue);
        return 1;
    }

    *PValue = (Pvoid_t *) strdupz(object);

    PValue = JudyHSIns(&JGUID_object_map, (void *) object, (Word_t)strlen(object), PJE0);
    if (*PValue) {
        int rc = JudyHSDel(&JGUID_map, (void *)&uuid, (Word_t)sizeof(uuid_t), PJE0);
        errno = 0;
        error("Error storing map of object %s to GUID; cleanup rc=%d", object, rc);
        return 1;
    }

    uuid_t *value = mallocz(sizeof(uuid_t));
    memcpy(value, &uuid, sizeof(uuid_t));
    *PValue = value;
    count++;
#ifdef NETDATA_INTERNAL_CHECKS
    count++;
    char uuid_s[37];
    uuid_unparse(uuid, uuid_s);
    info("GUID Added item %ld [%s] on [%s]", count, uuid_s, object);
#endif
    return 0;
}

int guid_find(uuid_t uuid, char *object, size_t max_bytes)
{
    Pvoid_t *PValue;

    PValue = JudyHSGet(JGUID_map, (void *) uuid, (Word_t) sizeof(uuid_t));
    if (unlikely(!PValue))
        return 0;

    if (likely(object && max_bytes))
        strncpyz(object, (char *) *PValue, max_bytes);

    return 1;
}

int find_guid_by_object(char *object, uuid_t *uuid)
{
    Pvoid_t *PValue;

    PValue = JudyHSGet(JGUID_object_map, (void *) object, (Word_t) strlen(object));
    if (unlikely(!PValue)) {
        return 1;
    }

    if (likely(uuid)) {
        memcpy(uuid, *PValue, sizeof(*uuid));
    }
    return 0;
}
