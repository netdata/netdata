#include "aclk_lws_wss_client.h"

#include "libnetdata/libnetdata.h"

static int aclk_wss_callback(struct lws *wsi, enum lws_callback_reasons reason, void *user, void *in, size_t len);

struct aclk_lws_wss_perconnect_data {
    int todo;
};

struct lws_context *lws_context;

//TODO config??
int aclk_lws_wss_use_ssl = 1;

struct lws_wss_packet_buffer {
	void* data;
	size_t data_size;
	struct lws_wss_packet_buffer *next;
};

struct lws_wss_packet_buffer *lws_write_buffer_head = NULL;

static inline struct lws_wss_packet_buffer *lws_wss_packet_buffer_new(void* data, size_t size)
{
	struct lws_wss_packet_buffer *new = callocz(1, sizeof(struct lws_wss_packet_buffer));
	if(data) {
		new->data = malloc(LWS_PRE+size);
    	memcpy(new->data+LWS_PRE, data, size);
		new->data_size = size;
	}
	return new;
}

static inline void lws_wss_packet_buffer_push(struct lws_wss_packet_buffer **list, struct lws_wss_packet_buffer *item)
{
	struct lws_wss_packet_buffer *tail = *list;
	if(!*list) {
		*list = item;
		return;
	}
	while(tail->next) {
		tail = tail->next;
	}
	tail->next = item;
}

static inline struct lws_wss_packet_buffer *lws_wss_packet_buffer_pop(struct lws_wss_packet_buffer **list)
{
	struct lws_wss_packet_buffer *ret = *list;
	if(ret != NULL)
		*list = ret->next;
	//ret->next = NULL;
	return ret;
}

static inline void lws_wss_packet_buffer_free(struct lws_wss_packet_buffer *item)
{
	free(item->data);
	free(item);
}

//TODO clean me up - make normal buffering
#define LWS_DATA_BUFFER_IN_SIZE 1024*1024
char lws_data_buffer_in[LWS_DATA_BUFFER_IN_SIZE];
char* lws_data_buffer_in_rd = NULL;
char* lws_data_buffer_in_wr = NULL;
int lws_in_count = 0;

