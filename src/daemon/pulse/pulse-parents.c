// SPDX-License-Identifier: GPL-3.0-or-later

#include "pulse-parents.h"

// --------------------------------------------------------------------------------------------------------------------
// parents

static struct {
    // counters
    size_t stream_info_requests_received;
    size_t stream_requests_received;
    size_t stream_rejections_sent;
    size_t rejections_sent_by_reason[STREAM_HANDSHAKE_NEGATIVE_MAX];
    size_t disconnects_by_reason[STREAM_HANDSHAKE_NEGATIVE_MAX];

    // gauges
    size_t nodes_permanent;
    size_t nodes_ephemeral;
    size_t nodes_waiting;
    size_t nodes_running;
} parent;

void pulse_parent_stream_info_received_request(void) {
    __atomic_add_fetch(&parent.stream_info_requests_received, 1, __ATOMIC_RELAXED);
}

void pulse_parent_receiver_request(void) {
    __atomic_add_fetch(&parent.stream_requests_received, 1, __ATOMIC_RELAXED);
}

void pulse_parent_receiver_rejected(STREAM_HANDSHAKE reason) {
    __atomic_add_fetch(&parent.stream_rejections_sent, 1, __ATOMIC_RELAXED);

    int r = reason < 0 && reason > -STREAM_HANDSHAKE_NEGATIVE_MAX ? -reason : 0;
    __atomic_add_fetch(&parent.rejections_sent_by_reason[r], 1, __ATOMIC_RELAXED);
}

void pulse_parent_receiver_waiting(int16_t hops __maybe_unused) {
    __atomic_add_fetch(&parent.nodes_waiting, 1, __ATOMIC_RELAXED);
}

void pulse_parent_receiver_not_waiting(int16_t hops __maybe_unused) {
    __atomic_sub_fetch(&parent.nodes_waiting, 1, __ATOMIC_RELAXED);
}

void pulse_parent_receiver_running(int16_t hops __maybe_unused) {
    __atomic_add_fetch(&parent.nodes_running, 1, __ATOMIC_RELAXED);
}

void pulse_parent_receiver_not_running(int16_t hops __maybe_unused, STREAM_HANDSHAKE reason) {
    __atomic_sub_fetch(&parent.nodes_running, 1, __ATOMIC_RELAXED);

    int r = reason < 0 && reason > -STREAM_HANDSHAKE_NEGATIVE_MAX ? -reason : 0;
    __atomic_add_fetch(&parent.disconnects_by_reason[r], 1, __ATOMIC_RELAXED);
}

void pulse_parent_node_permanent_added(void) {
    __atomic_add_fetch(&parent.nodes_permanent, 1, __ATOMIC_RELAXED);
}

void pulse_parent_node_permanent_removed(void) {
    __atomic_sub_fetch(&parent.nodes_permanent, 1, __ATOMIC_RELAXED);
}

void pulse_parent_node_ephemeral_added(void) {
    __atomic_add_fetch(&parent.nodes_ephemeral, 1, __ATOMIC_RELAXED);
}

void pulse_parent_node_ephemeral_removed(void) {
    __atomic_sub_fetch(&parent.nodes_ephemeral, 1, __ATOMIC_RELAXED);
}

// --------------------------------------------------------------------------------------------------------------------
// children / senders

static struct {
    // counters
    size_t stream_info_requests_sent;
    size_t connection_attempts;
    size_t connections_rejected_by_reason[STREAM_HANDSHAKE_NEGATIVE_MAX];
    size_t disconnects_by_reason[STREAM_HANDSHAKE_NEGATIVE_MAX];

    // gauges
    size_t nodes_running;
    size_t nodes_connecting;
} sender;

void pulse_stream_info_sent_request(void) {
    __atomic_add_fetch(&sender.stream_info_requests_sent, 1, __ATOMIC_RELAXED);
}

void pulse_sender_running(int16_t hops __maybe_unused) {
    __atomic_add_fetch(&sender.nodes_running, 1, __ATOMIC_RELAXED);
}

void pulse_sender_not_running(int16_t hops __maybe_unused, STREAM_HANDSHAKE reason, bool from_receiver) {
    __atomic_sub_fetch(&sender.nodes_running, 1, __ATOMIC_RELAXED);

    if(from_receiver)
        reason = STREAM_HANDSHAKE_SND_DISCONNECT_RECEIVER_LEFT;

    int r = reason < 0 && reason > -STREAM_HANDSHAKE_NEGATIVE_MAX ? -reason : 0;
    __atomic_add_fetch(&sender.disconnects_by_reason[r], 1, __ATOMIC_RELAXED);
}

void pulse_sender_connecting(void) {
    __atomic_add_fetch(&sender.nodes_connecting, 1, __ATOMIC_RELAXED);
}

void pulse_sender_not_connecting(void) {
    __atomic_sub_fetch(&sender.nodes_connecting, 1, __ATOMIC_RELAXED);
}

void pulse_sender_connection_failed(const char *destination __maybe_unused, STREAM_HANDSHAKE reason) {
    int r = reason < 0 && reason > -STREAM_HANDSHAKE_NEGATIVE_MAX ? -reason : 0;
    __atomic_add_fetch(&sender.connections_rejected_by_reason[r], 1, __ATOMIC_RELAXED);
}

// --------------------------------------------------------------------------------------------------------------------

void pulse_parents_do(bool extended __maybe_unused) {

}