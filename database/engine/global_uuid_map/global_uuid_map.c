// SPDX-License-Identifier: GPL-3.0-or-later

#include "global_uuid_map.h"

static Pvoid_t JGUID_map = (Pvoid_t) NULL;
static Pvoid_t JGUID_object_map = (Pvoid_t) NULL;
static uv_rwlock_t guid_lock;
static uv_rwlock_t object_lock;
static uv_rwlock_t global_lock;


int guid_store(uuid_t uuid, char *object)
{
#ifdef NETDATA_INTERNAL_CHECKS
    static uint32_t count = 0;
#endif
    if (unlikely(!object))
        return 0;

    Pvoid_t *PValue;

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
        uuid_t *value = mallocz(sizeof(uuid_t));
        memcpy(value, &uuid, sizeof(uuid_t));
        *PValue = value;
    }
    uv_rwlock_wrunlock(&global_lock);
#ifdef NETDATA_INTERNAL_CHECKS
    count++;
    char uuid_s[36 + 1];
    uuid_unparse(uuid, uuid_s);
    info("GUID Added item %"PRIu32" [%s] on [%s]", count, uuid_s, object);
#endif
    return 0;
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
        memcpy(uuid, *PValue, sizeof(*uuid));

    return 0;
}

int find_or_generate_guid(char *object, uuid_t *uuid)
{
    int rc = find_guid_by_object(object, uuid);
    if (rc) {
        uuid_generate(*uuid);
        if (guid_store(*uuid, object))
            return 1;
    }
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
    return;
}

//void assign_guid_to_metric(RRDDIM *rd)
//{
//    struct page_cache *pg_cache;
//    struct rrdengine_instance *ctx;
//    uuid_t temp_id;
//    Pvoid_t *PValue;
//    EVP_MD_CTX *evpctx;
//    unsigned char hash_value[EVP_MAX_MD_SIZE];
//    unsigned int hash_len;
//
//    if (unlikely(!rd->state->metric_uuid)) {
//        char uuid_s[36 + 1];
//        uuid_unparse(rd->metric_uuid, uuid_s);
//        info("Netdata knows about the metric [%s] [%s] with GUID %s", rd->id,rd->rrdset->id, uuid_s);
//        return;
//    }
//
//    // Calculate fake GUID and see if dbengine knows about it
//    ctx = rd->rrdset->rrdhost->rrdeng_ctx;
//    pg_cache = &ctx->pg_cache;
//    evpctx = EVP_MD_CTX_create();
//    EVP_DigestInit_ex(evpctx, EVP_sha256(), NULL);
//    EVP_DigestUpdate(evpctx, rd->id, strlen(rd->id));
//    EVP_DigestUpdate(evpctx, rd->rrdset->id, strlen(rd->rrdset->id));
//    EVP_DigestFinal_ex(evpctx, hash_value, &hash_len);
//    EVP_MD_CTX_destroy(evpctx);
//    assert(hash_len > sizeof(temp_id));
//    memcpy(&temp_id, hash_value, sizeof(temp_id));
//
//    char uuid_s[36 + 1];
//    uuid_unparse(temp_id, uuid_s);
////    info("Generated FAKE [%s] for [%s/%s]", uuid_s, rd->id,rd->rrdset->id);
//    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
//    PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, &temp_id, sizeof(uuid_t));
//    if (likely(PValue)) {
//        info("DBENGINE knows about the metric [%s] [%s] with GUID %s -- reusing it",  rd->id,rd->rrdset->id, uuid_s);
//        rd->metric_uuid = malloc(sizeof(*rd->metric_uuid));
//        memcpy(rd->metric_uuid, &temp_id, sizeof(*rd->metric_uuid));
//    }
//    else
//        info("GUID [%s] not found in map for [%s] [%s]", uuid_s, rd->id,rd->rrdset->id);
//    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
//    //return (NULL != PValue);
//}
