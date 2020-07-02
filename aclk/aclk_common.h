#ifndef ACLK_COMMON_H
#define ACLK_COMMON_H

#include "libnetdata/libnetdata.h"

extern netdata_mutex_t aclk_shared_state_mutex;
#define ACLK_SHARED_STATE_LOCK netdata_mutex_lock(&aclk_shared_state_mutex)
#define ACLK_SHARED_STATE_UNLOCK netdata_mutex_unlock(&aclk_shared_state_mutex)

typedef enum aclk_cmd {
    ACLK_CMD_CLOUD,
    ACLK_CMD_ONCONNECT,
    ACLK_CMD_INFO,
    ACLK_CMD_CHART,
    ACLK_CMD_CHARTDEL,
    ACLK_CMD_ALARM,
    ACLK_CMD_MAX
} ACLK_CMD;

typedef enum aclk_metadata_state {
    ACLK_METADATA_REQUIRED,
    ACLK_METADATA_CMD_QUEUED,
    ACLK_METADATA_SENT
} ACLK_METADATA_STATE;

typedef enum aclk_agent_state {
    AGENT_INITIALIZING,
    AGENT_STABLE
} ACLK_AGENT_STATE;
extern struct aclk_shared_state {
    ACLK_METADATA_STATE metadata_submitted;
    ACLK_AGENT_STATE agent_state;
    time_t last_popcorn_interrupt;
} aclk_shared_state;

typedef enum aclk_proxy_type {
    PROXY_TYPE_UNKNOWN = 0,
    PROXY_TYPE_SOCKS5,
    PROXY_TYPE_HTTP,
    PROXY_DISABLED,
    PROXY_NOT_SET,
} ACLK_PROXY_TYPE;

const char *aclk_proxy_type_to_s(ACLK_PROXY_TYPE *type);

#define ACLK_PROXY_PROTO_ADDR_SEPARATOR "://"
#define ACLK_PROXY_ENV "env"
#define ACLK_PROXY_CONFIG_VAR "proxy"

ACLK_PROXY_TYPE aclk_verify_proxy(const char *string);
const char *aclk_lws_wss_get_proxy_setting(ACLK_PROXY_TYPE *type);
void safe_log_proxy_censor(char *proxy);
int aclk_decode_base_url(char *url, char **aclk_hostname, char **aclk_port);
const char *aclk_get_proxy(ACLK_PROXY_TYPE *type);

#endif //ACLK_COMMON_H
