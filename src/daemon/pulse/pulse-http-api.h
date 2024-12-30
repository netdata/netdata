// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_HTTP_API_H
#define NETDATA_PULSE_HTTP_API_H

#include "daemon/common.h"

void pulse_web_client_connected(void);
void pulse_web_client_disconnected(void);

void pulse_web_request_completed(uint64_t dt,
                                     uint64_t bytes_received,
                                     uint64_t bytes_sent,
                                     uint64_t content_size,
                                     uint64_t compressed_content_size);

#if defined(PULSE_INTERNALS)
void pulse_web_do(bool extended);
#endif

#endif //NETDATA_PULSE_HTTP_API_H
