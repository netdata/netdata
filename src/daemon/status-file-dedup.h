// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STATUS_FILE_DEDUP_H
#define NETDATA_STATUS_FILE_DEDUP_H

#include "libnetdata/libnetdata.h"
#include "status-file.h"

uint64_t daemon_status_file_hash(DAEMON_STATUS_FILE *ds, const char *msg, const char *cause);

void daemon_status_dedup_to_json(BUFFER *wb, DAEMON_STATUS_DEDUP *dp);
bool daemon_status_dedup_from_json(json_object *jobj, void *data, BUFFER *error);

bool dedup_already_posted(DAEMON_STATUS_FILE *ds, uint64_t hash, bool sentry);
void dedup_keep_hash(DAEMON_STATUS_FILE *ds, uint64_t hash, bool sentry);

#endif //NETDATA_STATUS_FILE_DEDUP_H
