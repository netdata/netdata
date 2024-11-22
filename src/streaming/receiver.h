// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RECEIVER_H
#define NETDATA_RECEIVER_H

#include "daemon/common.h"
#include "stream-conf.h"

struct receiver_state;
uint32_t stream_currently_connected_receivers(void);

int rrdpush_receiver_thread_spawn(struct web_client *w, char *decoded_query_string, void *h2o_ctx);

void stream_receiver_cancel_threads(void);
bool receiver_has_capability(RRDHOST *host, STREAM_CAPABILITIES caps);

void receiver_state_free(struct receiver_state *rpt);
bool stop_streaming_receiver(RRDHOST *host, STREAM_HANDSHAKE reason);

char *receiver_program_version_strdupz(RRDHOST *host);

#endif //NETDATA_RECEIVER_H
