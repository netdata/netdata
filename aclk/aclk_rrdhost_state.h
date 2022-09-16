#ifndef ACLK_RRDHOST_STATE_H
#define ACLK_RRDHOST_STATE_H

#include "libnetdata/libnetdata.h"

typedef struct aclk_rrdhost_state {
    char *claimed_id; // Claimed ID if host has one otherwise NULL
    char *prev_claimed_id; // Claimed ID if changed (reclaimed) during runtime
} aclk_rrdhost_state;

#endif /* ACLK_RRDHOST_STATE_H */
