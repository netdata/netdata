// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CLAIM_H
#define NETDATA_CLAIM_H 1

#include "daemon/common.h"
#include "cloud-status.h"
#include "claim_id.h"

const char *claim_agent_failure_reason_get(void);
void claim_agent_failure_reason_set(const char *format, ...) PRINTFLIKE(1, 2);

extern struct config cloud_config;

bool claim_agent(const char *url, const char *token, const char *rooms, const char *proxy, bool insecure);
bool claim_agent_automatically(void);

bool claimed_id_save_to_file(const char *claimed_id_str);

bool is_agent_claimed(void);
bool claim_id_matches(const char *claim_id);
bool claim_id_matches_any(const char *claim_id);
bool load_claiming_state(void);
void cloud_conf_load(int silent);
void cloud_conf_init_after_registry(void);
bool cloud_conf_save(void);
bool cloud_conf_regenerate(const char *claimed_id_str, const char *machine_guid, const char *hostname, const char *token, const char *rooms, const char *url, const char *proxy, bool insecure);
CLOUD_STATUS claim_reload_and_wait_online(void);

const char *cloud_config_url_get(void);
void cloud_config_url_set(const char *url);
const char *cloud_config_proxy_get(void);
bool cloud_config_insecure_get(void);

#endif //NETDATA_CLAIM_H
