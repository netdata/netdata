// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CLAIM_H
#define NETDATA_CLAIM_H 1

#include "daemon/common.h"

extern char *claiming_pending_arguments;
extern struct config cloud_config;

typedef enum __attribute__((packed)) {
    CLAIM_AGENT_OK,
    CLAIM_AGENT_CLOUD_DISABLED,
    CLAIM_AGENT_NO_CLOUD_URL,
    CLAIM_AGENT_CANNOT_EXECUTE_CLAIM_SCRIPT,
    CLAIM_AGENT_CLAIM_SCRIPT_FAILED,
    CLAIM_AGENT_CLAIM_SCRIPT_RETURNED_INVALID_CODE,
    CLAIM_AGENT_FAILED_WITH_MESSAGE,
} CLAIM_AGENT_RESPONSE;

CLAIM_AGENT_RESPONSE claim_agent(const char *claiming_arguments, bool force, const char **msg);
char *get_agent_claimid(void);
void load_claiming_state(void);
void load_cloud_conf(int silent);
void claim_reload_all(void);

bool netdata_random_session_id_generate(void);
const char *netdata_random_session_id_get_filename(void);
bool netdata_random_session_id_matches(const char *guid);
int api_v2_claim(struct web_client *w, char *url);

#endif //NETDATA_CLAIM_H
