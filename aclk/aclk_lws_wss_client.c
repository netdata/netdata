#include "aclk_lws_wss_client.h"

#include "libnetdata/libnetdata.h"

static int aclk_lws_wss_callback(struct lws *wsi, enum lws_callback_reasons reason, void *user, void *in, size_t len);

struct aclk_lws_wss_perconnect_data {
	int todo;
};

struct lws_wss_packet_buffer {
	unsigned char* data;
	size_t data_size;
	struct lws_wss_packet_buffer *next;
};

static inline struct lws_wss_packet_buffer *lws_wss_packet_buffer_new(void* data, size_t size)
{
	struct lws_wss_packet_buffer *new = callocz(1, sizeof(struct lws_wss_packet_buffer));
	if(data) {
		new->data = mallocz(LWS_PRE+size);
		memcpy(new->data+LWS_PRE, data, size);
		new->data_size = size;
	}
	return new;
}

static inline void lws_wss_packet_buffer_append(struct lws_wss_packet_buffer **list, struct lws_wss_packet_buffer *item)
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

	return ret;
}

static inline void lws_wss_packet_buffer_free(struct lws_wss_packet_buffer *item)
{
	freez(item->data);
	freez(item);
}

static inline void _aclk_lws_wss_read_buffer_clear(struct lws_ring *ringbuffer)
{
	size_t elems = lws_ring_get_count_waiting_elements(ringbuffer, NULL);
	if(elems > 0)
		lws_ring_consume(ringbuffer, NULL, NULL, elems);
}

static inline void _aclk_lws_wss_write_buffer_clear(struct lws_wss_packet_buffer **list)
{
	struct lws_wss_packet_buffer *i;
	while((i = lws_wss_packet_buffer_pop(list)) != NULL) {
		lws_wss_packet_buffer_free(i);
	}
	*list = NULL;
}

static inline void aclk_lws_wss_clear_io_buffers(struct aclk_lws_wss_engine_instance *inst)
{
	aclk_lws_mutex_lock(&inst->read_buf_mutex);
	_aclk_lws_wss_read_buffer_clear(inst->read_ringbuffer);
	aclk_lws_mutex_unlock(&inst->read_buf_mutex);
	aclk_lws_mutex_lock(&inst->write_buf_mutex);
	_aclk_lws_wss_write_buffer_clear(&inst->write_buffer_head);
	aclk_lws_mutex_unlock(&inst->write_buf_mutex);
}

static const struct lws_protocols protocols[] = {
	{
		"aclk-wss",
		aclk_lws_wss_callback,
		sizeof(struct aclk_lws_wss_perconnect_data),
		0, 0, 0, 0
	},
	{ NULL, NULL, 0, 0, 0, 0, 0 }
};

static void aclk_lws_wss_log_divert(int level, const char *line) {
	switch(level){
		case LLL_ERR:
			error("Libwebsockets Error: %s", line);
			break;
		case LLL_WARN:
			debug(D_ACLK, "Libwebsockets Warn: %s", line);
			break;
		default:
			error("Libwebsockets try to log with unknown log level (%d), msg: %s", level, line);
	}
}

struct aclk_lws_wss_engine_instance* aclk_lws_wss_client_init (const struct aclk_lws_wss_engine_callbacks *callbacks, const char *target_hostname, int target_port) {
	static int lws_logging_initialized = 0;
	struct lws_context_creation_info info;
	struct aclk_lws_wss_engine_instance *inst;

	if(unlikely(!lws_logging_initialized)) {
		lws_set_log_level(LLL_ERR | LLL_WARN, aclk_lws_wss_log_divert);
		lws_logging_initialized = 1;
	}

	if(!callbacks || !target_hostname)
		return NULL;

	inst = callocz(1, sizeof(struct aclk_lws_wss_engine_instance));

	inst->host = target_hostname;
	inst->port = target_port;

	memset(&info, 0, sizeof(struct lws_context_creation_info));
	info.options = LWS_SERVER_OPTION_DO_SSL_GLOBAL_INIT;
	info.port = CONTEXT_PORT_NO_LISTEN;
	info.protocols = protocols;
	info.user = inst;
	
	inst->lws_context = lws_create_context(&info);
	if(!inst->lws_context)
		goto failure_cleanup_2;

	inst->callbacks = *callbacks;

