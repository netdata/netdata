#include "lws_wss_client.h"

#include "libnetdata/libnetdata.h"

struct aclk_lws_wss_perconnect_data {
    int todo;
}

static const struct aclk_lws_protocols protocols[] = {
	{
		"aclk-wss",
		aclk-wss-callback,
		sizeof(struct lws_wss_perconnect_data),
		0,
	},
	{ NULL, NULL, 0, 0 }
};

/**
 * libwebsockets WSS client code thread entry point
 *
 * 
 *
 * @param ptr is a pointer to the netdata_static_thread structure.
 *
 * @return always NULL
 */
void *aclk_lws_wss_client_thread_main (void *t_param) {
	struct lws_context_creation_info info;
	memset(&info, 0, sizeof info);
    info.options = LWS_SERVER_OPTION_DO_SSL_GLOBAL_INIT;
	info.port = CONTEXT_PORT_NO_LISTEN;
    info.protocols = protocols;

}