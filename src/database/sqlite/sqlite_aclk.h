// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_H
#define NETDATA_SQLITE_ACLK_H

#define ACLK_MAX_ALERT_UPDATES  "50"
#define ACLK_SYNC_QUERY_SIZE 512

static inline int uuid_parse_fix(char *in, nd_uuid_t uuid)
{
    in[8] = '-';
    in[13] = '-';
    in[18] = '-';
    in[23] = '-';
    return uuid_parse(in, uuid);
}

enum aclk_database_opcode {
    ACLK_DATABASE_NOOP = 0,
    ACLK_DATABASE_NODE_STATE,
    ACLK_DATABASE_PUSH_ALERT_CONFIG,
    ACLK_DATABASE_NODE_UNREGISTER,
    ACLK_MQTT_WSS_CLIENT_SET,
    ACLK_MQTT_WSS_CLIENT_RESET,
    ACLK_CANCEL_NODE_UPDATE_TIMER,
    ACLK_QUEUE_NODE_INFO,
    ACLK_QUERY_EXECUTE,
    ACLK_QUERY_BATCH_ADD,
    ACLK_QUERY_BATCH_EXECUTE,
    ACLK_SYNC_SHUTDOWN,

    // leave this last
    // we need it to check for worker utilization
    ACLK_MAX_ENUMERATIONS_DEFINED
};

enum aclk_alert_snapshot_state {
    ACLK_ALERT_SNAPSHOT_IDLE = 0,
    ACLK_ALERT_SNAPSHOT_PENDING,
    ACLK_ALERT_SNAPSHOT_READY,
};

typedef time_t aclk_send_timestamp_t __attribute__((aligned(8)));

typedef struct aclk_sync_cfg_t {
    RRDHOST *host;
    uv_timer_t timer;
    bool timer_initialized;

    // pending context checkpoint - saved when deferred during context load or post-processing
    // protected by pending_ctx_spinlock (accessed from ACLK query thread and context worker thread)
    SPINLOCK pending_ctx_spinlock;
    uint64_t pending_ctx_generation;
    bool pending_ctx_checkpoint;
    char *pending_ctx_claim_id;
    char *pending_ctx_node_id;
    uint64_t pending_ctx_version_hash;
    time_t pending_ctx_saved_monotonic_s;

    int8_t send_snapshot; // atomic: shared by health, query, and alert-push workers
    bool stream_alerts;   // atomic: published by query worker and read by alert-push/status threads
    int alert_count;
    int snapshot_count;
    int checkpoint_count;
    aclk_send_timestamp_t node_info_send_time;     // atomic: event loop to alert-push worker
    aclk_send_timestamp_t node_collectors_send;    // atomic: node-info builders to alert-push worker
    aclk_send_timestamp_t node_manifest_send_time; // atomic: armed by collector/streaming/pluginsd
                                                   // threads and by build_node_info(); claimed by
                                                   // the alert-push worker
    SPINLOCK node_id_spinlock;
    char node_id[UUID_STR_LEN];

    // The newest manifest SENT for this config, so build_node_manifest() can drop an identical one.
    //
    // "Sent" and not "delivered", deliberately. Both fields are written when the manifest is handed to
    // the ACLK queue, because that is the state a later build must compare against: publication is
    // asynchronous, so a record holding only confirmed publications lags reality for as long as a
    // message is in flight, and a build inside that window compares against content the in-flight
    // message is about to replace. A dropped message invalidates the record instead
    // (aclk_node_manifest_publish_result()), so an unsent manifest never keeps suppressing later
    // builds - that was the original defect here.
    //
    // Atomic, because the writers are on different threads. Both fields are STORED only by the scan
    // pass in build_node_manifest() (the alert-push worker, one pass at a time), while the token alone
    // is CLEARED by whichever thread reports a drop - an ACLK query worker for anything the mqtt layer
    // rejected, the alert-push worker for a drop that happens inside the enqueue call, or the ACLK sync
    // event loop for a query it could not park and for the shutdown drain.
    //
    // The split is what keeps the CROSS-THREAD update a single atomic operation: a drop writes only
    // the token, so it never has to update the pair. The pair IS written together - by the scan pass,
    // as two stores - which leaves a window where the key is the new one and the token is still the
    // previous one. Nothing reads the pair in that window: the scan pass is its only reader and is
    // serialized against itself, and the drop path reads neither field, it only CASes the token.
    //
    // The token identifies an ENQUEUE, which the key cannot - identical content yields an identical
    // key - so invalidating by token cannot clear a record a later manifest has taken over, whatever
    // its content. A 0 token means nothing is outstanding, which is what callocz() leaves and what a
    // drop restores, so the first manifest of a config always sends.
    //
    // The key folds the ACLK session into the content hash because publishing is never acked, so a
    // new session can no longer be suppressed by what the previous one recorded - which is what a
    // cloud that lost the manifest needs. It only removes the suppression, though; something still
    // has to arm the rebuild. See aclk_arm_node_manifest_all_hosts().
    uint64_t node_manifest_sent_key;   // manifest_publication_key() of the newest manifest enqueued
    uint64_t node_manifest_sent_token; // its per-enqueue token; 0 when nothing is outstanding
} aclk_sync_cfg_t;

static inline void aclk_node_id_copy(aclk_sync_cfg_t *aclk_host_config, char dst[UUID_STR_LEN])
{
    spinlock_lock(&aclk_host_config->node_id_spinlock);
    memcpy(dst, aclk_host_config->node_id, UUID_STR_LEN);
    spinlock_unlock(&aclk_host_config->node_id_spinlock);
    dst[UUID_STR_LEN - 1] = '\0';
}

