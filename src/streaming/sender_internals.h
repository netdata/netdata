// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SENDER_INTERNALS_H
#define NETDATA_SENDER_INTERNALS_H

#include "rrdpush.h"
#include "common.h"
#include "aclk/https_client.h"

#define WORKER_SENDER_JOB_CONNECT                                0
#define WORKER_SENDER_JOB_PIPE_READ                              1
#define WORKER_SENDER_JOB_SOCKET_RECEIVE                         2
#define WORKER_SENDER_JOB_EXECUTE                                3
#define WORKER_SENDER_JOB_SOCKET_SEND                            4
#define WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE               5
#define WORKER_SENDER_JOB_DISCONNECT_OVERFLOW                    6
#define WORKER_SENDER_JOB_DISCONNECT_TIMEOUT                     7
#define WORKER_SENDER_JOB_DISCONNECT_POLL_ERROR                  8
#define WORKER_SENDER_JOB_DISCONNECT_SOCKET_ERROR                9
#define WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR                  10
#define WORKER_SENDER_JOB_DISCONNECT_PARENT_CLOSED              11
#define WORKER_SENDER_JOB_DISCONNECT_RECEIVE_ERROR              12
#define WORKER_SENDER_JOB_DISCONNECT_SEND_ERROR                 13
#define WORKER_SENDER_JOB_DISCONNECT_NO_COMPRESSION             14
#define WORKER_SENDER_JOB_BUFFER_RATIO                          15
#define WORKER_SENDER_JOB_BYTES_RECEIVED                        16
#define WORKER_SENDER_JOB_BYTES_SENT                            17
#define WORKER_SENDER_JOB_BYTES_COMPRESSED                      18
#define WORKER_SENDER_JOB_BYTES_UNCOMPRESSED                    19
#define WORKER_SENDER_JOB_BYTES_COMPRESSION_RATIO               20
#define WORKER_SENDER_JOB_REPLAY_REQUEST                        21
#define WORKER_SENDER_JOB_FUNCTION_REQUEST                      22
#define WORKER_SENDER_JOB_REPLAY_DICT_SIZE                      23
#define WORKER_SENDER_JOB_DISCONNECT_CANT_UPGRADE_CONNECTION    24

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 25
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 25
#endif

extern struct config stream_config;
extern char *netdata_ssl_ca_path;
extern char *netdata_ssl_ca_file;

bool attempt_to_connect(struct sender_state *state);
void rrdpush_sender_on_connect(RRDHOST *host);
void rrdpush_sender_after_connect(RRDHOST *host);
void rrdpush_sender_thread_close_socket(RRDHOST *host);

void rrdpush_sender_execute_commands_cleanup(struct sender_state *s);
void rrdpush_sender_execute_commands(struct sender_state *s);

#endif //NETDATA_SENDER_INTERNALS_H
