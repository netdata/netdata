// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_NODE_H
#define NETDATA_SQLITE_ACLK_NODE_H

void sql_build_node_info(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_build_node_collectors(struct aclk_database_worker_config *wc);
#endif //NETDATA_SQLITE_ACLK_NODE_H
