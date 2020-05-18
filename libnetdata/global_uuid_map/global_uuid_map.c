// SPDX-License-Identifier: GPL-3.0-or-later


#include "global_uuid_map.h"

static int guid_object_compare(void* a, void* b) {
    return strcmp((char *) a, (char *) b);
};

static Pvoid_t JGUID_map = (Pvoid_t) NULL;
static Pvoid_t JGUID_object_map = (Pvoid_t) NULL;

avl_tree guid_object_map = {
    NULL,
    guid_object_compare
};

#define add_object_with_guid(object) avl_insert(&guid_object_map, (avl *)(fd))

static inline GUID_OBJECT *add_object_with_guid(avl_tree_lock *tree, guid_object *value) {
    RRDVAR *ret = (RRDVAR *)avl_insert_lock(tree, (avl *)(rv));
    if(ret != rv)
        debug(D_VARIABLES, "Request to insert RRDVAR '%s' into index failed. Already exists.", rv->name);

    return ret;
}

int guid_store(uuid_t uuid, char *object)
{
    if (unlikely(!object))
        return 0;

    PWord_t *PValue = (PWord_t *) JudyHSIns(&JGUID_map, (void *)&uuid, (Word_t) sizeof(uuid_t), PJE0);
    if (PValue) {
        *PValue = (PWord_t) strdupz(object);
        return 0;
    }

//    PWord_t *PValue = (PWord_t *) JudyHSIns(&JGUID_object_map, (void *)&object, (Word_t) strlen(object), PJE0);
//    if (PValue) {
//        *PValue = (PWord_t) strdupz(object);
//        return 0;
//    }


    return 1;
}

int guid_find(uuid_t uuid, char *object, size_t max_bytes)
{
    PWord_t *PValue = (PWord_t *) JudyHSGet(JGUID_map, (void *) &uuid, (Word_t) sizeof(uuid_t));
    if (unlikely(!PValue))
        return 0;

    if (likely(object && max_bytes))
        strncpyz(object, (char *) *PValue, max_bytes);

    return 1;
}

//int find_guid_by_object(char *object, uuid_t *uuid)
//{
//    PWord_t *PValue = (PWord_t *) JudyHSGet(JGUID_object_map, (void *) &object, (Word_t) strlen(object));
//    if (unlikely(!PValue))
//        return 0;
//
//    if (likely(object && max_bytes))
//        strncpyz(object, (char *) *PValue, max_bytes);
//
//    return 1;
//}
