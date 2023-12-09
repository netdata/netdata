// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_API_PROGRESS_H
#define NETDATA_WEB_API_PROGRESS_H 1

#include "../libnetdata.h"

void query_progress_start(uuid_t *transaction, usec_t started_ut, WEB_CLIENT_ACL acl, const char *query, const char *payload);
void query_progress_done_another(uuid_t *transaction, size_t done);
void query_progress_set_all(uuid_t *transaction, size_t all);
void query_progress_done(uuid_t *transaction, usec_t finished_ut, short int response_code, size_t response_size);

#endif // NETDATA_WEB_API_PROGRESS_H