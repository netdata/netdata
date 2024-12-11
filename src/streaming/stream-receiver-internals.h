// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_RECEIVER_INTERNALS_H
#define NETDATA_STREAM_RECEIVER_INTERNALS_H

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
    char *timezone;         // Unused?
    char *abbrev_timezone;
    char *client_ip;        // Duplicated in pluginsd
    char *client_port;        // Duplicated in pluginsd
    char *program_name;        // Duplicated in pluginsd
    char *program_version;
    struct rrdhost_system_info *system_info;
    time_t last_msg_t;
    time_t connected_since_s;

    struct buffered_reader reader;

    struct {
        // The parser pointer is safe to read and use, only when having the host receiver lock.
        // Without this lock, the data pointed by the pointer may vanish randomly.
        // Also, since the receiver sets it when it starts, it should be read with
        // an atomic read.
        struct parser *parser;
        struct plugind cd;
        BUFFER *buffer;

        struct {
            bool enabled;
            size_t start;
            size_t used;
            size_t size;
            char *buf;
            struct decompressor_state decompressor;
        } compressed;

        struct {
            SPINLOCK spinlock;
            struct stream_opcode msg;
            uint32_t msg_slot;
            STREAM_CIRCULAR_BUFFER *scb;
        } send_to_child;

        struct pollfd_meta meta;
    } thread;

    struct {
        bool shutdown;      // signal the streaming parser to exit
        STREAM_HANDSHAKE reason;
    } exit;

    struct stream_receiver_config config;

    time_t replication_first_time_t;

#ifdef ENABLE_H2O
    void *h2o_ctx;
#endif
};

#ifdef ENABLE_H2O
#define is_h2o_rrdpush(x) ((x)->h2o_ctx != NULL)
#define unless_h2o_rrdpush(x) if(!is_h2o_rrdpush(x))
#endif

bool rrdhost_set_receiver(RRDHOST *host, struct receiver_state *rpt);
void rrdhost_clear_receiver(struct receiver_state *rpt);
void stream_receiver_log_status(struct receiver_state *rpt, const char *msg, const char *status, ND_LOG_FIELD_PRIORITY priority);

void stream_receiver_free(struct receiver_state *rpt);
bool stream_receiver_signal_to_stop_and_wait(RRDHOST *host, STREAM_HANDSHAKE reason);

ssize_t send_to_child(const char *txt, void *data, STREAM_TRAFFIC_TYPE type);
void stream_receiver_send_opcode(struct receiver_state *rpt, struct stream_opcode msg);
void stream_receiver_handle_op(struct stream_thread *sth, struct receiver_state *rpt, struct stream_opcode *msg);

#endif //NETDATA_STREAM_RECEIVER_INTERNALS_H
