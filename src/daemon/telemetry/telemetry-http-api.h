// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TELEMETRY_HTTP_API_H
#define NETDATA_TELEMETRY_HTTP_API_H

#include "daemon/common.h"

uint64_t telemetry_web_client_connected(void);
void telemetry_web_client_disconnected(void);

void telemetry_web_request_completed(uint64_t dt,
                                     uint64_t bytes_received,
                                     uint64_t bytes_sent,
                                     uint64_t content_size,
                                     uint64_t compressed_content_size);

#if defined(TELEMETRY_INTERNALS)
void telemetry_web_do(bool extended);
#endif

#endif //NETDATA_TELEMETRY_HTTP_API_H