static const struct lws_protocols protocols[] = {
	{
		"aclk-wss",
		aclk_wss_callback,
		sizeof(struct aclk_lws_wss_perconnect_data),
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
struct aclk_lws_wss_engine_instance* aclk_lws_wss_client_init (void (*connection_established_callback)()) {
	struct lws_context_creation_info info;
	struct aclk_lws_wss_engine_instance *inst;
	inst = callocz(1, sizeof(struct aclk_lws_wss_engine_instance));

	memset(&info, 0, sizeof(struct lws_context_creation_info));
    info.options = LWS_SERVER_OPTION_DO_SSL_GLOBAL_INIT;
	info.port = CONTEXT_PORT_NO_LISTEN;
    info.protocols = protocols;
	info.user = inst;
	
	inst->lws_context = lws_create_context(&info);
	inst->connection_established_callback = connection_established_callback;
	lws_context = inst->lws_context;

	return inst;
}

void _aclk_wss_connect(){
	struct lws_client_connect_info i;

	//TODO make configurable
	memset(&i, 0, sizeof(i));
	i.context = lws_context;
	i.port = 9002;
	i.address = "127.0.0.1";
	i.path = "/mqtt";
	i.host = "127.0.0.1";
	i.protocol = "mqtt";
	if(aclk_lws_wss_use_ssl)
//TODO!!!!!! REMOVE ME 
#define ACLK_SSL_ALLOW_SELF_SIGNED 1
#ifdef ACLK_SSL_ALLOW_SELF_SIGNED
	i.ssl_connection = LCCSCF_USE_SSL | LCCSCF_ALLOW_SELFSIGNED | LCCSCF_SKIP_SERVER_CERT_HOSTNAME_CHECK;
#else
	i.ssl_connection = LCCSCF_USE_SSL;
#endif
	lws_client_connect_via_info(&i);
}

static int
aclk_wss_callback(struct lws *wsi, enum lws_callback_reasons reason,
			void *user, void *in, size_t len)
{
	struct aclk_lws_wss_engine_instance *inst = lws_context_user(lws_get_context(wsi));
	struct lws_wss_packet_buffer *data;

	char buffer[4096];

	switch (reason) {
	case LWS_CALLBACK_CLIENT_WRITEABLE:
		data = lws_wss_packet_buffer_pop(&lws_write_buffer_head);
		if(likely(data)) {
			lws_write(wsi, data->data + LWS_PRE, data->data_size, LWS_WRITE_BINARY);
			lws_wss_packet_buffer_free(data);
			if(lws_write_buffer_head)
				lws_callback_on_writable(inst->lws_wsi);
		}
		break;
	case LWS_CALLBACK_CLIENT_RECEIVE:
		if(!lws_data_buffer_in_rd) {
			lws_data_buffer_in_rd = lws_data_buffer_in;
			lws_data_buffer_in_wr = lws_data_buffer_in;
		}
		memcpy(lws_data_buffer_in_wr, in, len);
		lws_data_buffer_in_wr += len;
		lws_in_count += len;
		inst->data_to_read = 1; //to inform logic above there is reason to call mosquitto_loop_read
		break;
	case LWS_CALLBACK_PROTOCOL_INIT:
		//initial connection here
		//later we will reconnect with delay od ACLK_LWS_WSS_RECONNECT_TIMEOUT
		//in case this connection fails or drops
		_aclk_wss_connect();
		break;
	case LWS_CALLBACK_SERVER_NEW_CLIENT_INSTANTIATED:
		//TODO if already active make some error noise
		//currently we expect only one connection per netdata
		if(!inst) {
			//TODO Error
			return 1;
		}
		inst->lws_wsi = wsi;
		break;
	case LWS_CALLBACK_USER:
		inst->reconnect_timeout_running = 0;
		_aclk_wss_connect();
		break;
	case LWS_CALLBACK_CLIENT_CLOSED:
	case LWS_CALLBACK_WS_PEER_INITIATED_CLOSE:
	case LWS_CALLBACK_CLIENT_CONNECTION_ERROR:
		if(!inst->reconnect_timeout_running) {
			lws_timed_callback_vh_protocol(lws_get_vhost(wsi),
						   lws_get_protocol(wsi),
					       LWS_CALLBACK_USER, ACLK_LWS_WSS_RECONNECT_TIMEOUT);
			inst->reconnect_timeout_running = 1;
		}
		//no break here on purpose we want to continue with LWS_CALLBACK_WSI_DESTROY
	case LWS_CALLBACK_WSI_DESTROY:
		inst->lws_wsi = NULL;
		inst->websocket_connection_up = 0;
		break;
	case LWS_CALLBACK_CLIENT_ESTABLISHED:
		inst->websocket_connection_up = 1;
		if(inst->connection_established_callback)
			inst->connection_established_callback();
		break;
	}
	return 0; //0-OK, other connection should be closed!
}

int aclk_lws_wss_client_write(struct aclk_lws_wss_engine_instance *inst, void *buf, size_t count)
{
	if(inst && inst->lws_wsi && inst->websocket_connection_up)
	{
		lws_wss_packet_buffer_push(&lws_write_buffer_head, lws_wss_packet_buffer_new(buf, count));
		lws_callback_on_writable(inst->lws_wsi);
		return count;
	}
	return 0;
}

int aclk_lws_wss_client_read(struct aclk_lws_wss_engine_instance *inst, void *buf, size_t count)
{
	int rdsize = lws_in_count;
	
	if(lws_in_count > 0) {
		if(rdsize > count)
			rdsize = count;
		if(lws_data_buffer_in_rd + rdsize > lws_data_buffer_in_wr)
			rdsize = lws_data_buffer_in_wr - lws_data_buffer_in_rd;
		memcpy(buf, lws_data_buffer_in_rd, rdsize);
		lws_data_buffer_in_rd += rdsize;
		lws_in_count -= rdsize;
		if(lws_in_count <= 0)
			inst->data_to_read = 0;
	}
	return rdsize;
}

int aclk_lws_wss_service_loop(struct aclk_lws_wss_engine_instance *inst)
{
	return lws_service(inst->lws_context, 0);
}