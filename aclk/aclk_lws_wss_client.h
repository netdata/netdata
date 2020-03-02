#ifndef ACLK_LWS_WSS_CLIENT_H
#define ACLK_LWS_WSS_CLIENT_H

#include <libwebsockets.h>

#include "libnetdata/libnetdata.h"

// This is as define because ideally the ACLK at high level
// can do mosqitto writes and reads only from one thread
// which is cleaner implementation IMHO
// in such case this mutexes are not necessarry and life
// is simpler
#define ACLK_LWS_MOSQUITTO_IO_CALLS_MULTITHREADED 1

#define ACLK_LWS_WSS_RECV_BUFF_SIZE_BYTES (128 * 1024)

#ifdef ACLK_LWS_MOSQUITTO_IO_CALLS_MULTITHREADED
#define aclk_lws_mutex_init(x) netdata_mutex_init(x)
#define aclk_lws_mutex_lock(x) netdata_mutex_lock(x)
#define aclk_lws_mutex_unlock(x) netdata_mutex_unlock(x)
#else
#define aclk_lws_mutex_init(x)
#define aclk_lws_mutex_lock(x)
#define aclk_lws_mutex_unlock(x)
#endif

struct aclk_lws_wss_engine_callbacks {
    void (*connection_established_callback)();
    void (*data_rcvd_callback)();
    void (*data_writable_callback)();
    void (*connection_closed)();
};

struct lws_wss_packet_buffer;

struct aclk_lws_wss_engine_instance {
    //target host/port for connection
    char *host;
    int port;

    //internal data
    struct lws_context *lws_context;
    struct lws *lws_wsi;

#ifdef ACLK_LWS_MOSQUITTO_IO_CALLS_MULTITHREADED
    netdata_mutex_t write_buf_mutex;
    netdata_mutex_t read_buf_mutex;
#endif

    struct lws_wss_packet_buffer *write_buffer_head;
    struct lws_ring *read_ringbuffer;

    //flags to be readed by engine user
    int websocket_connection_up;

    // currently this is by default disabled

    int data_to_read;
    int upstream_reconnect_request;
};

void aclk_lws_wss_client_destroy();

int aclk_lws_wss_connect(char *host, int port);

int aclk_lws_wss_client_write(void *buf, size_t count);
int aclk_lws_wss_client_read(void *buf, size_t count);
void aclk_lws_wss_service_loop();

void aclk_lws_wss_mqtt_layer_disconect_notif();

// Notifications inside the layer above
void aclk_lws_connection_established();
void aclk_lws_connection_data_received();
void aclk_lws_connection_closed();


#endif
