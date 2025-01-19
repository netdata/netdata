// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_PARENTS_H
#define NETDATA_PULSE_PARENTS_H

#include "libnetdata/libnetdata.h"
#include "streaming/stream-handshake.h"

void pulse_receiver_waiting(int16_t hops);
void pulse_receiver_not_waiting(int16_t hops);
void pulse_receiver_running(int16_t hops);
void pulse_receiver_not_running(int16_t hops, STREAM_HANDSHAKE reason);
void pulse_sender_running(int16_t hops);
void pulse_sender_not_running(int16_t hops, STREAM_HANDSHAKE reason, bool from_receiver);
void pulse_sender_connecting(void);
void pulse_sender_not_connecting(void);

#endif //NETDATA_PULSE_PARENTS_H
