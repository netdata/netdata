// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CLAIM_ID_H
#define NETDATA_CLAIM_ID_H

#include "claim.h"

void claim_id_keep_current(void);

bool claim_id_set_str(const char *claim_id_str);
void claim_id_set(ND_UUID new_claim_id);
void claim_id_clear_previous_working(void);
ND_UUID claim_id_get_uuid(void);
void claim_id_get_str(char str[UUID_STR_LEN]);
const char *claim_id_get_str_mallocz(void);

typedef struct {
    ND_UUID uuid;
    char str[UUID_STR_LEN];
} CLAIM_ID;

#define claim_id_is_set(claim_id) (!UUIDiszero(claim_id.uuid))

CLAIM_ID claim_id_get(void);
CLAIM_ID claim_id_get_last_working(void);
CLAIM_ID rrdhost_claim_id_get(RRDHOST *host);

#endif //NETDATA_CLAIM_ID_H
