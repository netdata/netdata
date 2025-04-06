// SPDX-License-Identifier: GPL-3.0-or-later

#include "claim_id.h"

static struct {
    SPINLOCK spinlock;
    ND_UUID claim_uuid;
    ND_UUID claim_uuid_saved;
} claim = {
    .spinlock = SPINLOCK_INITIALIZER,
};

void claim_id_clear_previous_working(void) {
    spinlock_lock(&claim.spinlock);
    claim.claim_uuid_saved = UUID_ZERO;
    spinlock_unlock(&claim.spinlock);
}

void claim_id_set(ND_UUID new_claim_id) {
    spinlock_lock(&claim.spinlock);

    if(!UUIDiszero(claim.claim_uuid)) {
        if(aclk_online())
            claim.claim_uuid_saved = claim.claim_uuid;
        claim.claim_uuid = UUID_ZERO;
    }

    claim.claim_uuid = new_claim_id;
    if(localhost)
        localhost->aclk.claim_id_of_origin = claim.claim_uuid;

    spinlock_unlock(&claim.spinlock);
}

// returns true when the supplied str is a valid UUID.
// giving NULL, an empty string, or "NULL" is valid.
bool claim_id_set_str(const char *claim_id_str) {
    bool rc;

    ND_UUID uuid;
    if(!claim_id_str || !*claim_id_str || strcmp(claim_id_str, "NULL") == 0) {
        uuid = UUID_ZERO,
        rc = true;
    }
    else
        rc = uuid_parse(claim_id_str, uuid.uuid) == 0;

    claim_id_set(uuid);

    return rc;
}

ND_UUID claim_id_get_uuid(void) {
    static ND_UUID uuid;
    spinlock_lock(&claim.spinlock);
    uuid = claim.claim_uuid;
    spinlock_unlock(&claim.spinlock);
    return uuid;
}

void claim_id_get_str(char str[UUID_STR_LEN]) {
    ND_UUID uuid = claim_id_get_uuid();

    if(UUIDiszero(uuid))
        memset(str, 0, UUID_STR_LEN);
    else
        uuid_unparse_lower(uuid.uuid, str);
}

const char *claim_id_get_str_mallocz(void) {
    char *str = mallocz(UUID_STR_LEN);
    claim_id_get_str(str);
    return str;
}

CLAIM_ID claim_id_get(void) {
    CLAIM_ID ret = {
        .uuid = claim_id_get_uuid(),
    };

    if(claim_id_is_set(ret))
        uuid_unparse_lower(ret.uuid.uuid, ret.str);
    else
        ret.str[0] = '\0';

    return ret;
}

CLAIM_ID claim_id_get_last_working(void) {
    CLAIM_ID ret = { 0 };

    spinlock_lock(&claim.spinlock);
    ret.uuid = claim.claim_uuid_saved;
    spinlock_unlock(&claim.spinlock);

    if(claim_id_is_set(ret))
        uuid_unparse_lower(ret.uuid.uuid, ret.str);
    else
        ret.str[0] = '\0';

    return ret;
}

CLAIM_ID rrdhost_claim_id_get(RRDHOST *host) {
    CLAIM_ID ret = { 0 };

    if(host == localhost) {
        ret.uuid = claim_id_get_uuid();
        if(UUIDiszero(ret.uuid) || (!aclk_online() && !UUIDiszero(host->aclk.claim_id_of_parent)))
            ret.uuid = host->aclk.claim_id_of_parent;
    }
    else {
        if (!UUIDiszero(host->aclk.claim_id_of_origin))
            ret.uuid = host->aclk.claim_id_of_origin;
        else
            ret.uuid = host->aclk.claim_id_of_parent;
    }

    if(claim_id_is_set(ret))
        uuid_unparse_lower(ret.uuid.uuid, ret.str);

    return ret;
}
