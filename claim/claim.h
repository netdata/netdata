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

#endif //NETDATA_CLAIM_H
