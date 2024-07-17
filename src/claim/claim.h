// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CLAIM_H
#define NETDATA_CLAIM_H 1

#include "daemon/common.h"

typedef enum __attribute__((packed)) {
    CLOUD_STATUS_AVAILABLE = 1,     // cloud and aclk functionality is available, but the agent is not claimed
    CLOUD_STATUS_BANNED,            // the agent has been banned from cloud
    CLOUD_STATUS_OFFLINE,           // the agent tries to connect to cloud, but cannot do it
    CLOUD_STATUS_ONLINE,            // the agent is connected to cloud
} CLOUD_STATUS;

const char *cloud_status_to_string(CLOUD_STATUS status);
CLOUD_STATUS cloud_status(void);
time_t cloud_last_change(void);
time_t cloud_next_connection_attempt(void);
size_t cloud_connection_id(void);
const char *cloud_status_aclk_offline_reason(void);
const char *cloud_status_aclk_base_url(void);
CLOUD_STATUS buffer_json_cloud_status(BUFFER *wb, time_t now_s);

const char *claim_agent_failure_reason_get(void);
void claim_agent_failure_reason_set(const char *reason);

extern struct config cloud_config;

bool claim_agent(const char *url, const char *token, const char *rooms, const char *proxy, bool insecure);
bool claim_agent_automatically(void);

bool claimed_id_save_to_file(const char *claimed_id_str);

char *aclk_get_claimed_id(void);
bool load_claiming_state(void);
void cloud_conf_load(int silent);
void cloud_conf_init_after_registry(void);
bool cloud_conf_save(void);
void cloud_conf_regenerate(const char *claimed_id_str, const char *machine_guid, const char *hostname, const char *token, const char *rooms, const char *url, const char *proxy, int insecure);
CLOUD_STATUS claim_reload_and_wait_online(void);

bool netdata_random_session_id_generate(void);
const char *netdata_random_session_id_get_filename(void);
bool netdata_random_session_id_matches(const char *guid);
int api_v2_claim(struct web_client *w, char *url);

const char *cloud_url(void);
const char *cloud_proxy(void);
bool cloud_insecure(void);

#endif //NETDATA_CLAIM_H
