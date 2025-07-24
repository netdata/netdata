// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_H
#define NETDATA_STREAM_H 1

#include "libnetdata/libnetdata.h"
#include "stream-traffic-types.h"

struct rrdhost;
struct sender_state;
struct receiver_state;

#include "stream-conf.h"
#include "stream-handshake.h"
#include "stream-capabilities.h"
#include "stream-parents.h"

// starting and stopping senders
void *stream_sender_start_localhost(void *ptr);
void stream_sender_start_host(struct rrdhost *host);
void stream_sender_signal_to_stop_and_wait(struct rrdhost *host, STREAM_HANDSHAKE reason, bool wait);
void stream_connector_remove_host(RRDHOST *host);

// managing host sender structures
void stream_sender_structures_init(struct rrdhost *host, bool stream, STRING *parents, STRING *api_key, STRING *send_charts_matching);
void stream_sender_structures_free(struct rrdhost *host);

// querying host sender information
bool stream_sender_is_connected_with_ssl(struct rrdhost *host);
bool stream_sender_has_compression(struct rrdhost *host);
bool stream_sender_has_capabilities(struct rrdhost *host, STREAM_CAPABILITIES capabilities);

// receiver API
uint32_t stream_receivers_currently_connected(void);
struct web_client;
int stream_receiver_accept_connection(struct web_client *w, char *decoded_query_string);
bool receiver_has_capability(struct rrdhost *host, STREAM_CAPABILITIES caps);
void stream_receiver_free(struct receiver_state *rpt);
bool stream_receiver_signal_to_stop_and_wait(struct rrdhost *host, STREAM_HANDSHAKE reason);
char *stream_receiver_program_version_strdupz(struct rrdhost *host);

#include "database/rrdhost-status.h"
#include "protocol/commands.h"
#include "stream-path.h"
#include "stream-control.h"

void stream_threads_cancel(void);

#endif //NETDATA_STREAM_H
