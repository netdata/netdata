// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_NODE_H
#define NETDATA_SQLITE_ACLK_NODE_H

void aclk_check_node_info_and_collectors(void);
void send_node_info_with_wait(RRDHOST *host);
void send_node_update_with_wait(RRDHOST *host, int live, int queryable);
#endif //NETDATA_SQLITE_ACLK_NODE_H
