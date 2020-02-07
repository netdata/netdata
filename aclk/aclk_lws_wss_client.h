#ifndef ACLK_LWS_WSS_CLIENT_H
#define ACLK_LWS_WSS_CLIENT_H

#include <libwebsockets.h>

#include "libnetdata/libnetdata.h"

#define ACLK_LWS_WSS_RECONNECT_TIMEOUT 5

// This is as define because ideally the ACLK at high level
// can do mosqitto writes and reads only from one thread
// which is cleaner implementation IMHO
// in such case this mutexes are not necessarry and life
// is simpler
#define ACLK_LWS_MOSQUITTO_IO_CALLS_MULTITHREADED 1

#define ACLK_LWS_WSS_RECV_BUFF_SIZE_BYTES 128*1024

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
};

struct lws_wss_packet_buffer;

struct aclk_lws_wss_engine_instance {
	//target host/port for connection
	const char *host;
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

	struct aclk_lws_wss_engine_callbacks callbacks;

	//flags to be readed by engine user
	int websocket_connection_up;

// currently this is by default disabled
// as decision has been made that reconnection
// will have to be done from top layer
// (after getting the new MQTT auth data)
// for now i keep it here as it is usefull for
// some of my internall testing
#ifdef AUTO_RECONNECT_ON_LWS_LAYER
	int reconnect_timeout_running;
#endif
	int data_to_read;
	int upstream_reconnect_request;
};

struct aclk_lws_wss_engine_instance* aclk_lws_wss_client_init (const struct aclk_lws_wss_engine_callbacks *callbacks, const char *target_hostname, int target_port);
void aclk_lws_wss_client_destroy(struct aclk_lws_wss_engine_instance* inst);

int aclk_lws_wss_client_write(struct aclk_lws_wss_engine_instance *inst, void *buf, size_t count);
int aclk_lws_wss_client_read (struct aclk_lws_wss_engine_instance *inst, void *buf, size_t count);
int aclk_lws_wss_service_loop(struct aclk_lws_wss_engine_instance *inst);

void aclk_lws_wss_mqtt_layer_disconect_notif(struct aclk_lws_wss_engine_instance *inst);

#endif