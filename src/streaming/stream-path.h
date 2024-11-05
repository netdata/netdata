// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_PATH_H
#define NETDATA_STREAM_PATH_H

#include "stream-capabilities.h"

#define STREAM_PATH_JSON_MEMBER "streaming_path"

typedef enum __attribute__((packed)) {
    STREAM_PATH_FLAG_NONE = 0,
    STREAM_PATH_FLAG_ACLK = (1 << 0),
} STREAM_PATH_FLAGS;

typedef struct stream_path {
    STRING *hostname;               // the hostname of the agent
    ND_UUID host_id;                // the machine guid of the agent
    ND_UUID node_id;                // the cloud node id of the agent
    ND_UUID claim_id;               // the cloud claim id of the agent
    time_t since;                   // the timestamp of the last update
    time_t first_time_t;            // the oldest timestamp in the db
    int16_t hops;                   // -1 = stale node, 0 = localhost, >0 the hops count
    STREAM_PATH_FLAGS flags;        // ACLK or NONE for the moment
    STREAM_CAPABILITIES capabilities; // streaming connection capabilities
    uint32_t start_time_ms;         // median time in ms the agent needs to start
    uint32_t shutdown_time_ms;      // median time in ms the agent needs to shutdown
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

void stream_path_retention_updated(struct rrdhost *host);
void stream_path_node_id_updated(struct rrdhost *host);

void stream_path_child_disconnected(struct rrdhost *host);
void stream_path_parent_disconnected(struct rrdhost *host);
STREAM_PATH rrdhost_stream_path_fetch(struct rrdhost *host);

bool stream_path_set_from_json(struct rrdhost *host, const char *json, bool from_parent);

#endif //NETDATA_STREAM_PATH_H
