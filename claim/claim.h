// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CLAIM_H
#define NETDATA_CLAIM_H 1

#include "daemon/common.h"

extern char *claiming_pending_arguments;
extern struct config cloud_config;

void claim_agent(char *claiming_arguments);
char *get_agent_claimid(void);
void load_claiming_state(void);
void load_cloud_conf(int silent);

bool netdata_random_session_id_generate(void);
const char *netdata_random_session_id_get_filename(void);
bool netdata_random_session_id_matches(const char *guid);
int api_v2_claim(struct web_client *w, char *url);

#endif //NETDATA_CLAIM_H
