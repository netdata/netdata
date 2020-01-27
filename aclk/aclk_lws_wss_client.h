#ifndef ACLK_LWS_WSS_CLIENT_H
#define ACLK_LWS_WSS_CLIENT_H

#include <libwebsockets.h>

#define ACLK_LWS_WSS_RECONNECT_TIMEOUT 5

struct aclk_lws_wss_engine_instance {
	struct lws_context *lws_context;
	struct lws *lws_wsi;
	void (*connection_established_callback)();
	int websocket_connection_up;
	int reconnect_timeout_running;
	int data_to_read;
	int upstream_reconnect_request;
};

struct aclk_lws_wss_engine_instance* aclk_lws_wss_client_init (void (*connection_established_callback)());

int aclk_lws_wss_client_write(struct aclk_lws_wss_engine_instance *inst, void *buf, size_t count);
int aclk_lws_wss_client_read (struct aclk_lws_wss_engine_instance *inst, void *buf, size_t count);
int aclk_lws_wss_service_loop(struct aclk_lws_wss_engine_instance *inst);

int aclk_lws_wss_mqtt_layer_disconect_notif(struct aclk_lws_wss_engine_instance *inst);

#endif