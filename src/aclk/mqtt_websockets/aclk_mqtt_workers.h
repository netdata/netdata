// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ACLK_MQTT_WORKERS_H
#define NETDATA_ACLK_MQTT_WORKERS_H

#define WORKER_ACLK_WAIT_CLAIMING               0
#define WORKER_ACLK_CONNECT                     1
#define WORKER_ACLK_NODE_UPDATE                 2
#define WORKER_ACLK_HANDLE_CONNECTION           3
#define WORKER_ACLK_DISCONNECTED                4
#define WORKER_ACLK_CMD_DISCONNECT              5
#define WORKER_ACLK_CMD_TIMEOUT                 6
#define WORKER_ACLK_CMD_RELOAD_CONF             7
#define WORKER_ACLK_CMD_UNKNOWN                 8
#define WORKER_ACLK_SENT_PING                   9
#define WORKER_ACLK_POLL_ERROR                  10
#define WORKER_ACLK_POLL_OK                     11
#define WORKER_ACLK_RX                          12
#define WORKER_ACLK_RX_ERROR                    13
#define WORKER_ACLK_PROCESS_RAW                 14
#define WORKER_ACLK_PROCESS_HANDSHAKE           15
#define WORKER_ACLK_PROCESS_ESTABLISHED         16
#define WORKER_ACLK_PROCESS_ERROR               17
#define WORKER_ACLK_PROCESS_CLOSED_GRACEFULLY   18
#define WORKER_ACLK_PROCESS_UNKNOWN             19
#define WORKER_ACLK_HANDLE_MQTT_INTERNAL        20
#define WORKER_ACLK_TX                          21
#define WORKER_ACLK_TX_ERROR                    22
#define WORKER_ACLK_TRY_SEND_ALL                23
#define WORKER_ACLK_HANDLE_INCOMING             24
#define WORKER_ACLK_CPT_CONNACK                 25
#define WORKER_ACLK_CPT_PUBACK                  26
#define WORKER_ACLK_CPT_PINGRESP                27
#define WORKER_ACLK_CPT_SUBACK                  28
#define WORKER_ACLK_CPT_PUBLISH                 29
#define WORKER_ACLK_CPT_DISCONNECT              30
#define WORKER_ACLK_CPT_UNKNOWN                 31
#define WORKER_ACLK_SEND_FRAGMENT               32
#define WORKER_ACLK_MSG_CALLBACK                33
#define WORKER_ACLK_WAITING_TO_CONNECT          34
#define WORKER_ACLK_RECLAIM_MEMORY              35
#define WORKER_ACLK_BUFFER_COMPACT              36

#endif //NETDATA_ACLK_MQTT_WORKERS_H
