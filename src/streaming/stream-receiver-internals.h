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

#ifdef NETDATA_LOG_STREAM_RECEIVER
    struct {
        struct timespec first_call;
        SPINLOCK spinlock;
        FILE *fp;
    } log;
#endif
};

bool rrdhost_set_receiver(RRDHOST *host, struct receiver_state *rpt);
void rrdhost_clear_receiver(struct receiver_state *rpt, STREAM_HANDSHAKE reason);
void stream_receiver_log_status(struct receiver_state *rpt, const char *msg, STREAM_HANDSHAKE reason, ND_LOG_FIELD_PRIORITY priority);

void stream_receiver_free(struct receiver_state *rpt);
bool stream_receiver_signal_to_stop_and_wait(RRDHOST *host, STREAM_HANDSHAKE reason);

void stream_receiver_send_opcode(struct receiver_state *rpt, struct stream_opcode msg);
void stream_receiver_handle_op(struct stream_thread *sth, struct receiver_state *rpt, struct stream_opcode *msg);

void stream_receiver_check_all_nodes_from_poll(struct stream_thread *sth, usec_t now_ut);
void stream_receiver_replication_check_from_poll(struct stream_thread *sth, usec_t now_ut);

#endif //NETDATA_STREAM_RECEIVER_INTERNALS_H
