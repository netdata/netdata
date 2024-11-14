// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SENDER_INTERNALS_H
#define NETDATA_SENDER_INTERNALS_H

#include "rrdpush.h"
#include "h2o-common.h"
#include "aclk/https_client.h"
#include "stream-parents.h"

// connector thread
#define WORKER_SENDER_CONNECTOR_JOB_CONNECTING                          0
#define WORKER_SENDER_CONNECTOR_JOB_CONNECTED                           1
#define WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_BAD_HANDSHAKE            2
#define WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_TIMEOUT                  3
#define WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_CANT_UPGRADE_CONNECTION  4
#define WORKER_SENDER_CONNECTOR_JOB_QUEUED_NODES                        5
#define WORKER_SENDER_CONNECTOR_JOB_CONNECTED_NODES                     6
#define WORKER_SENDER_CONNECTOR_JOB_FAILED_NODES                        7
#define WORKER_SENDER_CONNECTOR_JOB_CANCELLED_NODES                     8

// dispatcher thread
#define WORKER_SENDER_DISPATCHER_JOB_LIST                               0
#define WORKER_SENDER_DISPATCHER_JOB_DEQUEUE                            1
#define WORKER_SENDER_DISPATCHER_JOB_POLL_ERROR                         2
#define WORKER_SENDER_DISPATCHER_JOB_PIPE_READ                          3
#define WORKER_SENDER_DISPATCHER_JOB_SOCKET_RECEIVE                     4
#define WORKER_SENDER_DISPATCHER_JOB_SOCKET_SEND                        5
#define WORKER_SENDER_DISPATCHER_JOB_EXECUTE                            6
#define WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_OVERFLOW                7
#define WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_TIMEOUT                 8
#define WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_SOCKET_ERROR            9
#define WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_PARENT_CLOSED           10
#define WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_RECEIVE_ERROR           11
#define WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_SEND_ERROR              12
#define WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_STOPPED                 13

// dispatcher execute requests
#define WORKER_SENDER_DISPATCHER_JOB_REPLAY_REQUEST                     14
#define WORKER_SENDER_DISPATCHER_JOB_FUNCTION_REQUEST                   15

// dispatcher metrics
#define WORKER_SENDER_DISPATCHER_JOB_NODES                              16
#define WORKER_SENDER_DISPATCHER_JOB_BUFFER_RATIO                       17
#define WORKER_SENDER_DISPATCHER_JOB_BYTES_RECEIVED                     18
#define WORKER_SENDER_DISPATCHER_JOB_BYTES_SENT                         19
#define WORKER_SENDER_DISPATCHER_JOB_BYTES_COMPRESSED                   20
#define WORKER_SENDER_DISPATCHER_JOB_BYTES_UNCOMPRESSED                 21
#define WORKER_SENDER_DISPATCHER_JOB_BYTES_COMPRESSION_RATIO            22
#define WORKER_SENDER_DISPATHCER_JOB_REPLAY_DICT_SIZE                   23

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 24
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 25
#endif

void stream_sender_start_host_routing(RRDHOST *host);

void rrdpush_sender_on_connect(RRDHOST *host);
void rrdpush_sender_thread_close_socket(struct sender_state *s);

void rrdpush_sender_execute_commands_cleanup(struct sender_state *s);
void rrdpush_sender_execute_commands(struct sender_state *s);

bool stream_sender_connect_to_parent(struct sender_state *s);
void stream_sender_dispatcher_wake_up(struct sender_state *s);

void stream_sender_cbuffer_recreate_timed(struct sender_state *s, time_t now_s, bool have_mutex, bool force);

bool stream_sender_is_host_stopped(struct sender_state *s);

uint64_t stream_sender_magic(RRDHOST *host);
pid_t stream_sender_tid(RRDHOST *host);

#endif //NETDATA_SENDER_INTERNALS_H
