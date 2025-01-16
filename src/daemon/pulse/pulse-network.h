// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_NETWORK_H
#define NETDATA_PULSE_NETWORK_H

#include "daemon/common.h"

void pulse_network_do(bool extended);

void pulse_web_server_received_bytes(size_t bytes);
void pulse_web_server_sent_bytes(size_t bytes);

void pulse_statsd_received_bytes(size_t bytes);
void pulse_statsd_sent_bytes(size_t bytes);

void pulse_stream_received_bytes(size_t bytes);
void pulse_stream_sent_bytes(size_t bytes);

#endif //NETDATA_PULSE_NETWORK_H
