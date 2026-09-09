// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ACLK_QUERY_QUEUE_H
#define NETDATA_ACLK_QUERY_QUEUE_H

#include "database/rrd.h"
#include "schema-wrappers/schema_wrappers.h"

#include "aclk_util.h"

typedef enum {
    UNKNOWN = 0,
    HTTP_API_V2,
    REGISTER_NODE,
    NODE_STATE_UPDATE,
    UPDATE_NODE_INFO,
    UPDATE_NODE_MANIFEST,
    ALARM_PROVIDE_CFG,
    ALARM_SNAPSHOT,
    UPDATE_NODE_COLLECTORS,
    CTX_SEND_SNAPSHOT,              // Context snapshot to the cloud
    CTX_SEND_SNAPSHOT_UPD,          // Context incremental update to the cloud
    CTX_CHECKPOINT,                 // Context checkpoint from the cloud
    CTX_STOP_STREAMING,             // Context stop streaming
    CREATE_NODE_INSTANCE,           // Create node instance on the agent
    SEND_NODE_INSTANCES,            // Send node instances to the cloud
    ALERT_START_STREAMING,          // Start alert streaming from cloud
    ALERT_CHECKPOINT,               // Do an alert version check
    ACLK_QUERY_TYPE_COUNT           // always keep this as last
} aclk_query_type_t;

struct aclk_query_http_api_v2 {
    char *payload;
    char *query;
};

struct aclk_bin_payload {
    char *payload;
    size_t size;
    enum aclk_topics topic;
    const char *msg_name;
};

// UPDATE_NODE_MANIFEST only: what the host config recorded when this message was enqueued, so the
// record can be undone if the message never reaches the mqtt layer. Without that, an unsent manifest
// keeps suppressing every later identical build for the rest of the ACLK session.
// See aclk_node_manifest_publish_result().
//
// The host is identified by machine_guid, not by the node_id the manifest is keyed under at the
// cloud: node_id is mutable and host->node_id can disagree with the ACLK config's copy, so it does
// not identify the config to write back to. machine_guid is immutable for the host's lifetime.
// The record is identified by token alone, never by the suppression key it was stored beside: two
// distinct enqueues of identical content share a key, so a key comparison would let a stale drop
// invalidate a later enqueue's record. The token is unique per enqueue and is strictly stronger.
struct aclk_manifest_publication {
    char machine_guid[GUID_LEN + 1]; // the host whose config recorded this send
    uint64_t token;                  // identifies THIS enqueue, so a drop cannot invalidate a later one
    bool published;                  // set once the message reached the mqtt layer
};

// ----------------------------------------------------------------------------
// Reference-counted completion for safe timed waits
// Both waiter and query hold a reference; last one to release frees the structure

struct aclk_sync_completion {
    struct completion compl;
    int32_t refcount;
};

static inline struct aclk_sync_completion *aclk_sync_completion_create(void) {
    struct aclk_sync_completion *sc = callocz(1, sizeof(*sc));
    completion_init(&sc->compl);
    sc->refcount = 2;  // One for waiter, one for query
    return sc;
}

static inline void aclk_sync_completion_release(struct aclk_sync_completion *sc) {
    if (__atomic_sub_fetch(&sc->refcount, 1, __ATOMIC_ACQ_REL) == 0) {
        completion_destroy(&sc->compl);
        freez(sc);
    }
}

// Called by query processing to signal completion and release query's reference
static inline void aclk_sync_completion_signal(struct aclk_sync_completion *sc) {
    completion_mark_complete(&sc->compl);
    aclk_sync_completion_release(sc);
}

// Called by waiter - waits with timeout, then releases waiter's reference
// Returns true if completed within timeout, false if timed out
static inline bool aclk_sync_completion_timedwait(struct aclk_sync_completion *sc, uint64_t timeout_s) {
    bool result = completion_timedwait_for(&sc->compl, timeout_s);
    aclk_sync_completion_release(sc);
    return result;
}

// ----------------------------------------------------------------------------

typedef struct {
    aclk_query_type_t type;
    bool allocated;

    // dedup_id is used to deduplicate queries in the list
    // if type and dedup_id is the same message is deduplicated
    // set dedup_id to NULL to never deduplicate the message
    // set dedup_id to constant (e.g. empty string "") to make
    // message of this type ever exist only once in the list
    char *dedup_id;
    char *callback_topic;
    char *msg_id;
    union {
        char *claim_id;
        char *machine_guid;
    };

    struct timeval created_tv;
    usec_t created;
    int timeout;

    uint64_t version;
    union {
        struct aclk_query_http_api_v2 http_api_v2;
        struct aclk_bin_payload bin_payload;
        void *payload;
        char *node_id;
    } data;
    struct aclk_manifest_publication manifest;
    struct aclk_sync_completion *sync_completion;
} aclk_query_t;

aclk_query_t *aclk_query_new(aclk_query_type_t type);
void aclk_query_free(aclk_query_t *query);

void aclk_execute_query(aclk_query_t *query);
void aclk_add_job(aclk_query_t *query);

// Applies the outcome of a manifest publication to the host config. A message that went out needs
// nothing - it was recorded when it was enqueued; a dropped one has that record invalidated and its
// request re-armed. Implemented on the sqlite side (sqlite_aclk_node.c), which owns that state - same
// split as aclk_execute_query() above.
void aclk_node_manifest_publish_result(const struct aclk_manifest_publication *publication);

#define QUEUE_IF_PAYLOAD_PRESENT(query)                                                                                \
    do {                                                                                                               \
        if (likely((query)->data.bin_payload.payload)) {                                                               \
            aclk_execute_query(query);                                                                                 \
        } else {                                                                                                       \
            nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to generate payload");                                               \
            aclk_query_free(query);                                                                                    \
        }                                                                                                              \
    } while (0)

#endif /* NETDATA_ACLK_QUERY_QUEUE_H */
