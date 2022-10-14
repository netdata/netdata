// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_METADATA_H
#define NETDATA_SQLITE_METADATA_H

#include "sqlite3.h"
#include "sqlite_functions.h"

// To initialize and shutdown
void metadata_sync_init(void);
void metadata_sync_shutdown(void);
void metadata_sync_shutdown_prepare(void);

void metaqueue_dimension_update(RRDDIM *rd);
void metaqueue_chart_update(RRDSET *st);
void metaqueue_dimension_update_flags(RRDDIM *rd);
void metaqueue_host_update_system_info(RRDHOST *host);
void metaqueue_host_update_info(const char *machine_guid);
void metaqueue_delete_dimension_uuid(uuid_t *uuid);
void metaqueue_store_claim_id(uuid_t *host_uuid, uuid_t *claim_uuid);
void metaqueue_store_host_labels(const char *machine_guid);
void metaqueue_chart_labels(RRDSET *st);
void migrate_localhost(uuid_t *host_uuid);
void metaqueue_buffer(BUFFER *buffer);

// UNIT TEST
int metadata_unittest(void);
#endif //NETDATA_SQLITE_METADATA_H
