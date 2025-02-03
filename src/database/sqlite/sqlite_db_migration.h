// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef NETDATA_SQLITE_DB_MIGRATION_H
#define NETDATA_SQLITE_DB_MIGRATION_H

#ifdef __cplusplus
extern "C" {
#endif

#include "database/rrd.h"
#include "database/sqlite/vendored/sqlite3.h"

int perform_database_migration(sqlite3 *database, int target_version);
int perform_context_database_migration(sqlite3 *database, int target_version);
int table_exists_in_database(sqlite3 *database, const char *table);
int perform_ml_database_migration(sqlite3 *database, int target_version);

#ifdef __cplusplus
}
#endif

#endif //NETDATA_SQLITE_DB_MIGRATION_H
