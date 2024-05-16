// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_QUERY_PROGRESS_H
#define NETDATA_QUERY_PROGRESS_H 1

#include "../libnetdata.h"

void query_progress_start_or_update(nd_uuid_t *transaction, usec_t started_ut, HTTP_REQUEST_MODE mode, HTTP_ACL acl, const char *query, BUFFER *payload, const char *client);
void query_progress_done_step(nd_uuid_t *transaction, size_t done);
void query_progress_set_finish_line(nd_uuid_t *transaction, size_t all);
void query_progress_finished(nd_uuid_t *transaction, usec_t finished_ut, short int response_code, usec_t duration_ut, size_t response_size, size_t sent_size);
void query_progress_functions_update(nd_uuid_t *transaction, size_t done, size_t all);

int web_api_v2_report_progress(nd_uuid_t *transaction, BUFFER *wb);

#define RRDFUNCTIONS_PROGRESS_HELP "View the progress on the running and latest Netdata API Requests"
int progress_function_result(BUFFER *wb, const char *hostname);

#endif // NETDATA_QUERY_PROGRESS_H
