// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_PARENTS_H
#define NETDATA_PULSE_PARENTS_H

#include "libnetdata/libnetdata.h"
#include "streaming/stream-handshake.h"

// receiver

void pulse_parent_receiver_waiting(int16_t hops);
void pulse_parent_receiver_not_waiting(int16_t hops);
void pulse_parent_receiver_running(int16_t hops);
void pulse_parent_receiver_not_running(int16_t hops, STREAM_HANDSHAKE reason);

// receiver events

void pulse_parent_stream_info_received_request(void);

void pulse_parent_receiver_request(void);
void pulse_parent_receiver_rejected(STREAM_HANDSHAKE reason);

void pulse_parent_node_permanent_added(void);
void pulse_parent_node_permanent_removed(void);

void pulse_parent_node_ephemeral_added(void);
void pulse_parent_node_ephemeral_removed(void);

// sender
void pulse_sender_running(int16_t hops);
void pulse_sender_not_running(int16_t hops, STREAM_HANDSHAKE reason, bool from_receiver);
void pulse_sender_connecting(void);
void pulse_sender_not_connecting(void);

void pulse_parents_do(bool extended);

#endif //NETDATA_PULSE_PARENTS_H
