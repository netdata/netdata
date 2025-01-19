// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_THREAD_H
#define NETDATA_STREAM_THREAD_H

#include "libnetdata/libnetdata.h"
#include "stream-circular-buffer.h"
#include "stream-handshake.h"

struct stream_thread;
struct pollfd_slotted {
    struct stream_thread *sth;
    int32_t slot;
    int fd;
};

#define PFD_EMPTY (struct pollfd_slotted){ .sth = NULL, .fd = -1, .slot = -1, }

typedef enum __attribute__((packed)) {
    STREAM_OPCODE_NONE                                  = 0,
    STREAM_OPCODE_SENDER_POLLOUT                        = (1 << 0), // move traffic around as soon as possible
    STREAM_OPCODE_RECEIVER_POLLOUT                      = (1 << 1), // disconnect the node, it has buffer overflow
    STREAM_OPCODE_SENDER_BUFFER_OVERFLOW                = (1 << 2), // reconnect the node, it has buffer overflow
    STREAM_OPCODE_RECEIVER_BUFFER_OVERFLOW              = (1 << 3), // reconnect the node, it has buffer overflow
    STREAM_OPCODE_SENDER_RECONNECT_WITHOUT_COMPRESSION  = (1 << 4), // reconnect the node, but disable compression
    STREAM_OPCODE_SENDER_STOP_RECEIVER_LEFT             = (1 << 5), // disconnect the node, the receiver left
    STREAM_OPCODE_SENDER_STOP_HOST_CLEANUP              = (1 << 6), // disconnect the node, it is being de-allocated
} STREAM_OPCODE;

struct stream_opcode {
    int32_t thread_slot;                // the dispatcher id this message refers to
    uint32_t session;                   // random number used to verify that the message the dispatcher receives is for this sender
    STREAM_OPCODE opcode;               // the actual message to be delivered
    STREAM_HANDSHAKE reason;
    struct pollfd_meta *meta;
};

// IMPORTANT: to add workers, you have to edit WORKER_PARSER_FIRST_JOB accordingly

// stream thread events
#define WORKER_STREAM_JOB_LIST                                          0
#define WORKER_STREAM_JOB_DEQUEUE                                       1
#define WORKER_STREAM_JOB_PREP                                          2
#define WORKER_STREAM_JOB_POLL_ERROR                                    3
#define WORKER_SENDER_JOB_PIPE_READ                                     4

// socket operations
#define WORKER_STREAM_JOB_SOCKET_RECEIVE                                5
#define WORKER_STREAM_JOB_SOCKET_SEND                                   6

// compression
#define WORKER_STREAM_JOB_COMPRESS                                      7
#define WORKER_STREAM_JOB_DECOMPRESS                                    8

// receiver events
#define WORKER_RECEIVER_JOB_BYTES_READ                                  9
#define WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED                          10

// sender received commands
#define WORKER_SENDER_JOB_EXECUTE                                       11
#define WORKER_SENDER_JOB_EXECUTE_REPLAY                                12
#define WORKER_SENDER_JOB_EXECUTE_FUNCTION                              13
#define WORKER_SENDER_JOB_EXECUTE_META                                  14

// disconnect reasons
#define WORKER_STREAM_JOB_DISCONNECT_REMOTE_CLOSED                      15
#define WORKER_STREAM_JOB_DISCONNECT_RECEIVE_ERROR                      16
#define WORKER_STREAM_JOB_DISCONNECT_SEND_ERROR                         17
#define WORKER_STREAM_JOB_DISCONNECT_TIMEOUT                            18
#define WORKER_STREAM_JOB_DISCONNECT_SOCKET_ERROR                       19

// sender-only disconnect reasons
#define WORKER_SENDER_JOB_DISCONNECT_OVERFLOW                           20
#define WORKER_SENDER_JOB_DISCONNECT_COMPRESSION_ERROR                  21
#define WORKER_SENDER_JOB_DISCONNECT_RECEIVER_LEFT                      22
#define WORKER_SENDER_JOB_DISCONNECT_HOST_CLEANUP                       23

// dispatcher metrics
// this has to be the same at pluginsd_parser.h
#define WORKER_RECEIVER_JOB_REPLICATION_COMPLETION                      24
#define WORKER_STREAM_METRIC_NODES                                      25
#define WORKER_SENDER_JOB_BUFFER_RATIO                                  26
#define WORKER_SENDER_JOB_BYTES_RECEIVED                                27
#define WORKER_SENDER_JOB_BYTES_SENT                                    28
#define WORKER_SENDER_JOB_BYTES_COMPRESSED                              29
#define WORKER_SENDER_JOB_BYTES_UNCOMPRESSED                            30
#define WORKER_SENDER_JOB_BYTES_COMPRESSION_RATIO                       31
#define WORKER_SENDER_JOB_REPLAY_DICT_SIZE                              32
#define WORKER_SENDER_JOB_MESSAGES                                      33
#define WORKER_STREAM_JOB_RECEIVERS_WAITING_LIST_SIZE                   34
#define WORKER_STREAM_JOB_SEND_MISSES                                   35

// IMPORTANT: to add workers, you have to edit WORKER_PARSER_FIRST_JOB accordingly

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 36
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 34
#endif

#define STREAM_MAX_THREADS 2048
#define THREAD_TAG_STREAM "STREAM"