// Returns whether the stored string actually changed. That is the id the manifest is transmitted
// and keyed under at the cloud, so it - not host->node_id - is what tells a caller whether an
// already-pending manifest has been invalidated.
static inline bool aclk_node_id_set(aclk_sync_cfg_t *aclk_host_config, const nd_uuid_t node_id)
{
    char node_id_str[UUID_STR_LEN];
    uuid_unparse_lower(node_id, node_id_str);

    spinlock_lock(&aclk_host_config->node_id_spinlock);
    bool changed = memcmp(aclk_host_config->node_id, node_id_str, UUID_STR_LEN) != 0;
    if (changed)
        memcpy(aclk_host_config->node_id, node_id_str, UUID_STR_LEN);
    spinlock_unlock(&aclk_host_config->node_id_spinlock);

    return changed;
}

static inline time_t aclk_send_timestamp_get(const aclk_send_timestamp_t *send_time)
{
    return __atomic_load_n(send_time, __ATOMIC_ACQUIRE);
}

static inline void aclk_send_timestamp_set(aclk_send_timestamp_t *send_time, time_t value)
{
    __atomic_store_n(send_time, value, __ATOMIC_RELEASE);
}

// Arms a send without ever pushing an already-armed deadline further out.
// Used by the node manifest, whose triggers (one per function registration or
// removal) are frequent enough that re-arming on every change could keep the
// coalescing window from ever elapsing.
//
// A stored value LATER than the one being armed is replaced, so a backward wall-clock step (a bad
// RTC corrected by the first NTP sync, a restored VM snapshot) cannot strand the request until the
// clock catches up. Unlike aclk_send_timestamp_set(), which self-heals on the next publication,
// arm-only-if-zero would keep an unreachable deadline forever.
static inline void aclk_send_timestamp_arm(aclk_send_timestamp_t *send_time, time_t value)
{
    time_t expected = __atomic_load_n(send_time, __ATOMIC_ACQUIRE);

    while (expected == 0 || expected > value) {
        // failure order must not be stronger than success order, and the CAS writes the observed
        // value back into `expected` regardless of it, so RELAXED is sufficient on failure -
        // matching aclk_send_timestamp_claim() below
        if (__atomic_compare_exchange_n(
                send_time, &expected, value, false, __ATOMIC_RELEASE, __ATOMIC_RELAXED))
            return;
        // expected now holds the current value; re-test and retry
    }
}

static inline bool aclk_send_timestamp_claim(aclk_send_timestamp_t *send_time, time_t expected)
{
    // A same-value publication before this claim is covered by the send that follows;
    // one after the claim remains pending instead of being cleared.
    return __atomic_compare_exchange_n(
        send_time, &expected, 0, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED);
}

static inline enum aclk_alert_snapshot_state aclk_alert_snapshot_state_get(aclk_sync_cfg_t *aclk_host_config)
{
    return (enum aclk_alert_snapshot_state)__atomic_load_n(&aclk_host_config->send_snapshot, __ATOMIC_ACQUIRE);
}

static inline void aclk_alert_snapshot_request(aclk_sync_cfg_t *aclk_host_config)
{
    // A request arriving while READY is covered by the full snapshot already scheduled from the frozen health state.
    int8_t expected = ACLK_ALERT_SNAPSHOT_IDLE;
    (void)__atomic_compare_exchange_n(
        &aclk_host_config->send_snapshot,
        &expected,
        ACLK_ALERT_SNAPSHOT_PENDING,
        false,
        __ATOMIC_RELAXED,
        __ATOMIC_RELAXED);
}

static inline bool aclk_alert_snapshot_mark_ready(aclk_sync_cfg_t *aclk_host_config)
{
    int8_t expected = __atomic_load_n(&aclk_host_config->send_snapshot, __ATOMIC_RELAXED);
    if (expected != ACLK_ALERT_SNAPSHOT_PENDING)
        return false;

    return __atomic_compare_exchange_n(
        &aclk_host_config->send_snapshot,
        &expected,
        ACLK_ALERT_SNAPSHOT_READY,
        false,
        __ATOMIC_RELEASE,
        __ATOMIC_RELAXED);
}

static inline void aclk_alert_snapshot_complete(aclk_sync_cfg_t *aclk_host_config)
{
    __atomic_store_n(&aclk_host_config->send_snapshot, ACLK_ALERT_SNAPSHOT_IDLE, __ATOMIC_RELEASE);
}

static inline bool aclk_alert_streaming_enabled(aclk_sync_cfg_t *aclk_host_config)
{
    return __atomic_load_n(&aclk_host_config->stream_alerts, __ATOMIC_ACQUIRE);
}

static inline void aclk_alert_streaming_set(aclk_sync_cfg_t *aclk_host_config, bool enabled)
{
    __atomic_store_n(&aclk_host_config->stream_alerts, enabled, __ATOMIC_RELEASE);
}

void create_aclk_config(RRDHOST *host, nd_uuid_t *host_uuid, nd_uuid_t *node_id);
void destroy_aclk_config(RRDHOST *host);

// true when the caller runs on the ACLK sync event loop, which must never wait for a lock a host
// teardown may hold while blocked on that loop - see the definition
bool aclk_sync_on_event_loop_thread(void);
void aclk_synchronization_init(void);
void aclk_synchronization_shutdown(void);
void aclk_push_alert_config(const char *node_id, const char *config_hash);
void schedule_node_state_update(RRDHOST *host, uint64_t delay);
void unregister_node(const char *machine_guid);
void aclk_queue_node_info(RRDHOST *host, bool immediate);
void aclk_arm_node_manifest(RRDHOST *host);
void aclk_arm_node_manifest_all_hosts(void);

#endif //NETDATA_SQLITE_ACLK_H
