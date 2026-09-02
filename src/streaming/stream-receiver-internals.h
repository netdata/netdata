// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_RECEIVER_INTERNALS_H
#define NETDATA_STREAM_RECEIVER_INTERNALS_H

#include "libnetdata/libnetdata.h"

#ifdef NETDATA_LOG_STREAM_RECEIVER
#include "stream-traffic-types.h"
struct receiver_state;
void stream_receiver_log_payload(struct receiver_state *rpt, const char *payload, STREAM_TRAFFIC_TYPE type, bool inbound);
#else
#define stream_receiver_log_payload(s, payload, type, inbound) debug_dummy()
#endif

#include "stream.h"
#include "stream-thread.h"
#include "stream-conf.h"
#include "database/rrd.h"
#include "plugins.d/plugins_d.h"

#define STREAM_RECEIVER_METADATA_MAX_LENGTH RRD_ID_LENGTH_MAX

static inline bool stream_receiver_parse_hops(const char *value, int16_t *hops) {
    if(!value || !hops)
        return false;

    char *endptr;
    errno = 0;
    long parsed = strtol(value, &endptr, 0);

    if(errno != 0 || endptr == value || *endptr != '\0' || parsed < 1 || parsed > INT16_MAX)
        return false;

    *hops = (int16_t)parsed;
    return true;
}

static inline bool stream_receiver_metadata_size_is_valid(const char *value) {
    return value && strnlen(value, STREAM_RECEIVER_METADATA_MAX_LENGTH + 1) <= STREAM_RECEIVER_METADATA_MAX_LENGTH;
}

static inline bool stream_receiver_metadata_should_reject(
    bool recognized, size_t name_length, size_t value_length)
{
    return recognized &&
           (name_length > STREAM_RECEIVER_METADATA_MAX_LENGTH ||
            value_length > STREAM_RECEIVER_METADATA_MAX_LENGTH);
}

struct stream_receiver_unused_metadata {
    size_t fields;
    size_t name_bytes;
    size_t value_bytes;
};

static inline void stream_receiver_unused_metadata_add(
    struct stream_receiver_unused_metadata *unused, size_t name_length, size_t value_length)
{
    unused->fields++;
    unused->name_bytes += name_length;
    unused->value_bytes += value_length;
}

static inline void stream_receiver_parse_user_agent(
    const char *user_agent, char **program_name, char **program_version)
{
    *program_name = NULL;
    *program_version = NULL;

    if(!user_agent || !*user_agent)
        return;

    const char *separator = strchr(user_agent, '/');
    size_t name_length = separator ? (size_t)(separator - user_agent) : strlen(user_agent);
    *program_name = strndupz(user_agent, MIN(name_length, (size_t)STREAM_RECEIVER_METADATA_MAX_LENGTH));

    if(separator && separator[1])
        *program_version = strndupz(separator + 1, STREAM_RECEIVER_METADATA_MAX_LENGTH);
}

struct parser;

struct receiver_state {
    RRDHOST *host;
    ND_SOCK sock;
    int16_t hops;
    int32_t utc_offset;
    STREAM_CAPABILITIES capabilities;
    char *key;
    char *hostname;
    char *registry_hostname;
    char *machine_guid;
    char *os;
    char *timezone;             // Unused?
    char *abbrev_timezone;
    char *remote_ip;            // Duplicated in pluginsd
    char *remote_port;          // Duplicated in pluginsd
    char *program_name;         // Duplicated in pluginsd
    char *program_version;
    struct rrdhost_system_info *system_info;
    time_t connected_since_s;

    struct {
        // The parser pointer is safe to read and use, only when having the host receiver lock.
        // Without this lock, the data pointed by the pointer may vanish randomly.
        // Also, since the receiver sets it when it starts, it should be read with
        // an atomic read.
        struct parser *parser;
        struct plugind cd;

        // compressed data input
        struct {
            bool enabled;
            size_t start;
            size_t used;
            size_t size;
            char *buf;
            struct decompressor_state decompressor;
        } compressed;

        // uncompressed data input (either directly or via the decompressor)
        struct buffered_reader uncompressed;

        // a single line of input (composed via uncompressed buffer input)
        BUFFER *line_buffer;

        struct {
            SPINLOCK spinlock;
            struct stream_opcode msg;
            uint32_t msg_slot;
            STREAM_CIRCULAR_BUFFER *scb;
        } send_to_child;

        nd_poll_event_t wanted;
        usec_t last_traffic_ut;
        size_t bytes_received;          // raw socket bytes received on this connection (diagnostics)
        bool keepalive_initialized;
        struct pollfd_meta meta;
    } thread;

    struct {
        uint32_t last_counter_sum;  // copy from the host, to detect progress
        usec_t last_progress_ut;    // last time we found some progress (monotonic)
        usec_t last_checked_ut;     // last time we checked for stalled progress (monotonic)

        time_t first_time_s;
    } replication;

    struct {
        bool shutdown;      // signal the streaming parser to exit
        STREAM_HANDSHAKE reason;
    } exit;

    struct stream_receiver_config config;
    int handshake_update_every;

#ifdef NETDATA_LOG_STREAM_RECEIVER
    struct {
        struct timespec first_call;
        SPINLOCK spinlock;
        FILE *fp;
    } log;
#endif
};

typedef enum {
    RRDHOST_SET_RECEIVER_OK,                // attached
    RRDHOST_SET_RECEIVER_ALREADY_ATTACHED,  // another receiver already attached
    RRDHOST_SET_RECEIVER_CLEANUP_BUSY,      // obsolete-all cleanup is running; caller should answer BUSY_TRY_LATER
    RRDHOST_SET_RECEIVER_VNODE_IS_LOCAL,    // the host is collected locally as a vnode; caller should answer LOCAL_VNODE
} RRDHOST_SET_RECEIVER_RESULT;

RRDHOST_SET_RECEIVER_RESULT rrdhost_set_receiver(RRDHOST *host, struct receiver_state *rpt);
void rrdhost_clear_receiver(struct receiver_state *rpt, STREAM_HANDSHAKE reason);
void stream_receiver_log_status(struct receiver_state *rpt, const char *msg, STREAM_HANDSHAKE reason, ND_LOG_FIELD_PRIORITY priority);

void stream_receiver_free(struct receiver_state *rpt);
bool stream_receiver_signal_to_stop_and_wait(RRDHOST *host, STREAM_HANDSHAKE reason);
void stream_receiver_reconcile_keepalive(struct receiver_state *rpt);

void stream_receiver_send_opcode(struct receiver_state *rpt, struct stream_opcode msg);
void stream_receiver_handle_op(struct stream_thread *sth, struct receiver_state *rpt, struct stream_opcode *msg);

void stream_receiver_check_all_nodes_from_poll(struct stream_thread *sth, usec_t now_ut);
void stream_receiver_replication_check_from_poll(struct stream_thread *sth, usec_t now_ut);

#endif //NETDATA_STREAM_RECEIVER_INTERNALS_H
