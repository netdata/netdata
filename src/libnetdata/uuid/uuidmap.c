// SPDX-License-Identifier: GPL-3.0-or-later

#include "uuidmap.h"

struct uuidmap_entry {
    nd_uuid_t uuid;
    REFCOUNT refcount;
};

static struct {
    Pvoid_t uuid_to_id;     // JudyL: UUID string -> ID
    Pvoid_t id_to_uuid;     // JudyL: ID -> UUID binary
    uuidmap_t next_id;
    RW_SPINLOCK spinlock;

    int64_t memory;
    int32_t entries;

    ARAL *ar;
} uuid_map = {
    .uuid_to_id = NULL,
    .id_to_uuid = NULL,
    .next_id = 0,
    .spinlock = RW_SPINLOCK_INITIALIZER,
};

static struct aral_statistics uuidmap_stats = { 0 };
struct aral_statistics *uuidmap_aral_statistics(void) { return &uuidmap_stats; }

uuidmap_t uuidmap_create(const nd_uuid_t uuid) {
    JudyAllocThreadPulseReset();

    rw_spinlock_write_lock(&uuid_map.spinlock);

    if(!uuid_map.ar) {
        uuid_map.ar = aral_create(
            "uuidmap",
            sizeof(struct uuidmap_entry),
            0,
            0,
            &uuidmap_stats,
            NULL, NULL, false, true, true);
    }

    uuidmap_t id = 0;

    // Try to insert or get existing UUID
    Pvoid_t *PValue = JudyHSIns(&uuid_map.uuid_to_id, (void *)uuid, sizeof(nd_uuid_t), PJE0);
    if(!PValue || PValue == PJERR)
        fatal("UUIDMAP: corrupted JudyHS array");

    // If value exists, return it
    if (*PValue != 0) {
        id = (uuidmap_t)(uintptr_t)*PValue;

        PValue = JudyLGet(uuid_map.id_to_uuid, id, PJE0);
        if (!PValue || PValue == PJERR)
            fatal("UUIDMAP: corrupted JudyL array");

        struct uuidmap_entry *ue = *PValue;
        ue->refcount++;

        goto done;
    }

    id = ++uuid_map.next_id;
    *(uuidmap_t *)PValue = id;

    // Store ID -> UUID mapping
    PValue = JudyLIns(&uuid_map.id_to_uuid, id, PJE0);
    if (!PValue || PValue == PJERR)
        fatal("UUIDMAP: corrupted JudyL array");

    struct uuidmap_entry *ue = aral_mallocz(uuid_map.ar);
    nd_uuid_copy(ue->uuid, uuid);
    ue->refcount = 1;
    *PValue = ue;

    uuid_map.entries++;
    uuid_map.memory += sizeof(*ue);

done:
    uuid_map.memory += JudyAllocThreadPulseGetAndReset();
    rw_spinlock_write_unlock(&uuid_map.spinlock);
    return id;
}

void uuidmap_free(uuidmap_t id) {
    JudyAllocThreadPulseReset();

    rw_spinlock_write_lock(&uuid_map.spinlock);

    Pvoid_t *PValue = JudyLGet(uuid_map.id_to_uuid, id, PJE0);
    if (PValue == PJERR)
        fatal("UUIDMAP: corrupted JudyL array");

    if (!PValue) {
        rw_spinlock_write_unlock(&uuid_map.spinlock);
        return;
    }

    struct uuidmap_entry *ue = *PValue;
    ue->refcount--;
    if(!ue->refcount) {

        int rc;
        rc = JudyHSDel(&uuid_map.uuid_to_id, (void *)ue->uuid, sizeof(nd_uuid_t), PJE0);
        if(unlikely(!rc))
            fatal("UUIDMAP: cannot delete UUID from JudyHS");

        rc = JudyLDel(&uuid_map.id_to_uuid, id, PJE0);
        if(unlikely(!rc))
            fatal("UUIDMAP: cannot delete ID from JudyL");

        uuid_map.memory -= sizeof(*ue);
        uuid_map.entries--;

        aral_freez(uuid_map.ar, ue);
    }

    uuid_map.memory += JudyAllocThreadPulseGetAndReset();
    rw_spinlock_write_unlock(&uuid_map.spinlock);
}

nd_uuid_t *uuidmap_uuid_ptr(uuidmap_t id) {
    if (id == 0) return NULL;

    rw_spinlock_read_lock(&uuid_map.spinlock);

    Pvoid_t *PValue = JudyLGet(uuid_map.id_to_uuid, id, PJE0);
    if (PValue == PJERR)
        fatal("UUIDMAP: corrupted JudyL array");

    if(!PValue) {
        rw_spinlock_read_unlock(&uuid_map.spinlock);
        return NULL;
    }

    struct uuidmap_entry *ue = *PValue;

    rw_spinlock_read_unlock(&uuid_map.spinlock);
    return &ue->uuid;
}

bool uuidmap_uuid(uuidmap_t id, nd_uuid_t out_uuid) {
    nd_uuid_t *uuid = uuidmap_uuid_ptr(id);

    if(!uuid) {
        nd_uuid_clear(out_uuid);
        return false;
    }

    return true;
}

ND_UUID uuidmap_get(uuidmap_t id) {
    ND_UUID uuid;
    uuidmap_uuid(id, uuid.uuid);
    return uuid;
}
