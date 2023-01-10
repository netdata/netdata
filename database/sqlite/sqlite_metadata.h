// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_METADATA_H
#define NETDATA_SQLITE_METADATA_H

#include "sqlite3.h"
#include "sqlite_functions.h"

// To initialize and shutdown
void metadata_sync_init(void);
void metadata_sync_shutdown(void);
void metadata_sync_shutdown_prepare(void);

void metaqueue_delete_dimension_uuid(uuid_t *uuid);
void migrate_localhost(uuid_t *host_uuid);

// UNIT TEST
int metadata_unittest(void);
#endif //NETDATA_SQLITE_METADATA_H
