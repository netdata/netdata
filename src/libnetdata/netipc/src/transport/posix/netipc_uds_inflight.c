#include "netipc_uds_internal.h"

#include <stdlib.h>

int nipc_uds_inflight_add(nipc_uds_session_t *s, uint64_t id)
{
    for (uint32_t i = 0; i < s->inflight_count; i++) {
        if (s->inflight_ids[i] == id)
            return -1;
    }

    if (s->inflight_count >= s->inflight_capacity) {
        uint32_t new_cap = s->inflight_capacity ? s->inflight_capacity * 2 : 16;
        uint64_t *new_ids = realloc(s->inflight_ids,
                                    (size_t)new_cap * sizeof(uint64_t));
        if (!new_ids)
            return -2;
        s->inflight_ids = new_ids;
        s->inflight_capacity = new_cap;
    }

    s->inflight_ids[s->inflight_count++] = id;
    return 0;
}

int nipc_uds_inflight_remove(nipc_uds_session_t *s, uint64_t id)
{
    for (uint32_t i = 0; i < s->inflight_count; i++) {
        if (s->inflight_ids[i] == id) {
            s->inflight_ids[i] = s->inflight_ids[s->inflight_count - 1];
            s->inflight_count--;
            return 0;
        }
    }
    return -1;
}

void nipc_uds_inflight_fail_all(nipc_uds_session_t *s)
{
    if (!s || s->role != NIPC_UDS_ROLE_CLIENT)
        return;

    s->inflight_count = 0;
}
