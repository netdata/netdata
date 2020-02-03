#ifndef ACLK_LWS_WSS_CLIENT_H
#define ACLK_LWS_WSS_CLIENT_H

#include <libwebsockets.h>

#define ACLK_LWS_WSS_RECONNECT_TIMEOUT 5

struct aclk_lws_wss_engine_callbacks {
	void (*connection_established_callback)();
	void (*data_rcvd_callback)();
	void (*data_writable_callback)();
};

struct aclk_lws_wss_engine_instance {
	//target host/port for connection
	const char *host;
	int port;

	//internal data
	struct lws_context *lws_context;
	struct lws *lws_wsi;

	struct aclk_lws_wss_engine_callbacks callbacks;

	//flags to be readed by engine user
	int websocket_connection_up;
	int reconnect_timeout_running;
	int data_to_read;
	int upstream_reconnect_request;
};

struct aclk_lws_wss_engine_instance* aclk_lws_wss_client_init (const struct aclk_lws_wss_engine_callbacks *callbacks, const char *target_hostname, int target_port);

int aclk_lws_wss_client_write(struct aclk_lws_wss_engine_instance *inst, void *buf, size_t count);
int aclk_lws_wss_client_read (struct aclk_lws_wss_engine_instance *inst, void *buf, size_t count);
int aclk_lws_wss_service_loop(struct aclk_lws_wss_engine_instance *inst);

int aclk_lws_wss_mqtt_layer_disconect_notif(struct aclk_lws_wss_engine_instance *inst);

#endif