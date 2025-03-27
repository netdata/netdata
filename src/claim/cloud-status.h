// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CLOUD_STATUS_H
#define NETDATA_CLOUD_STATUS_H

#include "libnetdata/libnetdata.h"
#include "claim/cloud-status.h"

typedef enum __attribute__((packed)) {
    CLOUD_STATUS_AVAILABLE = 1,     // cloud and aclk functionality is available, but the agent is not claimed
    CLOUD_STATUS_BANNED,            // the agent has been banned from cloud
    CLOUD_STATUS_OFFLINE,           // the agent tries to connect to cloud, but cannot do it
    CLOUD_STATUS_INDIRECT,          // the agent is connected to cloud via a parent
    CLOUD_STATUS_ONLINE,            // the agent is connected to cloud
} CLOUD_STATUS;

ENUM_STR_DEFINE_FUNCTIONS_EXTERN(CLOUD_STATUS);

CLOUD_STATUS cloud_status(void);

time_t cloud_last_change(void);
time_t cloud_next_connection_attempt(void);
size_t cloud_connection_id(void);
const char *cloud_status_aclk_offline_reason(void);
const char *cloud_status_aclk_base_url(void);
CLOUD_STATUS buffer_json_cloud_status(BUFFER *wb, time_t now_s);

#endif //NETDATA_CLOUD_STATUS_H
