#ifndef ACLK_RRDHOST_STATE_H
#define ACLK_RRDHOST_STATE_H

#include "libnetdata/libnetdata.h"

#ifdef ACLK_LEGACY
typedef enum aclk_cmd {
    ACLK_CMD_CLOUD,
    ACLK_CMD_ONCONNECT,
    ACLK_CMD_INFO,
    ACLK_CMD_CHART,
    ACLK_CMD_CHARTDEL,
    ACLK_CMD_ALARM,
    ACLK_CMD_CLOUD_QUERY_2,
    ACLK_CMD_CHILD_CONNECT,
    ACLK_CMD_CHILD_DISCONNECT
} ACLK_CMD;

typedef enum aclk_metadata_state {
    ACLK_METADATA_REQUIRED,
    ACLK_METADATA_CMD_QUEUED,
    ACLK_METADATA_SENT
} ACLK_METADATA_STATE;
#endif

typedef enum aclk_agent_state {
    ACLK_HOST_INITIALIZING,
    ACLK_HOST_STABLE
} ACLK_AGENT_STATE;

typedef struct aclk_rrdhost_state {
    char *claimed_id; // Claimed ID if host has one otherwise NULL
    char *prev_claimed_id; // Claimed ID if changed (reclaimed) during runtime

#ifdef ACLK_LEGACY
    // per child popcorning
    ACLK_AGENT_STATE state;
    ACLK_METADATA_STATE metadata;

    time_t timestamp_created;
    time_t t_last_popcorn_update;
#endif /* ACLK_LEGACY */
} aclk_rrdhost_state;

#endif /* ACLK_RRDHOST_STATE_H */
