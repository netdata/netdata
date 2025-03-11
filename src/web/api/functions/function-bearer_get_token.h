// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FUNCTION_BEARER_GET_TOKEN_H
#define NETDATA_FUNCTION_BEARER_GET_TOKEN_H

#include "database/rrd.h"

int function_bearer_get_token(BUFFER *wb, const char *function, BUFFER *payload, const char *source);
int call_function_bearer_get_token(RRDHOST *host, struct web_client *w, const char *claim_id, const char *machine_guid, const char *node_id);

#define RRDFUNCTIONS_BEARER_GET_TOKEN "bearer_get_token"
#define RRDFUNCTIONS_BEARER_GET_TOKEN_HELP "Get a bearer token for authenticated direct access to the agent"

#endif //NETDATA_FUNCTION_BEARER_GET_TOKEN_H
