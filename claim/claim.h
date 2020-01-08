// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CLAIM_H
#define NETDATA_CLAIM_H 1

#include "../daemon/common.h"

extern char *claiming_pending_arguments;

void claim_agent(char *claiming_arguments);
char *is_agent_claimed(void);
void load_claiming_state(void);

#endif //NETDATA_CLAIM_H
