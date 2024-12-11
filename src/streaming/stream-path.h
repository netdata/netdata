// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_PATH_H
#define NETDATA_STREAM_PATH_H

#include "stream-capabilities.h"

#define STREAM_PATH_JSON_MEMBER "streaming_path"

typedef struct stream_path STREAM_PATH;

typedef struct rrdhost_stream_path {
    RW_SPINLOCK spinlock;
    uint16_t size;
    uint16_t used;
    STREAM_PATH *array;
} RRDHOST_STREAM_PATH;

struct rrdhost;

void rrdhost_stream_path_init(struct rrdhost *host);

void stream_path_send_to_parent(struct rrdhost *host);
void stream_path_send_to_child(struct rrdhost *host);

void rrdhost_stream_path_to_json(BUFFER *wb, struct rrdhost *host, const char *key, bool add_version);
void rrdhost_stream_path_clear(struct rrdhost *host, bool destroy);

void stream_path_retention_updated(struct rrdhost *host);
void stream_path_node_id_updated(struct rrdhost *host);

void stream_path_child_disconnected(struct rrdhost *host);
void stream_path_parent_disconnected(struct rrdhost *host);

uint64_t rrdhost_stream_path_total_reboot_time_ms(struct rrdhost *host);

bool stream_path_set_from_json(struct rrdhost *host, const char *json, bool from_parent);

bool rrdhost_is_host_in_stream_path_before_us(struct rrdhost *host, ND_UUID remote_agent_host_id, int16_t our_hops);

#endif //NETDATA_STREAM_PATH_H
