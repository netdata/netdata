// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_H
#define NETDATA_SQLITE_ACLK_H

extern void sql_queue_chart_to_aclk(RRDSET *st, int cmd);
//extern sqlite3 *db_meta;
extern void sql_create_aclk_table(RRDHOST *host);

#endif //NETDATA_SQLITE_ACLK_H
