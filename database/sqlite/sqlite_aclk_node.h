// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_NODE_H
#define NETDATA_SQLITE_ACLK_NODE_H


extern sqlite3 *db_meta;
void sql_build_node_info(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_reset_node_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_drop_host_aclk_table_list(uuid_t *host_uuid);

#endif //NETDATA_SQLITE_ACLK_NODE_H
