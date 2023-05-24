// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef NETDATA_SQLITE_DB_MIGRATION_H
#define NETDATA_SQLITE_DB_MIGRATION_H

#include "daemon/common.h"
#include "sqlite3.h"


int perform_database_migration(sqlite3 *database, int target_version);
int perform_context_database_migration(sqlite3 *database, int target_version);
int table_exists_in_database(const char *table);

#endif //NETDATA_SQLITE_DB_MIGRATION_H
