// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_PATH_H
#define NETDATA_STREAM_PATH_H

#include "stream_capabilities.h"

#define STREAM_PATH_JSON_MEMBER "streaming_path"

typedef enum __attribute__((packed)) {
    STREAM_PATH_FLAG_NONE = 0,
    STREAM_PATH_FLAG_ACLK = (1 << 0),
} STREAM_PATH_FLAGS;

typedef struct stream_path {
    STRING *hostname;
    ND_UUID host_id;
    ND_UUID node_id;
    ND_UUID claim_id;
    time_t since;
    time_t first_time_t;
    time_t last_time_t;
    int16_t hops; // -1 = stale node, 0 = localhost, >0 the hops count
    STREAM_PATH_FLAGS flags;
    STREAM_CAPABILITIES capabilities;
} STREAM_PATH;

typedef struct rrdhost_stream_path {
    SPINLOCK spinlock;
    uint16_t size;
    uint16_t used;
    STREAM_PATH *array;
} RRDHOST_STREAM_PATH;


struct rrdhost;

void stream_path_send_to_parent(struct rrdhost *host);
void stream_path_send_to_child(struct rrdhost *host);

void rrdhost_stream_path_to_json(BUFFER *wb, struct rrdhost *host, const char *key, bool add_version);

void rrdhost_stream_path_clear(struct rrdhost *host, bool destroy);

void stream_path_child_disconnected(struct rrdhost *host);
void stream_path_parent_disconnected(struct rrdhost *host);

bool stream_path_set_from_json(struct rrdhost *host, const char *json, bool from_parent);

#endif //NETDATA_STREAM_PATH_H
