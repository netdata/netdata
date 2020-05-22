// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDPUSH_H
#define NETDATA_RRDPUSH_H 1

#include "../libnetdata/libnetdata.h"
#include "web/server/web_client.h"
#include "daemon/common.h"

#define CONNECTED_TO_SIZE 100
#define STREAMING_PROTOCOL_CURRENT_VERSION (uint32_t)3

#define STREAMING_PROTOCOL_VERSION "1.1"
#define START_STREAMING_PROMPT "Hit me baby, push them over..."
#define START_STREAMING_PROMPT_V2  "Hit me baby, push them over and bring the host labels..."
#define START_STREAMING_PROMPT_VN "Hit me baby, push them over with the version="

#define HTTP_HEADER_SIZE 8192

#define rrdpush_buffer_lock(host) netdata_mutex_lock(&((host)->rrdpush_sender_buffer_mutex))
#define rrdpush_buffer_unlock(host) netdata_mutex_unlock(&((host)->rrdpush_sender_buffer_mutex))

typedef enum {
    RRDPUSH_MULTIPLE_CONNECTIONS_ALLOW,
    RRDPUSH_MULTIPLE_CONNECTIONS_DENY_NEW
} RRDPUSH_MULTIPLE_CONNECTIONS_STRATEGY;

/*   TODO: Kill this again
typedef enum {
    SENDER_DISCONNECTED,
    SENDER_CONNECTED
} SenderState;
*/

typedef struct {
    char *os_name;
    char *os_id;
    char *os_version;
    char *kernel_name;
    char *kernel_version;
} stream_encoded_t;

extern unsigned int default_rrdpush_enabled;
extern char *default_rrdpush_destination;
extern char *default_rrdpush_api_key;
extern char *default_rrdpush_send_charts_matching;
extern unsigned int remote_clock_resync_iterations;

extern int rrdpush_init();
extern int configured_as_master();
extern void rrdset_done_push(RRDSET *st);
extern void rrdset_push_chart_definition_now(RRDSET *st);
extern void *rrdpush_sender_thread(void *ptr);
extern void rrdpush_send_labels(RRDHOST *host);

extern int rrdpush_receiver_thread_spawn(RRDHOST *host, struct web_client *w, char *url);
extern void rrdpush_sender_thread_stop(RRDHOST *host);

extern void rrdpush_sender_send_this_host_variable_now(RRDHOST *host, RRDVAR *rv);

#endif //NETDATA_RRDPUSH_H
