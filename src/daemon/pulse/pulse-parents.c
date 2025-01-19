// SPDX-License-Identifier: GPL-3.0-or-later

#include "pulse-parents.h"

static size_t nodes_receivers_waiting = 0;
static size_t nodes_receivers_running = 0;
static size_t nodes_senders_running = 0;
static size_t nodes_senders_connecting = 0;

void pulse_receiver_waiting(int16_t hops) {
    __atomic_add_fetch(&nodes_receivers_waiting, 1, __ATOMIC_RELAXED);
}

void pulse_receiver_not_waiting(int16_t hops) {
    __atomic_sub_fetch(&nodes_receivers_waiting, 1, __ATOMIC_RELAXED);
}

void pulse_receiver_running(int16_t hops) {
    __atomic_add_fetch(&nodes_receivers_running, 1, __ATOMIC_RELAXED);
}

void pulse_receiver_not_running(int16_t hops, STREAM_HANDSHAKE reason) {
    __atomic_sub_fetch(&nodes_receivers_running, 1, __ATOMIC_RELAXED);
}

void pulse_sender_running(int16_t hops) {
    __atomic_add_fetch(&nodes_senders_running, 1, __ATOMIC_RELAXED);
}

void pulse_sender_not_running(int16_t hops, STREAM_HANDSHAKE reason, bool from_receiver) {
    __atomic_sub_fetch(&nodes_senders_running, 1, __ATOMIC_RELAXED);
}

void pulse_sender_connecting(void) {
    __atomic_add_fetch(&nodes_senders_running, 1, __ATOMIC_RELAXED);
}

void pulse_sender_not_connecting(void) {
    __atomic_sub_fetch(&nodes_senders_running, 1, __ATOMIC_RELAXED);
}
