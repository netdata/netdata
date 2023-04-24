// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_LABEL_H
#define NETDATA_SQLITE_LABEL_H

#include "daemon/common.h"
#include "sqlite3.h"

extern sqlite3 *db_label_meta;

int sql_init_label_database(int memory);
#endif //NETDATA_SQLITE_LABEL_H