	aclk_lws_mutex_init(&inst->write_buf_mutex);
	aclk_lws_mutex_init(&inst->read_buf_mutex);

	inst->read_ringbuffer = lws_ring_create(1, ACLK_LWS_WSS_RECV_BUFF_SIZE_BYTES, NULL);
	if(!inst->read_ringbuffer)
		goto failure_cleanup;

	return inst;

failure_cleanup:
	lws_context_destroy(inst->lws_context);
failure_cleanup_2:
	freez(inst);
	return NULL;
}

void aclk_lws_wss_client_destroy(struct aclk_lws_wss_engine_instance* inst) {
	lws_context_destroy(inst->lws_context);
	inst->lws_context = NULL;
	inst->lws_wsi = NULL;

	aclk_lws_wss_clear_io_buffers(inst);

#ifdef ACLK_LWS_MOSQUITTO_IO_CALLS_MULTITHREADED
	pthread_mutex_destroy(&inst->write_buf_mutex);
	pthread_mutex_destroy(&inst->read_buf_mutex);
#endif
}

void _aclk_wss_connect(struct aclk_lws_wss_engine_instance *inst){
	struct lws_client_connect_info i;

	memset(&i, 0, sizeof(i));
	i.context = inst->lws_context;
	i.port = inst->port;
	i.address = inst->host;
	i.path = "/mqtt";
	i.host = inst->host;
	i.protocol = "mqtt";
#ifdef ACLK_SSL_ALLOW_SELF_SIGNED
	i.ssl_connection = LCCSCF_USE_SSL | LCCSCF_ALLOW_SELFSIGNED | LCCSCF_SKIP_SERVER_CERT_HOSTNAME_CHECK;
#else
	i.ssl_connection = LCCSCF_USE_SSL;
#endif
	lws_client_connect_via_info(&i);
}

static inline int received_data_to_ringbuff(struct lws_ring *buffer, void* data, size_t len) {
	if( lws_ring_insert(buffer, data, len) != len ) {
		error("ACLK_LWS_WSS_CLIENT: receive buffer full. Closing connection to prevent flooding.");
		return 0;
	}
	return 1;
}

static int
aclk_lws_wss_callback(struct lws *wsi, enum lws_callback_reasons reason,
			void *user, void *in, size_t len)
{
	UNUSED(user);
	struct aclk_lws_wss_engine_instance *inst = lws_context_user(lws_get_context(wsi));
	struct lws_wss_packet_buffer *data;
	int retval = 0;

	if( !inst ) {
		error("Callback received without any aclk_lws_wss_engine_instance!");
		return -1;
	}

	if( inst->upstream_reconnect_request ) {
		error("Closing lws connectino due to libmosquitto error.");
		char *upstream_connection_error = "MQTT protocol error. Closing underlying wss connection.";
		lws_close_reason(wsi, LWS_CLOSE_STATUS_PROTOCOL_ERR, (unsigned char*)upstream_connection_error, strlen(upstream_connection_error));
		retval = -1;
		inst->upstream_reconnect_request = 0;
	}

	switch (reason) {
	case LWS_CALLBACK_CLIENT_WRITEABLE:
		aclk_lws_mutex_lock(&inst->write_buf_mutex);
		data = lws_wss_packet_buffer_pop(&inst->write_buffer_head);
		if(likely(data)) {
			lws_write(wsi, data->data + LWS_PRE, data->data_size, LWS_WRITE_BINARY);
			lws_wss_packet_buffer_free(data);
			if(inst->write_buffer_head)
				lws_callback_on_writable(inst->lws_wsi);
		}
		aclk_lws_mutex_unlock(&inst->write_buf_mutex);
		break;
	case LWS_CALLBACK_CLIENT_RECEIVE:
		aclk_lws_mutex_lock(&inst->read_buf_mutex);
		if(!received_data_to_ringbuff(inst->read_ringbuffer, in, len))
			retval = 1;
		aclk_lws_mutex_unlock(&inst->read_buf_mutex);

		if(likely(inst->callbacks.data_rcvd_callback))
			// to future myself -> do not call this while read lock is active as it will eventually
			// want to acquire same lock later in aclk_lws_wss_client_read() function
			inst->callbacks.data_rcvd_callback();
		else
			inst->data_to_read = 1; //to inform logic above there is reason to call mosquitto_loop_read
		break;
	case LWS_CALLBACK_PROTOCOL_INIT:
		//initial connection here
		//later we will reconnect with delay od ACLK_LWS_WSS_RECONNECT_TIMEOUT
		//in case this connection fails or drops
		_aclk_wss_connect(inst);
		break;
	case LWS_CALLBACK_SERVER_NEW_CLIENT_INSTANTIATED:
		//TODO if already active make some error noise
		//currently we expect only one connection per netdata
		inst->lws_wsi = wsi;
		break;
#ifdef AUTO_RECONNECT_ON_LWS_LAYER
	case LWS_CALLBACK_USER:
		inst->reconnect_timeout_running = 0;
		_aclk_wss_connect(inst);
		break;
#endif
	case LWS_CALLBACK_CLIENT_CONNECTION_ERROR:
		error("Could not connect MQTT over WSS server \"%s:%d\". LwsReason:\"%s\"", inst->host, inst->port, (in ? (char*)in : "not given"));
		/* FALLTHRU */
	case LWS_CALLBACK_CLIENT_CLOSED:
	case LWS_CALLBACK_WS_PEER_INITIATED_CLOSE:
#ifdef AUTO_RECONNECT_ON_LWS_LAYER
		if(!inst->reconnect_timeout_running) {
			lws_timed_callback_vh_protocol(lws_get_vhost(wsi),
						lws_get_protocol(wsi),
						LWS_CALLBACK_USER, ACLK_LWS_WSS_RECONNECT_TIMEOUT);
			inst->reconnect_timeout_running = 1;
		}
		/* FALLTHRU */
#endif
		//no break here on purpose we want to continue with LWS_CALLBACK_WSI_DESTROY
	case LWS_CALLBACK_WSI_DESTROY:
		aclk_lws_wss_clear_io_buffers(inst);
		inst->lws_wsi = NULL;
		inst->websocket_connection_up = 0;
		break;
	case LWS_CALLBACK_CLIENT_ESTABLISHED:
		inst->websocket_connection_up = 1;
		if(inst->callbacks.connection_established_callback)
			inst->callbacks.connection_established_callback();
		break;
	default:
		break;
	}
	return retval; //0-OK, other connection should be closed!
}

