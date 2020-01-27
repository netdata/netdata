#include "aclk_lws_wss_client.h"

#include "libnetdata/libnetdata.h"

static int aclk_lws_wss_callback(struct lws *wsi, enum lws_callback_reasons reason, void *user, void *in, size_t len);

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

// buffer used for outgoing packets
// we store them here when we wait for lws_writable callback
// notifying us that we can write to socket
struct lws_wss_packet_buffer *aclk_lws_wss_write_buffer_head = NULL;

#define ACLK_LWS_WSS_RECV_BUFF_SIZE_BYTES 128*1024
struct lws_ring *aclk_lws_wss_read_ringbuffer = NULL;

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

static inline void _aclk_lws_wss_read_buffer_clear()
{
	size_t elems = lws_ring_get_count_waiting_elements(aclk_lws_wss_read_ringbuffer, NULL);
	lws_ring_consume(aclk_lws_wss_read_ringbuffer, NULL, NULL, elems);
}

static inline void _aclk_lws_wss_write_buffer_clear()
{
	struct lws_wss_packet_buffer *i;
	while(i = lws_wss_packet_buffer_pop(&aclk_lws_wss_write_buffer_head)) {
		lws_wss_packet_buffer_free(i);
	}
	aclk_lws_wss_write_buffer_head = NULL;
}

static inline void aclk_lws_wss_clear_io_buffers()
{
	_aclk_lws_wss_read_buffer_clear();
	_aclk_lws_wss_write_buffer_clear();
}

static const struct lws_protocols protocols[] = {
	{
		"aclk-wss",
		aclk_lws_wss_callback,
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
	if(!inst)
		return NULL;

	memset(&info, 0, sizeof(struct lws_context_creation_info));
    info.options = LWS_SERVER_OPTION_DO_SSL_GLOBAL_INIT;
	info.port = CONTEXT_PORT_NO_LISTEN;
    info.protocols = protocols;
	info.user = inst;
	
	inst->lws_context = lws_create_context(&info);
	inst->connection_established_callback = connection_established_callback;
	lws_context = inst->lws_context;

	aclk_lws_wss_read_ringbuffer = lws_ring_create(1, ACLK_LWS_WSS_RECV_BUFF_SIZE_BYTES, NULL);
	if(!aclk_lws_wss_read_ringbuffer)
		goto failure_cleanup;

	return inst;

failure_cleanup:
	lws_context_destroy(inst->lws_context);
	free(inst);
	return NULL;
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

static inline int received_data_to_ringbuff(void* data, size_t len) {
	if( lws_ring_insert(aclk_lws_wss_read_ringbuffer, data, len) != len ) {
		error("ACLK_LWS_WSS_CLIENT: receive buffer full. Closing connection to prevent flooding.");
		return 0;
	}
	return 1;
}

static int
aclk_lws_wss_callback(struct lws *wsi, enum lws_callback_reasons reason,
			void *user, void *in, size_t len)
{
	struct aclk_lws_wss_engine_instance *inst = lws_context_user(lws_get_context(wsi));
	struct lws_wss_packet_buffer *data;

	switch (reason) {
	case LWS_CALLBACK_CLIENT_WRITEABLE:
		data = lws_wss_packet_buffer_pop(&aclk_lws_wss_write_buffer_head);
		if(likely(data)) {
			lws_write(wsi, data->data + LWS_PRE, data->data_size, LWS_WRITE_BINARY);
			lws_wss_packet_buffer_free(data);
			if(aclk_lws_wss_write_buffer_head)
				lws_callback_on_writable(inst->lws_wsi);
		}
		break;
	case LWS_CALLBACK_CLIENT_RECEIVE:
		if(!received_data_to_ringbuff(in, len))
			return 1;
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
		aclk_lws_wss_clear_io_buffers();
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
		lws_wss_packet_buffer_push(&aclk_lws_wss_write_buffer_head, lws_wss_packet_buffer_new(buf, count));
		lws_callback_on_writable(inst->lws_wsi);
		return count;
	}
	return 0;
}

int aclk_lws_wss_client_read(struct aclk_lws_wss_engine_instance *inst, void *buf, size_t count)
{
	size_t data_to_be_read = count;
	size_t readable_byte_count = lws_ring_get_count_waiting_elements(aclk_lws_wss_read_ringbuffer, NULL);
	if(unlikely(readable_byte_count <= 0)) {
		errno = EAGAIN;
		return -1;
	}

	if( readable_byte_count < data_to_be_read )
		data_to_be_read = readable_byte_count;

	data_to_be_read = lws_ring_consume(aclk_lws_wss_read_ringbuffer, NULL, buf, data_to_be_read);
	if(data_to_be_read == readable_byte_count)
		inst->data_to_read = 0;

	return data_to_be_read;
}

int aclk_lws_wss_service_loop(struct aclk_lws_wss_engine_instance *inst)
{
	return lws_service(inst->lws_context, 0);
}