typedef enum {
    EVLOOP_STATUS_CONTINUE,
    EVLOOP_STATUS_SOCKET_FULL,
    EVLOOP_STATUS_SOCKET_CLOSED,
    EVLOOP_STATUS_SOCKET_ERROR,
    EVLOOP_STATUS_NO_MORE_DATA,
    EVLOOP_STATUS_OPCODE_ON_ME,
    EVLOOP_STATUS_CANT_GET_LOCK,
    EVLOOP_STATUS_PARSER_FAILED,
} EVLOOP_STATUS;

#define EVLOOP_STATUS_STILL_ALIVE(status) (status == EVLOOP_STATUS_CONTINUE || status == EVLOOP_STATUS_NO_MORE_DATA || status == EVLOOP_STATUS_SOCKET_FULL || status == EVLOOP_STATUS_CANT_GET_LOCK)

typedef enum {
    POLLFD_TYPE_EMPTY,
    POLLFD_TYPE_SENDER,
    POLLFD_TYPE_RECEIVER,
    POLLFD_TYPE_PIPE,
} POLLFD_TYPE;

struct pollfd_meta {
    POLLFD_TYPE type;
    union {
        struct receiver_state *rpt;
        struct sender_state *s;
    };
};

DEFINE_JUDYL_TYPED(SENDERS, struct sender_state *);
DEFINE_JUDYL_TYPED(RECEIVERS, struct receiver_state *);
DEFINE_JUDYL_TYPED(META, struct pollfd_meta *);

struct stream_thread {
    ND_THREAD *thread;

    pid_t tid;
    size_t id;
    size_t nodes_count;

    struct {
        size_t bytes_received;
        size_t bytes_sent;
        size_t send_misses;
    } snd;

    struct {
        size_t bytes_received;
        size_t bytes_received_uncompressed;
        NETDATA_DOUBLE replication_completion;
    } rcv;

    struct {
        SPINLOCK spinlock; // ensure a single writer at a time
        int fds[2];
        size_t size;
        char *buffer;
    } pipe;

    struct {
        // the incoming queue of the dispatcher thread
        // the connector thread leaves the connected senders in this list, for the dispatcher to pick them up
        SPINLOCK spinlock;
        Word_t id;
        SENDERS_JudyLSet senders;
        RECEIVERS_JudyLSet receivers;

        size_t receivers_waiting;
    } queue;

    struct {
        usec_t last_accepted_ut;
        size_t metadata;
        size_t replication;
    } waiting_list;

    struct {
        SPINLOCK spinlock;
        size_t added;
        size_t processed;
        size_t bypassed;
        size_t size;
        size_t used;
        struct stream_opcode *array;         // the array of messages from the senders
        struct stream_opcode *copy;          // a copy of the array of messages from the senders, to work on
    } messages;

    struct {
        nd_poll_t *ndpl;
        struct pollfd_meta pipe;
        META_JudyLSet meta;
    } run;
};

struct stream_thread_globals {
    struct {
        SPINLOCK spinlock;
        size_t id;
        size_t cores;
    } assign;

    struct stream_thread threads[STREAM_MAX_THREADS];
};

struct rrdhost;
extern struct stream_thread_globals stream_thread_globals;

void stream_sender_move_queue_to_running_unsafe(struct stream_thread *sth);
void stream_receiver_move_entire_queue_to_running_unsafe(struct stream_thread *sth);
void stream_sender_check_all_nodes_from_poll(struct stream_thread *sth, usec_t now_ut);
void stream_sender_replication_check_from_poll(struct stream_thread *sth, usec_t now_ut);

void stream_receiver_add_to_queue(struct receiver_state *rpt);
void stream_sender_add_to_connector_queue(struct rrdhost *host);

bool stream_sender_process_poll_events(struct stream_thread *sth, struct sender_state *s, nd_poll_event_t events, usec_t now_ut);
bool stream_receive_process_poll_events(struct stream_thread *sth, struct receiver_state *rpt, nd_poll_event_t events, usec_t now_ut);

void stream_sender_cleanup(struct stream_thread *sth);
void stream_receiver_cleanup(struct stream_thread *sth);
void stream_sender_handle_op(struct stream_thread *sth, struct sender_state *s, struct stream_opcode *msg);

struct stream_thread *stream_thread_by_slot_id(size_t thread_slot);

void stream_thread_node_queued(struct rrdhost *host);
void stream_thread_node_removed(struct rrdhost *host);

// returns true if my_meta has received a message
bool stream_thread_process_opcodes(struct stream_thread *sth, struct pollfd_meta *my_meta);

void stream_receiver_move_to_running_unsafe(struct stream_thread *sth, struct receiver_state *rpt);

bool stream_sender_receive_data(struct stream_thread *sth, struct sender_state *s, usec_t now_ut, bool process_opcodes);
bool stream_sender_send_data(struct stream_thread *sth, struct sender_state *s, usec_t now_ut, bool process_opcodes_and_enable_removal);

bool stream_receiver_receive_data(struct stream_thread *sth, struct receiver_state *rpt, usec_t now_ut, bool process_opcodes);
bool stream_receiver_send_data(struct stream_thread *sth, struct receiver_state *rpt, usec_t now_ut, bool process_opcodes_and_enable_removal);

#include "stream-sender-internals.h"
#include "stream-receiver-internals.h"
#include "plugins.d/pluginsd_parser.h"

static inline bool rrdhost_is_this_a_stream_thread(RRDHOST *host) {
    pid_t tid = gettid_cached();
    return host->stream.rcv.status.tid == tid || host->stream.snd.status.tid == tid;
}

#endif //NETDATA_STREAM_THREAD_H
