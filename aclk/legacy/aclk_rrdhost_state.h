#ifndef ACLK_RRDHOST_STATE_H
#define ACLK_RRDHOST_STATE_H

#include "../../libnetdata/libnetdata.h"

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

typedef enum aclk_agent_state {
    ACLK_HOST_INITIALIZING,
    ACLK_HOST_STABLE
} ACLK_POPCORNING_STATE;

typedef struct aclk_rrdhost_state {
    char *claimed_id; // Claimed ID if host has one otherwise NULL

#ifdef ENABLE_ACLK
    // per child popcorning
    ACLK_POPCORNING_STATE state;
    ACLK_METADATA_STATE metadata;

    time_t timestamp_created;
    time_t t_last_popcorn_update;
#endif /* ENABLE_ACLK */
} aclk_rrdhost_state;

#endif /* ACLK_RRDHOST_STATE_H */
