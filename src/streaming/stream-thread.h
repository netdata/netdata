// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_THREAD_H
#define NETDATA_STREAM_THREAD_H

#include "libnetdata/libnetdata.h"
#include "stream-circular-buffer.h"

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
    struct pollfd_meta *meta;
};

// IMPORTANT: to add workers, you have to edit WORKER_PARSER_FIRST_JOB accordingly

// stream thread events
#define WORKER_STREAM_JOB_LIST                                          (WORKER_PARSER_FIRST_JOB - 34)
#define WORKER_STREAM_JOB_DEQUEUE                                       (WORKER_PARSER_FIRST_JOB - 33)
#define WORKER_STREAM_JOB_PREP                                          (WORKER_PARSER_FIRST_JOB - 32)
#define WORKER_STREAM_JOB_POLL_ERROR                                    (WORKER_PARSER_FIRST_JOB - 31)
#define WORKER_SENDER_JOB_PIPE_READ                                     (WORKER_PARSER_FIRST_JOB - 30)

// socket operations
#define WORKER_STREAM_JOB_SOCKET_RECEIVE                                (WORKER_PARSER_FIRST_JOB - 29)
#define WORKER_STREAM_JOB_SOCKET_SEND                                   (WORKER_PARSER_FIRST_JOB - 28)
#define WORKER_STREAM_JOB_SOCKET_ERROR                                  (WORKER_PARSER_FIRST_JOB - 27)

// compression
#define WORKER_STREAM_JOB_COMPRESS                                      (WORKER_PARSER_FIRST_JOB - 26)
#define WORKER_STREAM_JOB_DECOMPRESS                                    (WORKER_PARSER_FIRST_JOB - 25)

// receiver events
#define WORKER_RECEIVER_JOB_BYTES_READ                                  (WORKER_PARSER_FIRST_JOB - 24)
#define WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED                          (WORKER_PARSER_FIRST_JOB - 23)

// sender received commands
#define WORKER_SENDER_JOB_EXECUTE                                       (WORKER_PARSER_FIRST_JOB - 22)
#define WORKER_SENDER_JOB_EXECUTE_REPLAY                                (WORKER_PARSER_FIRST_JOB - 21)
#define WORKER_SENDER_JOB_EXECUTE_FUNCTION                              (WORKER_PARSER_FIRST_JOB - 20)
#define WORKER_SENDER_JOB_EXECUTE_META                                  (WORKER_PARSER_FIRST_JOB - 19)

#define WORKER_SENDER_JOB_DISCONNECT_OVERFLOW                           (WORKER_PARSER_FIRST_JOB - 18)
#define WORKER_SENDER_JOB_DISCONNECT_TIMEOUT                            (WORKER_PARSER_FIRST_JOB - 17)
#define WORKER_SENDER_JOB_DISCONNECT_SOCKET_ERROR                       (WORKER_PARSER_FIRST_JOB - 16)
#define WORKER_SENDER_JOB_DISCONNECT_REMOTE_CLOSED                      (WORKER_PARSER_FIRST_JOB - 15)
#define WORKER_SENDER_JOB_DISCONNECT_RECEIVE_ERROR                      (WORKER_PARSER_FIRST_JOB - 14)
#define WORKER_SENDER_JOB_DISCONNECT_SEND_ERROR                         (WORKER_PARSER_FIRST_JOB - 13)
#define WORKER_SENDER_JOB_DISCONNECT_COMPRESSION_ERROR                  (WORKER_PARSER_FIRST_JOB - 12)
#define WORKER_SENDER_JOB_DISCONNECT_RECEIVER_LEFT                      (WORKER_PARSER_FIRST_JOB - 11)
#define WORKER_SENDER_JOB_DISCONNECT_HOST_CLEANUP                       (WORKER_PARSER_FIRST_JOB - 10)

// dispatcher metrics
// this has to be the same at pluginsd_parser.h
#define WORKER_RECEIVER_JOB_REPLICATION_COMPLETION                      (WORKER_PARSER_FIRST_JOB - 9)
#define WORKER_STREAM_METRIC_NODES                                      (WORKER_PARSER_FIRST_JOB - 8)
#define WORKER_SENDER_JOB_BUFFER_RATIO                                  (WORKER_PARSER_FIRST_JOB - 7)
#define WORKER_SENDER_JOB_BYTES_RECEIVED                                (WORKER_PARSER_FIRST_JOB - 6)
#define WORKER_SENDER_JOB_BYTES_SENT                                    (WORKER_PARSER_FIRST_JOB - 5)
#define WORKER_SENDER_JOB_BYTES_COMPRESSED                              (WORKER_PARSER_FIRST_JOB - 4)
#define WORKER_SENDER_JOB_BYTES_UNCOMPRESSED                            (WORKER_PARSER_FIRST_JOB - 3)
#define WORKER_SENDER_JOB_BYTES_COMPRESSION_RATIO                       (WORKER_PARSER_FIRST_JOB - 2)
#define WORKER_SENDER_JOB_REPLAY_DICT_SIZE                              (WORKER_PARSER_FIRST_JOB - 1)
#define WORKER_SENDER_JOB_MESSAGES                                      (WORKER_PARSER_FIRST_JOB - 0)

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 35
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 34
#endif

#define STREAM_MAX_THREADS 2048
#define THREAD_TAG_STREAM "STREAM"

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
        SENDERS_JudyLSet senders;
        RECEIVERS_JudyLSet receivers;
    } queue;

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
void stream_receiver_move_queue_to_running_unsafe(struct stream_thread *sth);
void stream_sender_check_all_nodes_from_poll(struct stream_thread *sth, usec_t now_ut);

void stream_receiver_add_to_queue(struct receiver_state *rpt);
void stream_sender_add_to_connector_queue(struct rrdhost *host);

void stream_sender_process_poll_events(struct stream_thread *sth, struct sender_state *s, nd_poll_event_t events, usec_t now_ut);
void stream_receive_process_poll_events(struct stream_thread *sth, struct receiver_state *rpt, nd_poll_event_t events, usec_t now_ut);

void stream_sender_cleanup(struct stream_thread *sth);
void stream_receiver_cleanup(struct stream_thread *sth);
void stream_sender_handle_op(struct stream_thread *sth, struct sender_state *s, struct stream_opcode *msg);

struct stream_thread *stream_thread_by_slot_id(size_t thread_slot);

void stream_thread_node_queued(struct rrdhost *host);
void stream_thread_node_removed(struct rrdhost *host);

#include "stream-sender-internals.h"
#include "stream-receiver-internals.h"
#include "plugins.d/pluginsd_parser.h"

static inline bool rrdhost_is_this_a_stream_thread(RRDHOST *host) {
    pid_t tid = gettid_cached();
    return host->stream.rcv.status.tid == tid || host->stream.snd.status.tid == tid;
}

#endif //NETDATA_STREAM_THREAD_H
