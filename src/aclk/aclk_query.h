// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ACLK_QUERY_H
#define NETDATA_ACLK_QUERY_H

#include "libnetdata/libnetdata.h"

#include "mqtt_websockets/mqtt_wss_client.h"

#include "aclk_query_queue.h"

int mark_pending_req_cancelled(const char *msg_id);
void mark_pending_req_cancel_all();

void aclk_execute_query(aclk_query_t query);
void aclk_query_init(mqtt_wss_client client);
int http_api_v2(mqtt_wss_client client, aclk_query_t query);
int send_bin_msg(mqtt_wss_client client, aclk_query_t query);

#endif //NETDATA_AGENT_CLOUD_LINK_H