int aclk_lws_wss_client_write(struct aclk_lws_wss_engine_instance *inst, void *buf, size_t count)
{
	if(inst && inst->lws_wsi && inst->websocket_connection_up)
	{
		aclk_lws_mutex_lock(&inst->write_buf_mutex);
		lws_wss_packet_buffer_append(&inst->write_buffer_head, lws_wss_packet_buffer_new(buf, count));
		aclk_lws_mutex_unlock(&inst->write_buf_mutex);

		lws_callback_on_writable(inst->lws_wsi);
		return count;
	}
	return 0;
}

int aclk_lws_wss_client_read(struct aclk_lws_wss_engine_instance *inst, void *buf, size_t count)
{
	size_t data_to_be_read = count;

	aclk_lws_mutex_lock(&inst->read_buf_mutex);
	size_t readable_byte_count = lws_ring_get_count_waiting_elements(inst->read_ringbuffer, NULL);
	if(unlikely(readable_byte_count == 0)) {
		errno = EAGAIN;
		data_to_be_read = -1;
		goto abort;
	}

	if( readable_byte_count < data_to_be_read )
		data_to_be_read = readable_byte_count;

	data_to_be_read = lws_ring_consume(inst->read_ringbuffer, NULL, buf, data_to_be_read);
	if(data_to_be_read == readable_byte_count)
		inst->data_to_read = 0;

abort:
	aclk_lws_mutex_unlock(&inst->read_buf_mutex);
	return data_to_be_read;
}

int aclk_lws_wss_service_loop(struct aclk_lws_wss_engine_instance *inst)
{
	return lws_service(inst->lws_context, 0);
}

// in case the MQTT connection disconnect while lws transport is still operational
// we should drop connection and reconnect
// this function should be called when that happens to notify lws of that situation
void aclk_lws_wss_mqtt_layer_disconect_notif(struct aclk_lws_wss_engine_instance *inst)
{
	if(inst->lws_wsi && inst->websocket_connection_up) {
		inst->upstream_reconnect_request = 1;
		lws_callback_on_writable(inst->lws_wsi); //here we just do it to ensure we get callback called from lws, we don't need any actual data to be written.
	}
}