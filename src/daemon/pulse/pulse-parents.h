// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_PARENTS_H
#define NETDATA_PULSE_PARENTS_H

#include "libnetdata/libnetdata.h"
#include "streaming/stream-handshake.h"

typedef enum {
    PULSE_HOST_STATUS_NONE                  = 0,
    PULSE_HOST_STATUS_LOCAL                 = (1 << 0),
    PULSE_HOST_STATUS_VIRTUAL               = (1 << 1),
    PULSE_HOST_STATUS_LOADING               = (1 << 2),
    PULSE_HOST_STATUS_ARCHIVED              = (1 << 3),
    PULSE_HOST_STATUS_RCV_OFFLINE           = (1 << 4),
    PULSE_HOST_STATUS_RCV_WAITING           = (1 << 5),
    PULSE_HOST_STATUS_RCV_REPLICATING       = (1 << 6),
    PULSE_HOST_STATUS_RCV_REPLICATION_WAIT  = (1 << 7),
    PULSE_HOST_STATUS_RCV_RUNNING           = (1 << 8),
    PULSE_HOST_STATUS_SND_OFFLINE           = (1 << 9),
    PULSE_HOST_STATUS_SND_PENDING           = (1 << 10),
    PULSE_HOST_STATUS_SND_CONNECTING        = (1 << 11),
    PULSE_HOST_STATUS_SND_NO_DST            = (1 << 12),
    PULSE_HOST_STATUS_SND_NO_DST_FAILED     = (1 << 13),
    PULSE_HOST_STATUS_SND_WAITING           = (1 << 14),
    PULSE_HOST_STATUS_SND_REPLICATING       = (1 << 15),
    PULSE_HOST_STATUS_SND_RUNNING           = (1 << 16),
    PULSE_HOST_STATUS_DELETED               = (1 << 17),
    PULSE_HOST_STATUS_EPHEMERAL             = (1 << 18),
    PULSE_HOST_STATUS_PERMANENT             = (1 << 19),
} PULSE_HOST_STATUS;

void pulse_host_status(RRDHOST *host, PULSE_HOST_STATUS status, STREAM_HANDSHAKE reason);

// receiver events

void pulse_parent_stream_info_received_request(void);
void pulse_parent_receiver_request(void);
void pulse_parent_receiver_rejected(STREAM_HANDSHAKE reason);

// sender
void pulse_stream_info_sent_request(void);
void pulse_sender_stream_info_failed(const char *destination __maybe_unused, STREAM_HANDSHAKE reason);
void pulse_sender_connection_failed(const char *destination __maybe_unused, STREAM_HANDSHAKE reason);

void pulse_parents_do(bool extended);

#endif //NETDATA_PULSE_PARENTS_H
