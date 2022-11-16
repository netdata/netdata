#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <pthread.h>
#include <inttypes.h>

#include "common_internal.h"
#include "mqtt_constants.h"
#include "mqtt_wss_log.h"
#include "mqtt_ng.h"

#define UNIT_LOG_PREFIX "mqtt_client: "
#define FATAL(fmt, ...) mws_fatal(client->log, UNIT_LOG_PREFIX fmt, ##__VA_ARGS__)
#define ERROR(fmt, ...) mws_error(client->log, UNIT_LOG_PREFIX fmt, ##__VA_ARGS__)
#define WARN(fmt, ...)  mws_warn (client->log, UNIT_LOG_PREFIX fmt, ##__VA_ARGS__)
#define INFO(fmt, ...)  mws_info (client->log, UNIT_LOG_PREFIX fmt, ##__VA_ARGS__)
#define DEBUG(fmt, ...) mws_debug(client->log, UNIT_LOG_PREFIX fmt, ##__VA_ARGS__)

#define SMALL_STRING_DONT_FRAGMENT_LIMIT 128

#define MIN(a,b) (((a)<(b))?(a):(b))

#define LOCK_HDR_BUFFER(buffer) pthread_mutex_lock(&((buffer)->mutex))
#define UNLOCK_HDR_BUFFER(buffer) pthread_mutex_unlock(&((buffer)->mutex))

#define BUFFER_FRAG_GARBAGE_COLLECT         0x01
// some packets can be marked for garbage collection
// immediately when they are sent (e.g. sent PUBACK on QoS1)
#define BUFFER_FRAG_GARBAGE_COLLECT_ON_SEND 0x02
// as buffer fragment can point to both
// external data and data in the same buffer
// we mark the former case with BUFFER_FRAG_DATA_EXTERNAL
#define BUFFER_FRAG_DATA_EXTERNAL           0x04
// as single MQTT Packet can be stored into multiple
// buffer fragments (depending on copy requirements)
// this marks this fragment to be the first/last
#define BUFFER_FRAG_MQTT_PACKET_HEAD        0x10
#define BUFFER_FRAG_MQTT_PACKET_TAIL        0x20

typedef uint16_t buffer_frag_flag_t;
struct buffer_fragment {
    size_t len;
    size_t sent;
    buffer_frag_flag_t flags;
    void (*free_fnc)(void *ptr);
    char *data;

    uint16_t packet_id;

    struct buffer_fragment *next;
};

typedef struct buffer_fragment *mqtt_msg_data;

// buffer used for MQTT headers only
// not for actual data sent
struct header_buffer {
    size_t size;
    char *data;
    char *tail;
    struct buffer_fragment *tail_frag;
};

struct transaction_buffer {
    struct header_buffer hdr_buffer;
    // used while building new message
    // to be able to revert state easily
    // in case of error mid processing
    struct header_buffer state_backup;
    pthread_mutex_t mutex;
    struct buffer_fragment *sending_frag;
};

enum mqtt_client_state {
    RAW = 0,
    CONNECT_PENDING,
    CONNECTING,
    CONNECTED, 
    ERROR,
    DISCONNECTED
};

enum parser_state {
    MQTT_PARSE_FIXED_HEADER_PACKET_TYPE = 0,
    MQTT_PARSE_FIXED_HEADER_LEN,
    MQTT_PARSE_VARIABLE_HEADER,
    MQTT_PARSE_MQTT_PACKET_DONE
};

enum varhdr_parser_state {
    MQTT_PARSE_VARHDR_INITIAL = 0,
    MQTT_PARSE_VARHDR_OPTIONAL_REASON_CODE,
    MQTT_PARSE_VARHDR_PROPS,
    MQTT_PARSE_VARHDR_TOPICNAME,
    MQTT_PARSE_VARHDR_PACKET_ID,
    MQTT_PARSE_REASONCODES,
    MQTT_PARSE_PAYLOAD
};

struct mqtt_vbi_parser_ctx {
    char data[MQTT_VBI_MAXBYTES];
    uint8_t bytes;
    uint32_t result;
};

struct mqtt_property {
    uint32_t id;
    struct mqtt_property *next;
};

enum mqtt_properties_parser_state {
    PROPERTIES_LENGTH = 0,
    PROPERTY_ID
};

struct mqtt_properties_parser_ctx {
    enum mqtt_properties_parser_state state;
    struct mqtt_property *head;
    uint32_t properties_length;
    struct mqtt_vbi_parser_ctx vbi_parser_ctx;
    size_t bytes_consumed;
};

struct mqtt_connack {
    uint8_t flags;
    uint8_t reason_code;
};
struct mqtt_puback {
    uint16_t packet_id;
    uint8_t reason_code;
};

struct mqtt_suback {
    uint16_t packet_id;
    uint8_t *reason_codes;
    uint8_t reason_code_count;
    uint8_t reason_codes_pending;
};

struct mqtt_publish {
    uint16_t topic_len;
    char *topic;
    uint16_t packet_id;
    size_t data_len;
    char *data;
    uint8_t qos;
};

struct mqtt_disconnect {
    uint8_t reason_code;
};

struct mqtt_ng_parser {
    rbuf_t received_data;

    uint8_t mqtt_control_packet_type;
    uint32_t mqtt_fixed_hdr_remaining_length;
    size_t mqtt_parsed_len;

    struct mqtt_vbi_parser_ctx vbi_parser;
    struct mqtt_properties_parser_ctx properties_parser;

    enum parser_state state;
    enum varhdr_parser_state varhdr_state;

    struct mqtt_property *varhdr_properties;

    union {
        struct mqtt_connack connack;
        struct mqtt_puback puback;
        struct mqtt_suback suback;
        struct mqtt_publish publish;
        struct mqtt_disconnect disconnect;
    } mqtt_packet;
};


struct mqtt_ng_client {
    struct transaction_buffer main_buffer;

    enum mqtt_client_state client_state;

    mqtt_msg_data connect_msg;

    mqtt_wss_log_ctx_t log;

    mqtt_ng_send_fnc_t send_fnc_ptr;
    void *user_ctx;

    // time when last fragment of MQTT message was sent
    time_t time_of_last_send;

    struct mqtt_ng_parser parser;

    size_t max_mem_bytes;

    void (*puback_callback)(uint16_t packet_id);
    void (*connack_callback)(void* user_ctx, int connack_reply);
    void (*msg_callback)(const char *topic, const void *msg, size_t msglen, int qos);

    unsigned int ping_pending:1;
};

char pingreq[] = { MQTT_CPT_PINGREQ << 4, 0x00 };

struct buffer_fragment ping_frag = {
    .data = pingreq,
    .flags =  BUFFER_FRAG_MQTT_PACKET_HEAD | BUFFER_FRAG_MQTT_PACKET_TAIL,
    .free_fnc = NULL,
    .len = sizeof(pingreq),
    .next = NULL,
    .sent = 0,
    .packet_id = 0
};

int uint32_to_mqtt_vbi(uint32_t input, char *output) {
    int i = 1;
    *output = 0;

    /* MQTT 5 specs allows max 4 bytes of output
       making it 0xFF, 0xFF, 0xFF, 0x7F
       representing number 268435455 decimal
       see 1.5.5. Variable Byte Integer */
    if(input >= 256 * 1024 * 1024)
        return 0;

    if(!input) {
        *output = 0;
        return 1;
    }

    while(input) {
        output[i-1] = input & MQTT_VBI_DATA_MASK;
        input >>= 7;
        if (input)
            output[i-1] |= MQTT_VBI_CONTINUATION_FLAG;
        i++;
    }
    return i - 1;
}

int mqtt_vbi_to_uint32(char *input, uint32_t *output) {
    // dont want to operate directly on output
    // as I want it to be possible for input and output
    // pointer to be the same
    uint32_t result = 0;
    uint32_t multiplier = 1;

    do {
        result += (uint32_t)(*input & MQTT_VBI_DATA_MASK) * multiplier;
        if (multiplier > 128*128*128)
            return 1;
        multiplier <<= 7;
    } while (*input++ & MQTT_VBI_CONTINUATION_FLAG);
    *output = result;
    return 0;
}

#ifdef TESTS
#include <stdio.h>
#define MQTT_VBI_MAXLEN 4
// we add extra byte to check we dont write out of bounds
// in case where 4 bytes are supposed to be written
static const char _mqtt_vbi_0[MQTT_VBI_MAXLEN + 1] = { 0x00, 0x00, 0x00, 0x00, 0x00 };
static const char _mqtt_vbi_127[MQTT_VBI_MAXLEN + 1] = { 0x7F, 0x00, 0x00, 0x00, 0x00 };
static const char _mqtt_vbi_128[MQTT_VBI_MAXLEN + 1] = { 0x80, 0x01, 0x00, 0x00, 0x00 };
static const char _mqtt_vbi_16383[MQTT_VBI_MAXLEN + 1] = { 0xFF, 0x7F, 0x00, 0x00, 0x00 };
static const char _mqtt_vbi_16384[MQTT_VBI_MAXLEN + 1] = { 0x80, 0x80, 0x01, 0x00, 0x00 };
static const char _mqtt_vbi_2097151[MQTT_VBI_MAXLEN + 1] = { 0xFF, 0xFF, 0x7F, 0x00, 0x00 };
static const char _mqtt_vbi_2097152[MQTT_VBI_MAXLEN + 1] = { 0x80, 0x80, 0x80, 0x01, 0x00 };
static const char _mqtt_vbi_268435455[MQTT_VBI_MAXLEN + 1] = { 0xFF, 0xFF, 0xFF, 0x7F, 0x00 };
static const char _mqtt_vbi_999999999[MQTT_VBI_MAXLEN + 1] = { 0x80, 0x80, 0x80, 0x80, 0x01 };

#define MQTT_VBI_TESTCASE(case, expected_len) \
    { \
    memset(buf, 0, MQTT_VBI_MAXLEN + 1); \
    int len; \
    if ((len=uint32_to_mqtt_vbi(case, buf)) != expected_len) { \
        fprintf(stderr, "uint32_to_mqtt_vbi(case:%d, line:%d): Incorrect length returned. Expected %d, Got %d\n", case, __LINE__, expected_len, len); \
        return 1; \
    } \
    if (memcmp(buf, _mqtt_vbi_ ## case, MQTT_VBI_MAXLEN + 1 )) { \
        fprintf(stderr, "uint32_to_mqtt_vbi(case:%d, line:%d): Wrong output\n", case, __LINE__); \
        return 1; \
    } }


int test_uint32_mqtt_vbi() {
    char buf[MQTT_VBI_MAXLEN + 1];

    MQTT_VBI_TESTCASE(0,         1)
    MQTT_VBI_TESTCASE(127,       1)
    MQTT_VBI_TESTCASE(128,       2)
    MQTT_VBI_TESTCASE(16383,     2)
    MQTT_VBI_TESTCASE(16384,     3)
    MQTT_VBI_TESTCASE(2097151,   3)
    MQTT_VBI_TESTCASE(2097152,   4)
    MQTT_VBI_TESTCASE(268435455, 4)

    memset(buf, 0, MQTT_VBI_MAXLEN + 1);
    int len;
    if ((len=uint32_to_mqtt_vbi(268435456, buf)) != 0) {
        fprintf(stderr, "uint32_to_mqtt_vbi(case:268435456, line:%d): Incorrect length returned. Expected 0, Got %d\n", __LINE__, len);
        return 1;
    }

    return 0;
}

#define MQTT_VBI2UINT_TESTCASE(case, expected_error) \
    { \
    uint32_t result; \
    int ret = mqtt_vbi_to_uint32(_mqtt_vbi_ ## case, &result); \
    if (ret && !(expected_error)) { \
        fprintf(stderr, "mqtt_vbi_to_uint(case:%d, line:%d): Unexpectedly Errored\n", (case), __LINE__); \
        return 1; \
    } \
    if (!ret && (expected_error)) { \
        fprintf(stderr, "mqtt_vbi_to_uint(case:%d, line:%d): Should return error but didnt\n", (case), __LINE__); \
        return 1; \
    } \
    if (!ret && result != (case)) { \
        fprintf(stderr, "mqtt_vbi_to_uint(case:%d, line:%d): Returned wrong result %d\n", (case), __LINE__, result); \
        return 1; \
    }}


int test_mqtt_vbi_to_uint32() {
    MQTT_VBI2UINT_TESTCASE(0,         0)
    MQTT_VBI2UINT_TESTCASE(127,       0)
    MQTT_VBI2UINT_TESTCASE(128,       0)
    MQTT_VBI2UINT_TESTCASE(16383,     0)
    MQTT_VBI2UINT_TESTCASE(16384,     0)
    MQTT_VBI2UINT_TESTCASE(2097151,   0)
    MQTT_VBI2UINT_TESTCASE(2097152,   0)
    MQTT_VBI2UINT_TESTCASE(268435455, 0)
    MQTT_VBI2UINT_TESTCASE(999999999, 1)
    return 0;
}
#endif /* TESTS */

// this helps with switch statements
// as they have to use integer type (not pointer)
enum memory_mode {
    MEMCPY,
    EXTERNAL_FREE_AFTER_USE,
    CALLER_RESPONSIBLE
};

static inline enum memory_mode ptr2memory_mode(void * ptr) {
    if (ptr == NULL)
        return MEMCPY;
    if (ptr == CALLER_RESPONSIBILITY)
        return CALLER_RESPONSIBLE;
    return EXTERNAL_FREE_AFTER_USE;
}

#define frag_is_marked_for_gc(frag) ((frag->flags & BUFFER_FRAG_GARBAGE_COLLECT) || ((frag->flags & BUFFER_FRAG_GARBAGE_COLLECT_ON_SEND) && frag->sent == frag->len))

static void buffer_frag_free_data(struct buffer_fragment *frag)
{
    if ( frag->flags & BUFFER_FRAG_DATA_EXTERNAL && frag->data != NULL) {
        switch (ptr2memory_mode(frag->free_fnc)) {
            case MEMCPY:
                mw_free(frag->data);
                break;
            case EXTERNAL_FREE_AFTER_USE:
                frag->free_fnc(frag->data);
                break;
            case CALLER_RESPONSIBLE:
                break;
        }
        frag->data = NULL;
    }
}

#define HEADER_BUFFER_SIZE 1024*1024
#define GROWTH_FACTOR 1.25

#define BUFFER_BYTES_USED(buf) ((size_t)((buf)->tail - (buf)->data))
#define BUFFER_BYTES_AVAILABLE(buf) ((buf)->size - BUFFER_BYTES_USED(buf))
#define BUFFER_FIRST_FRAG(buf) ((struct buffer_fragment *)((buf)->tail_frag ? (buf)->data : NULL))
static void buffer_purge(struct header_buffer *buf) {
    struct buffer_fragment *frag = BUFFER_FIRST_FRAG(buf);
    while (frag) {
        buffer_frag_free_data(frag);
        frag = frag->next;
    }
    buf->tail = buf->data;
    buf->tail_frag = NULL;
}

static struct buffer_fragment *buffer_new_frag(struct header_buffer *buf, buffer_frag_flag_t flags)
{
    if (BUFFER_BYTES_AVAILABLE(buf) < sizeof(struct buffer_fragment))
        return NULL;

    struct buffer_fragment *frag = (struct buffer_fragment *)buf->tail;
    memset(frag, 0, sizeof(*frag));
    buf->tail += sizeof(*frag);

    if (/*!((frag)->flags & BUFFER_FRAG_MQTT_PACKET_HEAD) &&*/ buf->tail_frag)
        buf->tail_frag->next = frag;

    buf->tail_frag = frag;

    frag->data = buf->tail;

    frag->flags = flags;

    return frag;
}

static void buffer_rebuild(struct header_buffer *buf)
{
    struct buffer_fragment *frag = (struct buffer_fragment*)buf->data;
    do {
        buf->tail = (char*)frag + sizeof(struct buffer_fragment);
        buf->tail_frag = frag;
        if (!(frag->flags & BUFFER_FRAG_DATA_EXTERNAL)) {
            buf->tail_frag->data = buf->tail;
            buf->tail += frag->len;
        }
        if (frag->next != NULL)
            frag->next = (struct buffer_fragment*)buf->tail;
        frag = frag->next;
    } while(frag);
}

static void buffer_garbage_collect(struct header_buffer *buf, mqtt_wss_log_ctx_t log_ctx)
{
#if !defined(MQTT_DEBUG_VERBOSE) && !defined(ADDITIONAL_CHECKS)
    (void) log_ctx;
#endif
#ifdef MQTT_DEBUG_VERBOSE
        mws_debug(log_ctx, "Buffer Garbage Collection!");
#endif

    struct buffer_fragment *frag = BUFFER_FIRST_FRAG(buf);
    while (frag) {
        if (!frag_is_marked_for_gc(frag))
            break;

        buffer_frag_free_data(frag);

        frag = frag->next;
    }

    if (frag == BUFFER_FIRST_FRAG(buf)) {
#ifdef MQTT_DEBUG_VERBOSE
        mws_debug(log_ctx, "Buffer Garbage Collection! No Space Reclaimed!");
#endif
        return;
    }

    if (!frag) {
        buf->tail_frag = NULL;
        buf->tail = buf->data;
        return;
    }

#ifdef ADDITIONAL_CHECKS
    if (!(frag->flags & BUFFER_FRAG_MQTT_PACKET_HEAD)) {
        mws_error(log_ctx, "Expected to find end of buffer (NULL) or next packet head!");
        return;
    }
#endif

    memmove(buf->data, frag, buf->tail - (char*)frag);
    buffer_rebuild(buf);
}

static void transaction_buffer_garbage_collect(struct transaction_buffer *buf, mqtt_wss_log_ctx_t log_ctx)
{
#ifdef MQTT_DEBUG_VERBOSE
    mws_debug(log_ctx, "Transaction Buffer Garbage Collection! %s", buf->sending_frag == NULL ? "NULL" : "in flight message");
#endif

    // Invalidate the cached sending fragment
    // as we will move data around
    if (buf->sending_frag != &ping_frag)
        buf->sending_frag = NULL;

    buffer_garbage_collect(&buf->hdr_buffer, log_ctx);
}

static int transaction_buffer_grow(struct transaction_buffer *buf, mqtt_wss_log_ctx_t log_ctx, float rate, size_t max)
{
    if (buf->hdr_buffer.size >= max)
        return 0;

    // Invalidate the cached sending fragment
    // as we will move data around
    if (buf->sending_frag != &ping_frag)
        buf->sending_frag = NULL;

    buf->hdr_buffer.size *= rate;
    if (buf->hdr_buffer.size > max)
        buf->hdr_buffer.size = max;

    void *ret = mw_realloc(buf->hdr_buffer.data, buf->hdr_buffer.size);
    if (ret == NULL) {
        mws_warn(log_ctx, "Buffer growth failed (realloc)");
        return 1;
    }

    mws_debug(log_ctx, "Message metadata buffer was grown");

    buf->hdr_buffer.data = ret;
    buffer_rebuild(&buf->hdr_buffer);
    return 0;
}

inline static int transaction_buffer_init(struct transaction_buffer *to_init, size_t size)
{
    pthread_mutex_init(&to_init->mutex, NULL);

    to_init->hdr_buffer.size = size;
    to_init->hdr_buffer.data = mw_malloc(size);
    if (to_init->hdr_buffer.data == NULL)
        return 1;

    to_init->hdr_buffer.tail = to_init->hdr_buffer.data;
    to_init->hdr_buffer.tail_frag = NULL;
    return 0;
}

static void transaction_buffer_destroy(struct transaction_buffer *to_init)
{
    buffer_purge(&to_init->hdr_buffer);
    pthread_mutex_destroy(&to_init->mutex);
    mw_free(to_init->hdr_buffer.data);
}

// Creates transaction
// saves state of buffer before any operation was done
// allowing for rollback if things go wrong
#define transaction_buffer_transaction_start(buf) \
  { LOCK_HDR_BUFFER(buf); \
    memcpy(&(buf)->state_backup, &(buf)->hdr_buffer, sizeof((buf)->hdr_buffer)); }

#define transaction_buffer_transaction_commit(buf) UNLOCK_HDR_BUFFER(buf);

void transaction_buffer_transaction_rollback(struct transaction_buffer *buf, struct buffer_fragment *frag)
{
    memcpy(&buf->hdr_buffer, &buf->state_backup, sizeof(buf->hdr_buffer));
    if (buf->hdr_buffer.tail_frag != NULL)
        buf->hdr_buffer.tail_frag->next = NULL;

    while(frag) {
        buffer_frag_free_data(frag);
        // we are not actually freeing the structure itself
        // just the data it manages
        // structure itself is in permanent buffer
        // which is locked by HDR_BUFFER lock
        frag = frag->next;
    }

    UNLOCK_HDR_BUFFER(buf);
}

struct mqtt_ng_client *mqtt_ng_init(struct mqtt_ng_init *settings)
{
    struct mqtt_ng_client *client = mw_calloc(1, sizeof(struct mqtt_ng_client));
    if (client == NULL)
        return NULL;

    if (transaction_buffer_init(&client->main_buffer, HEADER_BUFFER_SIZE)) {
        mw_free(client);
        return NULL;
    }

    // TODO just embed the struct into mqtt_ng_client
    client->parser.received_data = settings->data_in;
    client->send_fnc_ptr = settings->data_out_fnc;
    client->user_ctx = settings->user_ctx;

    client->log = settings->log;

    client->puback_callback = settings->puback_callback;
    client->connack_callback = settings->connack_callback;
    client->msg_callback = settings->msg_callback;

    return client;
}

static inline uint8_t get_control_packet_type(uint8_t first_hdr_byte)
{
    return first_hdr_byte >> 4;
}

void mqtt_ng_destroy(struct mqtt_ng_client *client)
{
    transaction_buffer_destroy(&client->main_buffer);
    mw_free(client);
}

int frag_set_external_data(mqtt_wss_log_ctx_t log, struct buffer_fragment *frag, void *data, size_t data_len, free_fnc_t data_free_fnc)
{
    if (frag->len) {
        // TODO?: This could potentially be done in future if we set rule
        // external data always follows in buffer data
        // could help reduce fragmentation in some messages but
        // currently not worth it considering time is tight
        mws_fatal(log, UNIT_LOG_PREFIX "INTERNAL ERROR: Cannot set external data to fragment already containing in buffer data!");
        return 1;
    }

    switch (ptr2memory_mode(data_free_fnc)) {
        case MEMCPY:
            frag->data = mw_malloc(data_len);
            if (frag->data == NULL) {
                mws_error(log, UNIT_LOG_PREFIX "OOM while malloc @_optimized_add");
                return 1;
            }
            memcpy(frag->data, data, data_len);
            break;
        case EXTERNAL_FREE_AFTER_USE:
        case CALLER_RESPONSIBLE:
            frag->data = data;
            break;
    }
    frag->free_fnc = data_free_fnc;
    frag->len = data_len;

    frag->flags |= BUFFER_FRAG_DATA_EXTERNAL;
    return 0;
 }

// this is fixed part of variable header for connect packet
// mqtt-v5.0-cs1, 3.1.2.1, 2.1.2.2
static const char mqtt_protocol_name_frag[] =
    { 0x00, 0x04, 'M', 'Q', 'T', 'T', MQTT_VERSION_5_0 };

#define MQTT_UTF8_STRING_SIZE(string) (2 + strlen(string))

// see 1.5.5
#define MQTT_VARSIZE_INT_BYTES(value) ( value > 2097152 ? 4 : ( value > 16384 ? 3 : ( value > 128 ? 2 : 1 ) ) )

static size_t mqtt_ng_connect_size(struct mqtt_auth_properties *auth,
                    struct mqtt_lwt_properties *lwt)
{
    // First get the size of payload + variable header
    size_t size =
        + sizeof(mqtt_protocol_name_frag) /* Proto Name and Version */
        + 1 /* Connect Flags */
        + 2 /* Keep Alive */
        + 1 /* 3.1.2.11.1 Property Length - for now 0, TODO TODO*/;

    // CONNECT payload. 3.1.3
    if (auth->client_id)
        size += MQTT_UTF8_STRING_SIZE(auth->client_id);

    if (lwt) {
        // 3.1.3.2 will properties TODO TODO
        size += 1;

        // 3.1.3.3
        if (lwt->will_topic)
            size += MQTT_UTF8_STRING_SIZE(lwt->will_topic);

        // 3.1.3.4 will payload
        if (lwt->will_message) {
            size += 2 + lwt->will_message_size;
        }
    }

    // 3.1.3.5
    if (auth->username)
        size += MQTT_UTF8_STRING_SIZE(auth->username);

    // 3.1.3.6
    if (auth->password)
        size += MQTT_UTF8_STRING_SIZE(auth->password);

    return size;
}

#define BUFFER_TRANSACTION_NEW_FRAG(buf, flags, frag, on_fail) \
    { if(frag==NULL) { \
        frag = buffer_new_frag(buf, (flags)); } \
      if(frag==NULL) { on_fail; }}

#define CHECK_BYTES_AVAILABLE(buf, needed, fail) \
    { if (BUFFER_BYTES_AVAILABLE(buf) < (size_t)needed) { \
        fail; } }

#define DATA_ADVANCE(buf, bytes, frag) { size_t b = (bytes); (buf)->tail += b; (frag)->len += b; }

// TODO maybe just user client->buf.tail?
#define WRITE_POS(frag) (&(frag->data[frag->len]))

// [MQTT-1.5.2] Two Byte Integer
#define PACK_2B_INT(buffer, integer, frag) { *(uint16_t *)WRITE_POS(frag) = htobe16((integer)); \
            DATA_ADVANCE(buffer, sizeof(uint16_t), frag); }

static int _optimized_add(struct header_buffer *buf, mqtt_wss_log_ctx_t log_ctx, void *data, size_t data_len, free_fnc_t data_free_fnc, struct buffer_fragment **frag)
{
    if (data_len > SMALL_STRING_DONT_FRAGMENT_LIMIT) {
        if( (*frag = buffer_new_frag(buf, BUFFER_FRAG_DATA_EXTERNAL)) == NULL ) {
            mws_error(log_ctx, "Out of buffer space while generating the message");
            return 1;
        }
        if (frag_set_external_data(log_ctx, *frag, data, data_len, data_free_fnc)) {
            mws_error(log_ctx, "Error adding external data to newly created fragment");
            return 1;
        }
        // we dont want to write to this fragment anymore
        *frag = NULL;
    } else if (data_len) {
        // if the data are small dont bother creating new fragments
        // store in buffer directly
        CHECK_BYTES_AVAILABLE(buf, data_len, return 1);
        memcpy(buf->tail, data, data_len);
        DATA_ADVANCE(buf, data_len, *frag);
    }
    return 0;
}

#define TRY_GENERATE_MESSAGE(generator_function, buf, log_ctx, max_mem, ...) \
    int rc = generator_function(buf, log_ctx, ##__VA_ARGS__); \
    if (rc == MQTT_NG_MSGGEN_BUFFER_OOM) { \
        LOCK_HDR_BUFFER(buf); \
        transaction_buffer_garbage_collect((buf), log_ctx); \
        UNLOCK_HDR_BUFFER(buf); \
        rc = generator_function(buf, log_ctx, ##__VA_ARGS__); \
        if (rc == MQTT_NG_MSGGEN_BUFFER_OOM && max_mem) { \
            LOCK_HDR_BUFFER(buf); \
            transaction_buffer_grow((buf), log_ctx, GROWTH_FACTOR, max_mem); \
            UNLOCK_HDR_BUFFER(buf); \
            rc = generator_function(buf, log_ctx, ##__VA_ARGS__); \
        } \
        if (rc == MQTT_NG_MSGGEN_BUFFER_OOM) \
            mws_error(log_ctx, "%s failed to generate message due to insufficient buffer space (line %d)", __FUNCTION__, __LINE__); \
    } \
    return rc;

mqtt_msg_data mqtt_ng_generate_connect(struct transaction_buffer *trx_buf,
                                       mqtt_wss_log_ctx_t log_ctx,
                                       struct mqtt_auth_properties *auth,
                                       struct mqtt_lwt_properties *lwt,
                                       uint8_t clean_start,
                                       uint16_t keep_alive)
{
    // Sanity Checks First (are given parameters correct and up to MQTT spec)
    if (!auth->client_id) {
        mws_error(log_ctx, "ClientID must be set. [MQTT-3.1.3-3]");
        return NULL;
    }

    size_t len = strlen(auth->client_id);
    if (!len) {
        // [MQTT-3.1.3-6] server MAY allow empty client_id and treat it
        // as specific client_id (not same as client_id not given)
        // however server MUST allow ClientIDs between 1-23 bytes [MQTT-3.1.3-5]
        // so we will warn client server might not like this and he is using it
        // at his own risk!
        mws_warn(log_ctx, "client_id provided is empty string. This might not be allowed by server [MQTT-3.1.3-6]");
    }
    if(len > MQTT_MAX_CLIENT_ID) {
        // [MQTT-3.1.3-5] server MUST allow client_id length 1-32
        // server MAY allow longer client_id, if user provides longer client_id
        // warn them he is doing so at his own risk!
        mws_warn(log_ctx, "client_id provided is longer than 23 bytes, server might not allow that [MQTT-3.1.3-5]");
    }

    if (lwt) {
        if (lwt->will_message && lwt->will_message_size > 65535) {
            mws_error(log_ctx, "Will message cannot be longer than 65535 bytes due to MQTT protocol limitations [MQTT-3.1.3-4] and [MQTT-1.5.6]");
            return NULL;
        }

        if (!lwt->will_topic) { //TODO topic given with strlen==0 ? check specs
            mws_error(log_ctx, "If will message is given will topic must also be given [MQTT-3.1.3.3]");
            return NULL;
        }

        if (lwt->will_qos > MQTT_MAX_QOS) {
            // refer to [MQTT-3-1.2-12]
            mws_error(log_ctx, "QOS for LWT message is bigger than max");
            return NULL;
        }
    }

    // >> START THE RODEO <<
    transaction_buffer_transaction_start(trx_buf);

    // Calculate the resulting message size sans fixed MQTT header
    size_t size = mqtt_ng_connect_size(auth, lwt);

    // Start generating the message
    struct buffer_fragment *frag = NULL;
    mqtt_msg_data ret = NULL;

    BUFFER_TRANSACTION_NEW_FRAG(&trx_buf->hdr_buffer, BUFFER_FRAG_MQTT_PACKET_HEAD, frag, goto fail_rollback );
    ret = frag;

    // MQTT Fixed Header
    size_t needed_bytes = 1 /* Packet type */ + MQTT_VARSIZE_INT_BYTES(size) + sizeof(mqtt_protocol_name_frag) + 1 /* CONNECT FLAGS */ + 2 /* keepalive */ + 1 /* Properties TODO now fixed 0*/;
    CHECK_BYTES_AVAILABLE(&trx_buf->hdr_buffer, needed_bytes, goto fail_rollback);

    *WRITE_POS(frag) = MQTT_CPT_CONNECT << 4;
    DATA_ADVANCE(&trx_buf->hdr_buffer, 1, frag);
    DATA_ADVANCE(&trx_buf->hdr_buffer, uint32_to_mqtt_vbi(size, WRITE_POS(frag)), frag);

    memcpy(WRITE_POS(frag), mqtt_protocol_name_frag, sizeof(mqtt_protocol_name_frag));
    DATA_ADVANCE(&trx_buf->hdr_buffer, sizeof(mqtt_protocol_name_frag), frag);

    // [MQTT-3.1.2.3] Connect flags
    char *connect_flags = WRITE_POS(frag);
    *connect_flags = 0;
    if (auth->username)
        *connect_flags |= MQTT_CONNECT_FLAG_USERNAME;
    if (auth->password)
        *connect_flags |= MQTT_CONNECT_FLAG_PASSWORD;
    if (lwt) {
        *connect_flags |= MQTT_CONNECT_FLAG_LWT;
        *connect_flags |= lwt->will_qos << MQTT_CONNECT_FLAG_QOS_BITSHIFT;
        if (lwt->will_retain)
            *connect_flags |= MQTT_CONNECT_FLAG_LWT_RETAIN;
    }
    if (clean_start)
        *connect_flags |= MQTT_CONNECT_FLAG_CLEAN_START;

    DATA_ADVANCE(&trx_buf->hdr_buffer, 1, frag);

    PACK_2B_INT(&trx_buf->hdr_buffer, keep_alive, frag);

    // TODO Property Length [MQTT-3.1.3.2.1] temporary fixed 0
    *WRITE_POS(frag) = 0;
    DATA_ADVANCE(&trx_buf->hdr_buffer, 1, frag);

    // [MQTT-3.1.3.1] Client identifier
    CHECK_BYTES_AVAILABLE(&trx_buf->hdr_buffer, 2, goto fail_rollback);
    PACK_2B_INT(&trx_buf->hdr_buffer, strlen(auth->client_id), frag);
    if (_optimized_add(&trx_buf->hdr_buffer, log_ctx, auth->client_id, strlen(auth->client_id), auth->client_id_free, &frag))
        goto fail_rollback;

    if (lwt != NULL) {
        // Will Properties [MQTT-3.1.3.2]
        // TODO for now fixed 0
        BUFFER_TRANSACTION_NEW_FRAG(&trx_buf->hdr_buffer, 0, frag, goto fail_rollback);
        CHECK_BYTES_AVAILABLE(&trx_buf->hdr_buffer, 1, goto fail_rollback);
        *WRITE_POS(frag) = 0;
        DATA_ADVANCE(&trx_buf->hdr_buffer, 1, frag);

        // Will Topic [MQTT-3.1.3.3]
        CHECK_BYTES_AVAILABLE(&trx_buf->hdr_buffer, 2, goto fail_rollback);
        PACK_2B_INT(&trx_buf->hdr_buffer, strlen(lwt->will_topic), frag);
        if (_optimized_add(&trx_buf->hdr_buffer, log_ctx, lwt->will_topic, strlen(lwt->will_topic), lwt->will_topic_free, &frag))
            goto fail_rollback;

        // Will Payload [MQTT-3.1.3.4]
        if (lwt->will_message_size) {
            BUFFER_TRANSACTION_NEW_FRAG(&trx_buf->hdr_buffer, 0, frag, goto fail_rollback);
            CHECK_BYTES_AVAILABLE(&trx_buf->hdr_buffer, 2, goto fail_rollback);
            PACK_2B_INT(&trx_buf->hdr_buffer, lwt->will_message_size, frag);
            if (_optimized_add(&trx_buf->hdr_buffer, log_ctx, lwt->will_message, lwt->will_message_size, lwt->will_topic_free, &frag))
                goto fail_rollback;
        }
    }

    // [MQTT-3.1.3.5]
    if (auth->username) {
        BUFFER_TRANSACTION_NEW_FRAG(&trx_buf->hdr_buffer, 0, frag, goto fail_rollback);
        CHECK_BYTES_AVAILABLE(&trx_buf->hdr_buffer, 2, goto fail_rollback);
        PACK_2B_INT(&trx_buf->hdr_buffer, strlen(auth->username), frag);
        if (_optimized_add(&trx_buf->hdr_buffer, log_ctx, auth->username, strlen(auth->username), auth->username_free, &frag))
            goto fail_rollback;
    }

    // [MQTT-3.1.3.6]
    if (auth->password) {
        BUFFER_TRANSACTION_NEW_FRAG(&trx_buf->hdr_buffer, 0, frag, goto fail_rollback);
        CHECK_BYTES_AVAILABLE(&trx_buf->hdr_buffer, 2, goto fail_rollback);
        PACK_2B_INT(&trx_buf->hdr_buffer, strlen(auth->password), frag);
        if (_optimized_add(&trx_buf->hdr_buffer, log_ctx, auth->password, strlen(auth->password), auth->password_free, &frag))
            goto fail_rollback;
    }
    trx_buf->hdr_buffer.tail_frag->flags |= BUFFER_FRAG_MQTT_PACKET_TAIL;
    transaction_buffer_transaction_commit(trx_buf);
    return ret;
fail_rollback:
    transaction_buffer_transaction_rollback(trx_buf, ret);
    return NULL;
}

int mqtt_ng_connect(struct mqtt_ng_client *client,
                    struct mqtt_auth_properties *auth,
                    struct mqtt_lwt_properties *lwt,
                    uint8_t clean_start,
                    uint16_t keep_alive)
{
    client->client_state = RAW;
    client->parser.state = MQTT_PARSE_FIXED_HEADER_PACKET_TYPE;

    LOCK_HDR_BUFFER(&client->main_buffer);
    client->main_buffer.sending_frag = NULL;

    if (clean_start)
        buffer_purge(&client->main_buffer.hdr_buffer);

    UNLOCK_HDR_BUFFER(&client->main_buffer);

    client->connect_msg = mqtt_ng_generate_connect(&client->main_buffer, client->log, auth, lwt, clean_start, keep_alive);
    if (client->connect_msg == NULL) {
        return 1;
    }

    client->client_state = CONNECT_PENDING;
    return 0;
}

uint16_t get_unused_packet_id() {
    static uint16_t packet_id = 0;
    packet_id++;
    return packet_id ? packet_id : ++packet_id;
}

static inline size_t mqtt_ng_publish_size(const char *topic,
                            size_t msg_len)
{
    return 2 /* Topic Name Length */
        + strlen(topic)
        + 2 /* Packet identifier */
        + 1 /* Properties Length TODO for now fixed 0 */
        + msg_len;
}

int mqtt_ng_generate_publish(struct transaction_buffer *trx_buf,
                             mqtt_wss_log_ctx_t log_ctx,
                             char *topic,
                             free_fnc_t topic_free,
                             void *msg,
                             free_fnc_t msg_free,
                             size_t msg_len,
                             uint8_t publish_flags,
                             uint16_t *packet_id)
{
    // >> START THE RODEO <<
    transaction_buffer_transaction_start(trx_buf);

    // Calculate the resulting message size sans fixed MQTT header
    size_t size = mqtt_ng_publish_size(topic, msg_len);

    // Start generating the message
    struct buffer_fragment *frag = NULL;
    mqtt_msg_data mqtt_msg = NULL;

    BUFFER_TRANSACTION_NEW_FRAG(&trx_buf->hdr_buffer, BUFFER_FRAG_MQTT_PACKET_HEAD, frag, goto fail_rollback );
    mqtt_msg = frag;

    // MQTT Fixed Header
    size_t needed_bytes = 1 /* Packet type */ + MQTT_VARSIZE_INT_BYTES(size) + size - msg_len;
    CHECK_BYTES_AVAILABLE(&trx_buf->hdr_buffer, needed_bytes, goto fail_rollback);

    *WRITE_POS(frag) = (MQTT_CPT_PUBLISH << 4) | (publish_flags & 0xF);
    DATA_ADVANCE(&trx_buf->hdr_buffer, 1, frag);
    DATA_ADVANCE(&trx_buf->hdr_buffer, uint32_to_mqtt_vbi(size, WRITE_POS(frag)), frag);

    // MQTT Variable Header
    // [MQTT-3.3.2.1]
    PACK_2B_INT(&trx_buf->hdr_buffer, strlen(topic), frag);
    if (_optimized_add(&trx_buf->hdr_buffer, log_ctx, topic, strlen(topic), topic_free, &frag))
        goto fail_rollback;
    BUFFER_TRANSACTION_NEW_FRAG(&trx_buf->hdr_buffer, 0, frag, goto fail_rollback);

    // [MQTT-3.3.2.2]
    mqtt_msg->packet_id = get_unused_packet_id();
    *packet_id = mqtt_msg->packet_id;
    PACK_2B_INT(&trx_buf->hdr_buffer, mqtt_msg->packet_id, frag);

    // [MQTT-3.3.2.3.1] TODO Property Length for now fixed 0
    *WRITE_POS(frag) = 0;
    DATA_ADVANCE(&trx_buf->hdr_buffer, 1, frag);

    if( (frag = buffer_new_frag(&trx_buf->hdr_buffer, BUFFER_FRAG_DATA_EXTERNAL)) == NULL )
        goto fail_rollback;

    if (frag_set_external_data(log_ctx, frag, msg, msg_len, msg_free))
        goto fail_rollback;

    trx_buf->hdr_buffer.tail_frag->flags |= BUFFER_FRAG_MQTT_PACKET_TAIL;
    transaction_buffer_transaction_commit(trx_buf);
    return MQTT_NG_MSGGEN_OK;
fail_rollback:
    transaction_buffer_transaction_rollback(trx_buf, mqtt_msg);
    return MQTT_NG_MSGGEN_BUFFER_OOM;
}

int mqtt_ng_publish(struct mqtt_ng_client *client,
                    char *topic,
                    free_fnc_t topic_free,
                    void *msg,
                    free_fnc_t msg_free,
                    size_t msg_len,
                    uint8_t publish_flags,
                    uint16_t *packet_id)
{
    TRY_GENERATE_MESSAGE(mqtt_ng_generate_publish, &client->main_buffer, client->log, client->max_mem_bytes, topic, topic_free, msg, msg_free, msg_len, publish_flags, packet_id);
}

static inline size_t mqtt_ng_subscribe_size(struct mqtt_sub *subs, size_t sub_count)
{
    size_t len = 2 /* Packet Identifier */ + 1 /* Properties Length TODO for now fixed 0 */;
    len += sub_count * (2 /* topic filter string length */ + 1 /* [MQTT-3.8.3.1] Subscription Options Byte */);

    for (size_t i = 0; i < sub_count; i++) {
        len += strlen(subs[i].topic);
    }
    return len;
}

int mqtt_ng_generate_subscribe(struct transaction_buffer *trx_buf, mqtt_wss_log_ctx_t log_ctx, struct mqtt_sub *subs, size_t sub_count)
{
    // >> START THE RODEO <<
    transaction_buffer_transaction_start(trx_buf);

    // Calculate the resulting message size sans fixed MQTT header
    size_t size = mqtt_ng_subscribe_size(subs, sub_count);

    // Start generating the message
    struct buffer_fragment *frag = NULL;
    mqtt_msg_data ret = NULL;

    BUFFER_TRANSACTION_NEW_FRAG(&trx_buf->hdr_buffer, BUFFER_FRAG_MQTT_PACKET_HEAD, frag, goto fail_rollback);
    ret = frag;

    // MQTT Fixed Header
    size_t needed_bytes = 1 /* Packet type */ + MQTT_VARSIZE_INT_BYTES(size) + 3 /*Packet ID + Property Length*/;
    CHECK_BYTES_AVAILABLE(&trx_buf->hdr_buffer, needed_bytes, goto fail_rollback);

    *WRITE_POS(frag) = (MQTT_CPT_SUBSCRIBE << 4) | 0x2 /* [MQTT-3.8.1-1] */;
    DATA_ADVANCE(&trx_buf->hdr_buffer, 1, frag);
    DATA_ADVANCE(&trx_buf->hdr_buffer, uint32_to_mqtt_vbi(size, WRITE_POS(frag)), frag);

    // MQTT Variable Header
    // [MQTT-3.8.2] PacketID
    ret->packet_id = get_unused_packet_id();
    PACK_2B_INT(&trx_buf->hdr_buffer, ret->packet_id, frag);

    // [MQTT-3.8.2.1.1] Property Length // TODO for now fixed 0
    *WRITE_POS(frag) = 0;
    DATA_ADVANCE(&trx_buf->hdr_buffer, 1, frag);

    for (size_t i = 0; i < sub_count; i++) {
        BUFFER_TRANSACTION_NEW_FRAG(&trx_buf->hdr_buffer, 0, frag, goto fail_rollback);
        PACK_2B_INT(&trx_buf->hdr_buffer, strlen(subs[i].topic), frag);
        if (_optimized_add(&trx_buf->hdr_buffer, log_ctx, subs[i].topic, strlen(subs[i].topic), subs[i].topic_free, &frag))
            goto fail_rollback;
        BUFFER_TRANSACTION_NEW_FRAG(&trx_buf->hdr_buffer, 0, frag, goto fail_rollback);
        *WRITE_POS(frag) = subs[i].options;
        DATA_ADVANCE(&trx_buf->hdr_buffer, 1, frag);
    }

    trx_buf->hdr_buffer.tail_frag->flags |= BUFFER_FRAG_MQTT_PACKET_TAIL;
    transaction_buffer_transaction_commit(trx_buf);
    return MQTT_NG_MSGGEN_OK;
fail_rollback:
    transaction_buffer_transaction_rollback(trx_buf, ret);
    return MQTT_NG_MSGGEN_BUFFER_OOM;
}

int mqtt_ng_subscribe(struct mqtt_ng_client *client, struct mqtt_sub *subs, size_t sub_count)
{
    TRY_GENERATE_MESSAGE(mqtt_ng_generate_subscribe, &client->main_buffer, client->log, client->max_mem_bytes, subs, sub_count);
}

int mqtt_ng_generate_disconnect(struct transaction_buffer *trx_buf, mqtt_wss_log_ctx_t log_ctx, uint8_t reason_code)
{
    (void) log_ctx;
    // >> START THE RODEO <<
    transaction_buffer_transaction_start(trx_buf);

    // Calculate the resulting message size sans fixed MQTT header
    size_t size = reason_code ? 1 : 0;

    // Start generating the message
    struct buffer_fragment *frag = NULL;
    mqtt_msg_data ret = NULL;

    BUFFER_TRANSACTION_NEW_FRAG(&trx_buf->hdr_buffer, BUFFER_FRAG_MQTT_PACKET_HEAD, frag, goto fail_rollback);
    ret = frag;

    // MQTT Fixed Header
    size_t needed_bytes = 1 /* Packet type */ + MQTT_VARSIZE_INT_BYTES(size) + (reason_code ? 1 : 0);
    CHECK_BYTES_AVAILABLE(&trx_buf->hdr_buffer, needed_bytes, goto fail_rollback);

    *WRITE_POS(frag) = MQTT_CPT_DISCONNECT << 4;
    DATA_ADVANCE(&trx_buf->hdr_buffer, 1, frag);
    DATA_ADVANCE(&trx_buf->hdr_buffer, uint32_to_mqtt_vbi(size, WRITE_POS(frag)), frag);

    if (reason_code) {
        // MQTT Variable Header
        // [MQTT-3.14.2.1] PacketID
        *WRITE_POS(frag) = reason_code;
        DATA_ADVANCE(&trx_buf->hdr_buffer, 1, frag);
    }

    trx_buf->hdr_buffer.tail_frag->flags |= BUFFER_FRAG_MQTT_PACKET_TAIL;
    transaction_buffer_transaction_commit(trx_buf);
    return MQTT_NG_MSGGEN_OK;
fail_rollback:
    transaction_buffer_transaction_rollback(trx_buf, ret);
    return MQTT_NG_MSGGEN_BUFFER_OOM;
}

int mqtt_ng_disconnect(struct mqtt_ng_client *client, uint8_t reason_code)
{
    TRY_GENERATE_MESSAGE(mqtt_ng_generate_disconnect, &client->main_buffer, client->log, client->max_mem_bytes, reason_code);
}

static int mqtt_generate_puback(struct transaction_buffer *trx_buf, mqtt_wss_log_ctx_t log_ctx, uint16_t packet_id, uint8_t reason_code)
{
    (void) log_ctx;
    // >> START THE RODEO <<
    transaction_buffer_transaction_start(trx_buf);

    // Calculate the resulting message size sans fixed MQTT header
    size_t size = 2 /* Packet ID */ + (reason_code ? 1 : 0) /* reason code */;

    // Start generating the message
    struct buffer_fragment *frag = NULL;

    BUFFER_TRANSACTION_NEW_FRAG(&trx_buf->hdr_buffer, BUFFER_FRAG_MQTT_PACKET_HEAD | BUFFER_FRAG_GARBAGE_COLLECT_ON_SEND, frag, goto fail_rollback);

    // MQTT Fixed Header
    size_t needed_bytes = 1 /* Packet type */ + MQTT_VARSIZE_INT_BYTES(size) + size;
    CHECK_BYTES_AVAILABLE(&trx_buf->hdr_buffer, needed_bytes, goto fail_rollback);

    *WRITE_POS(frag) = MQTT_CPT_PUBACK << 4;
    DATA_ADVANCE(&trx_buf->hdr_buffer, 1, frag);
    DATA_ADVANCE(&trx_buf->hdr_buffer, uint32_to_mqtt_vbi(size, WRITE_POS(frag)), frag);

    // MQTT Variable Header
    PACK_2B_INT(&trx_buf->hdr_buffer, packet_id, frag);

    if (reason_code) {
        // MQTT Variable Header
        // [MQTT-3.14.2.1] PacketID
        *WRITE_POS(frag) = reason_code;
        DATA_ADVANCE(&trx_buf->hdr_buffer, 1, frag);
    }

    trx_buf->hdr_buffer.tail_frag->flags |= BUFFER_FRAG_MQTT_PACKET_TAIL;
    transaction_buffer_transaction_commit(trx_buf);
    return MQTT_NG_MSGGEN_OK;
fail_rollback:
    transaction_buffer_transaction_rollback(trx_buf, frag);
    return MQTT_NG_MSGGEN_BUFFER_OOM;
}

static int mqtt_ng_puback(struct mqtt_ng_client *client, uint16_t packet_id, uint8_t reason_code)
{
    TRY_GENERATE_MESSAGE(mqtt_generate_puback, &client->main_buffer, client->log, client->max_mem_bytes, packet_id, reason_code);
}

int mqtt_ng_ping(struct mqtt_ng_client *client)
{
    client->ping_pending = 1;
    return MQTT_NG_MSGGEN_OK;
}

#define MQTT_NG_CLIENT_NEED_MORE_BYTES         0x10
#define MQTT_NG_CLIENT_MQTT_PACKET_DONE        0x11
#define MQTT_NG_CLIENT_PARSE_DONE              0x12
#define MQTT_NG_CLIENT_WANT_WRITE              0x13
#define MQTT_NG_CLIENT_OK_CALL_AGAIN           0
#define MQTT_NG_CLIENT_PROTOCOL_ERROR         -1
#define MQTT_NG_CLIENT_SERVER_RETURNED_ERROR  -2
#define MQTT_NG_CLIENT_NOT_IMPL_YET           -3
#define MQTT_NG_CLIENT_OOM                    -4
#define MQTT_NG_CLIENT_INTERNAL_ERROR         -5

#define BUF_READ_CHECK_AT_LEAST(buf, x)                 \
    if (rbuf_bytes_available(buf) < (x)) \
        return MQTT_NG_CLIENT_NEED_MORE_BYTES;

#define vbi_parser_reset_ctx(ctx) memset(ctx, 0, sizeof(struct mqtt_vbi_parser_ctx))

static int vbi_parser_parse(struct mqtt_vbi_parser_ctx *ctx, rbuf_t data, mqtt_wss_log_ctx_t log)
{
    if (ctx->bytes > MQTT_VBI_MAXBYTES - 1) {
        mws_error(log, "MQTT Variable Byte Integer can't be longer than %d bytes", MQTT_VBI_MAXBYTES);
        return MQTT_NG_CLIENT_PROTOCOL_ERROR;
    }
    if (!ctx->bytes || ctx->data[ctx->bytes-1] & MQTT_VBI_CONTINUATION_FLAG) {
        BUF_READ_CHECK_AT_LEAST(data, 1);
        ctx->bytes++;
        rbuf_pop(data, &ctx->data[ctx->bytes-1], 1);
        if ( ctx->data[ctx->bytes-1] & MQTT_VBI_CONTINUATION_FLAG )
            return MQTT_NG_CLIENT_OK_CALL_AGAIN;
    }

    if (mqtt_vbi_to_uint32(ctx->data, &ctx->result)) {
            mws_error(log, "MQTT Variable Byte Integer failed to be parsed.");
            return MQTT_NG_CLIENT_PROTOCOL_ERROR;
    }

    return MQTT_NG_CLIENT_PARSE_DONE;
}

static void mqtt_properties_parser_ctx_reset(struct mqtt_properties_parser_ctx *ctx)
{
    ctx->state = PROPERTIES_LENGTH;
    ctx->head = NULL;
    ctx->properties_length = 0;
    ctx->bytes_consumed = 0;
    vbi_parser_reset_ctx(&ctx->vbi_parser_ctx);
}

// Parses [MQTT-2.2.2]
static int parse_properties_array(struct mqtt_properties_parser_ctx *ctx, rbuf_t data, mqtt_wss_log_ctx_t log)
{
    int rc;
    switch (ctx->state) {
        case PROPERTIES_LENGTH:
            rc = vbi_parser_parse(&ctx->vbi_parser_ctx, data, log);
            if (rc == MQTT_NG_CLIENT_PARSE_DONE) {
                ctx->properties_length = ctx->vbi_parser_ctx.result;
                ctx->bytes_consumed += ctx->vbi_parser_ctx.bytes;
                if (!ctx->properties_length)
                    return MQTT_NG_CLIENT_PARSE_DONE;
                ctx->state = PROPERTY_ID;
                vbi_parser_reset_ctx(&ctx->vbi_parser_ctx);
                break;
            }
            return rc;
        case PROPERTY_ID:
            // TODO ignore for now... just skip
            rbuf_bump_tail(data, ctx->properties_length);
            ctx->bytes_consumed += ctx->properties_length;
            return MQTT_NG_CLIENT_PARSE_DONE;
//            rc = vbi_parser_parse(&ctx->vbi_parser_ctx, data, log);
    }
    return MQTT_NG_CLIENT_OK_CALL_AGAIN;
}

static int parse_connack_varhdr(struct mqtt_ng_client *client)
{
    struct mqtt_ng_parser *parser = &client->parser;
    switch (parser->varhdr_state) {
        case MQTT_PARSE_VARHDR_INITIAL:
            BUF_READ_CHECK_AT_LEAST(parser->received_data, 2);
            rbuf_pop(parser->received_data, (char*)&parser->mqtt_packet.connack.flags, 1);
            rbuf_pop(parser->received_data, (char*)&parser->mqtt_packet.connack.reason_code, 1);
            parser->varhdr_state = MQTT_PARSE_VARHDR_PROPS;
            mqtt_properties_parser_ctx_reset(&parser->properties_parser);
            break;
        case MQTT_PARSE_VARHDR_PROPS:
            return parse_properties_array(&parser->properties_parser, parser->received_data, client->log);
        default:
            ERROR("invalid state for connack varhdr parser");
            return MQTT_NG_CLIENT_INTERNAL_ERROR;
    }
    return MQTT_NG_CLIENT_OK_CALL_AGAIN;
}

static int parse_disconnect_varhdr(struct mqtt_ng_client *client)
{
    struct mqtt_ng_parser *parser = &client->parser;
    switch (parser->varhdr_state) {
        case MQTT_PARSE_VARHDR_INITIAL:
            if (!parser->mqtt_fixed_hdr_remaining_length) {
                // [MQTT-3.14.2.1] if reason code omitted act same as == 0
                parser->mqtt_packet.disconnect.reason_code = 0;
                return MQTT_NG_CLIENT_PARSE_DONE;
            }
            BUF_READ_CHECK_AT_LEAST(parser->received_data, 1);
            rbuf_pop(parser->received_data, (char*)&parser->mqtt_packet.connack.reason_code, 1);
            if (parser->mqtt_fixed_hdr_remaining_length == 1)
                return MQTT_NG_CLIENT_PARSE_DONE;
            parser->varhdr_state = MQTT_PARSE_VARHDR_PROPS;
            mqtt_properties_parser_ctx_reset(&parser->properties_parser);
            break;
        case MQTT_PARSE_VARHDR_PROPS:
            return parse_properties_array(&parser->properties_parser, parser->received_data, client->log);
        default:
            ERROR("invalid state for connack varhdr parser");
            return MQTT_NG_CLIENT_INTERNAL_ERROR;
    }
    return MQTT_NG_CLIENT_OK_CALL_AGAIN;
}

static int parse_puback_varhdr(struct mqtt_ng_client *client)
{
    struct mqtt_ng_parser *parser = &client->parser;
    switch (parser->varhdr_state) {
        case MQTT_PARSE_VARHDR_INITIAL:
            BUF_READ_CHECK_AT_LEAST(parser->received_data, 2);
            rbuf_pop(parser->received_data, (char*)&parser->mqtt_packet.puback.packet_id, 2);
            parser->mqtt_packet.puback.packet_id = be16toh(parser->mqtt_packet.puback.packet_id);
            if (parser->mqtt_fixed_hdr_remaining_length < 3) {
                // [MQTT-3.4.2.1] if length is not big enough for reason code
                // it is omitted and handled same as if it was present and == 0
                // initially missed this detail and was wondering WTF is going on (sigh)
                parser->mqtt_packet.puback.reason_code = 0;
                return MQTT_NG_CLIENT_PARSE_DONE;
            }
            parser->varhdr_state = MQTT_PARSE_VARHDR_OPTIONAL_REASON_CODE;
            /* FALLTHROUGH */
        case MQTT_PARSE_VARHDR_OPTIONAL_REASON_CODE:
            BUF_READ_CHECK_AT_LEAST(parser->received_data, 1);
            rbuf_pop(parser->received_data, (char*)&parser->mqtt_packet.puback.reason_code, 1);
            // LOL so in CONNACK you have to have 0 byte to
            // signify empty properties list
            // but in PUBACK it can be omitted if remaining length doesn't allow it (sigh)
            if (parser->mqtt_fixed_hdr_remaining_length < 4)
                return MQTT_NG_CLIENT_PARSE_DONE;

            parser->varhdr_state = MQTT_PARSE_VARHDR_PROPS;
            mqtt_properties_parser_ctx_reset(&parser->properties_parser);
            /* FALLTHROUGH */
        case MQTT_PARSE_VARHDR_PROPS:
            return parse_properties_array(&parser->properties_parser, parser->received_data, client->log);
        default:
            ERROR("invalid state for puback varhdr parser");
            return MQTT_NG_CLIENT_INTERNAL_ERROR;
    }
    return MQTT_NG_CLIENT_OK_CALL_AGAIN;
}

static int parse_suback_varhdr(struct mqtt_ng_client *client)
{
    int rc;
    size_t avail;
    struct mqtt_ng_parser *parser = &client->parser;
    struct mqtt_suback *suback = &client->parser.mqtt_packet.suback;
    switch (parser->varhdr_state) {
        case MQTT_PARSE_VARHDR_INITIAL:
            suback->reason_codes = NULL;
            BUF_READ_CHECK_AT_LEAST(parser->received_data, 2);
            rbuf_pop(parser->received_data, (char*)&suback->packet_id, 2);
            suback->packet_id = be16toh(suback->packet_id);
            parser->varhdr_state = MQTT_PARSE_VARHDR_PROPS;
            parser->mqtt_parsed_len = 2;
            mqtt_properties_parser_ctx_reset(&parser->properties_parser);
            /* FALLTHROUGH */
        case MQTT_PARSE_VARHDR_PROPS:
           rc = parse_properties_array(&parser->properties_parser, parser->received_data, client->log);
            if (rc != MQTT_NG_CLIENT_PARSE_DONE) 
                return rc;
            parser->mqtt_parsed_len += parser->properties_parser.bytes_consumed;
            suback->reason_code_count = parser->mqtt_fixed_hdr_remaining_length - parser->mqtt_parsed_len;
            suback->reason_codes = mw_calloc(suback->reason_code_count, sizeof(*suback->reason_codes));
            suback->reason_codes_pending = suback->reason_code_count;
            parser->varhdr_state = MQTT_PARSE_REASONCODES;
            /* FALLTHROUGH */
        case MQTT_PARSE_REASONCODES:
            avail = rbuf_bytes_available(parser->received_data);
            if (avail < 1)
                return MQTT_NG_CLIENT_NEED_MORE_BYTES;

            suback->reason_codes_pending -= rbuf_pop(parser->received_data, (char*)suback->reason_codes, MIN(suback->reason_codes_pending, avail));

            if (!suback->reason_codes_pending)
                return MQTT_NG_CLIENT_PARSE_DONE;

            return MQTT_NG_CLIENT_NEED_MORE_BYTES;
        default:
            ERROR("invalid state for suback varhdr parser");
            return MQTT_NG_CLIENT_INTERNAL_ERROR;
    }
    return MQTT_NG_CLIENT_OK_CALL_AGAIN;
}

static int parse_publish_varhdr(struct mqtt_ng_client *client)
{
    int rc;
    struct mqtt_ng_parser *parser = &client->parser;
    struct mqtt_publish *publish = &client->parser.mqtt_packet.publish;
    switch (parser->varhdr_state) {
        case MQTT_PARSE_VARHDR_INITIAL:
            BUF_READ_CHECK_AT_LEAST(parser->received_data, 2);
            publish->qos = ((parser->mqtt_control_packet_type >> 1) & 0x03);
            rbuf_pop(parser->received_data, (char*)&publish->topic_len, 2);
            publish->topic_len = be16toh(publish->topic_len);
            publish->topic = mw_calloc(1, publish->topic_len + 1 /* add 0x00 */);
            if (publish->topic == NULL)
                return MQTT_NG_CLIENT_OOM;
            parser->varhdr_state = MQTT_PARSE_VARHDR_TOPICNAME;
            parser->mqtt_parsed_len = 2;
            /* FALLTHROUGH */
        case MQTT_PARSE_VARHDR_TOPICNAME:
            // TODO check empty topic can be valid? In which case we have to skip this step
            BUF_READ_CHECK_AT_LEAST(parser->received_data, publish->topic_len);
            rbuf_pop(parser->received_data, publish->topic, publish->topic_len);
            parser->mqtt_parsed_len += publish->topic_len;
            mqtt_properties_parser_ctx_reset(&parser->properties_parser);
            if (!publish->qos) { // PacketID present only for QOS > 0 [MQTT-3.3.2.2]
                parser->varhdr_state = MQTT_PARSE_VARHDR_PROPS;
                break;
            }
            parser->varhdr_state = MQTT_PARSE_VARHDR_PACKET_ID;
            /* FALLTHROUGH */
        case MQTT_PARSE_VARHDR_PACKET_ID:
            BUF_READ_CHECK_AT_LEAST(parser->received_data, 2);
            rbuf_pop(parser->received_data, (char*)&publish->packet_id, 2);
            publish->packet_id = be16toh(publish->packet_id);
            parser->varhdr_state = MQTT_PARSE_VARHDR_PROPS;
            parser->mqtt_parsed_len += 2;
            /* FALLTHROUGH */
        case MQTT_PARSE_VARHDR_PROPS:
            rc = parse_properties_array(&parser->properties_parser, parser->received_data, client->log);
            if (rc != MQTT_NG_CLIENT_PARSE_DONE) 
                return rc;
            parser->mqtt_parsed_len += parser->properties_parser.bytes_consumed;
            parser->varhdr_state = MQTT_PARSE_PAYLOAD;
            /* FALLTHROUGH */
        case MQTT_PARSE_PAYLOAD:
            if (parser->mqtt_fixed_hdr_remaining_length < parser->mqtt_parsed_len) {
                mw_free(publish->topic);
                publish->topic = NULL;
                ERROR("Error parsing PUBLISH message");
                return MQTT_NG_CLIENT_PROTOCOL_ERROR;
            }
            publish->data_len = parser->mqtt_fixed_hdr_remaining_length - parser->mqtt_parsed_len;
            if (!publish->data_len) {
                publish->data = NULL;
                return MQTT_NG_CLIENT_PARSE_DONE; // 0 length payload is OK [MQTT-3.3.3]
            }
            BUF_READ_CHECK_AT_LEAST(parser->received_data, publish->data_len);

            publish->data = mw_malloc(publish->data_len);
            if (publish->data == NULL) {
                mw_free(publish->topic);
                publish->topic = NULL;
                return MQTT_NG_CLIENT_OOM;
            }

            rbuf_pop(parser->received_data, publish->data, publish->data_len);
            parser->mqtt_parsed_len += publish->data_len;

            return MQTT_NG_CLIENT_PARSE_DONE;
        default:
            ERROR("invalid state for publish varhdr parser");
            return MQTT_NG_CLIENT_INTERNAL_ERROR;
    }
    return MQTT_NG_CLIENT_OK_CALL_AGAIN;
}

// TODO move to separate file, dont send whole client pointer just to be able
// to access LOG context send parser only which should include log
static int parse_data(struct mqtt_ng_client *client)
{
    int rc;
    struct mqtt_ng_parser *parser = &client->parser;
    switch(parser->state) {
        case MQTT_PARSE_FIXED_HEADER_PACKET_TYPE:
            BUF_READ_CHECK_AT_LEAST(parser->received_data, 1);
            rbuf_pop(parser->received_data, (char*)&parser->mqtt_control_packet_type, 1);
            vbi_parser_reset_ctx(&parser->vbi_parser);
            parser->state = MQTT_PARSE_FIXED_HEADER_LEN;
            break;
        case MQTT_PARSE_FIXED_HEADER_LEN:
            rc = vbi_parser_parse(&parser->vbi_parser, parser->received_data, client->log);
            if (rc == MQTT_NG_CLIENT_PARSE_DONE) {
                parser->mqtt_fixed_hdr_remaining_length = parser->vbi_parser.result;
                parser->state = MQTT_PARSE_VARIABLE_HEADER;
                parser->varhdr_state = MQTT_PARSE_VARHDR_INITIAL;
                break;
            }
            return rc;
        case MQTT_PARSE_VARIABLE_HEADER:
            switch (get_control_packet_type(parser->mqtt_control_packet_type)) {
                case MQTT_CPT_CONNACK:
                    rc = parse_connack_varhdr(client);
                    if (rc == MQTT_NG_CLIENT_PARSE_DONE) {
                        parser->state = MQTT_PARSE_MQTT_PACKET_DONE;
                        break;
                    }
                    return rc;
                case MQTT_CPT_PUBACK:
                    rc = parse_puback_varhdr(client);
                    if (rc == MQTT_NG_CLIENT_PARSE_DONE) {
                        parser->state = MQTT_PARSE_MQTT_PACKET_DONE;
                        break;
                    }
                    return rc;
                case MQTT_CPT_SUBACK:
                    rc = parse_suback_varhdr(client);
                    if (rc != MQTT_NG_CLIENT_NEED_MORE_BYTES && rc != MQTT_NG_CLIENT_OK_CALL_AGAIN) {
                        mw_free(parser->mqtt_packet.suback.reason_codes);
                    }
                    if (rc == MQTT_NG_CLIENT_PARSE_DONE) {
                        parser->state = MQTT_PARSE_MQTT_PACKET_DONE;
                        break;
                    }
                    return rc;
                case MQTT_CPT_PUBLISH:
                    rc = parse_publish_varhdr(client);
                    if (rc == MQTT_NG_CLIENT_PARSE_DONE) {
                        parser->state = MQTT_PARSE_MQTT_PACKET_DONE;
                        break;
                    }
                    return rc;
                case MQTT_CPT_PINGRESP:
                    if (parser->mqtt_fixed_hdr_remaining_length) {
                        ERROR ("PINGRESP has to be 0 Remaining Length."); // [MQTT-3.13.1]
                        return MQTT_NG_CLIENT_PROTOCOL_ERROR;
                    }
                    parser->state = MQTT_PARSE_MQTT_PACKET_DONE;
                    break;
                case MQTT_CPT_DISCONNECT:
                    rc = parse_disconnect_varhdr(client);
                    if (rc == MQTT_NG_CLIENT_PARSE_DONE) {
                        parser->state = MQTT_PARSE_MQTT_PACKET_DONE;
                        break;
                    }
                    return rc;
                default:
                    ERROR("Parsing Control Packet Type %" PRIu8 " not implemented yet.", get_control_packet_type(parser->mqtt_control_packet_type));
                    rbuf_bump_tail(parser->received_data, parser->mqtt_fixed_hdr_remaining_length);
                    parser->state = MQTT_PARSE_MQTT_PACKET_DONE;
                    return MQTT_NG_CLIENT_NOT_IMPL_YET;
            }
            // we could also return MQTT_NG_CLIENT_OK_CALL_AGAIN
            // and be called again later
            /* FALLTHROUGH */
        case MQTT_PARSE_MQTT_PACKET_DONE:
            parser->state = MQTT_PARSE_FIXED_HEADER_PACKET_TYPE;
            return MQTT_NG_CLIENT_MQTT_PACKET_DONE;
    }
    return MQTT_NG_CLIENT_OK_CALL_AGAIN;
}

// set next MQTT fragment to send
// return 1 if nothing to send
// return -1 on error
// return 0 if there is fragment set
static int mqtt_ng_next_to_send(struct mqtt_ng_client *client) {
    if (client->client_state == CONNECT_PENDING) {
        client->main_buffer.sending_frag = client->connect_msg;
        client->client_state = CONNECTING;
        return 0;
    }
    if (client->client_state != CONNECTED)
        return -1;

    struct buffer_fragment *frag = BUFFER_FIRST_FRAG(&client->main_buffer.hdr_buffer);
    while (frag) {
        if ( frag->sent != frag->len )
            break;
        frag = frag->next;
    }

    if ( client->ping_pending && (!frag || (frag->flags & BUFFER_FRAG_MQTT_PACKET_HEAD && frag->sent == 0)) ) {
        client->ping_pending = 0;
        ping_frag.sent = 0;
        client->main_buffer.sending_frag = &ping_frag;
        return 0;
    }

    client->main_buffer.sending_frag = frag;
    return frag == NULL ? 1 : 0;
}

// send current fragment
// return 0 if whole remaining length could be sent as a whole
// return -1 if send buffer was filled and
// nothing could be written anymore
// return 1 if last fragment of a message was fully sent
static int send_fragment(struct mqtt_ng_client *client) {
    struct buffer_fragment *frag = client->main_buffer.sending_frag;

    // for readability
    char *ptr = frag->data + frag->sent;
    size_t bytes = frag->len - frag->sent;

    size_t processed = 0;

    if (bytes)
        processed = client->send_fnc_ptr(client->user_ctx, ptr, bytes);
    else
        WARN("This fragment was fully sent already. This should not happen!");

    frag->sent += processed;
    if (frag->sent != frag->len)
        return -1;

    if (frag->flags & BUFFER_FRAG_MQTT_PACKET_TAIL) {
        client->time_of_last_send = time(NULL);
        client->main_buffer.sending_frag = NULL;
        return 1;
    }

    client->main_buffer.sending_frag = frag->next;
    
    return 0;
}

// attempt sending all fragments of current single MQTT packet
static int send_all_message_fragments(struct mqtt_ng_client *client) {
    int rc;
    while ( !(rc = send_fragment(client)) );
    return rc;
}

static void try_send_all(struct mqtt_ng_client *client) {
    do {
        if (client->main_buffer.sending_frag == NULL && mqtt_ng_next_to_send(client))
            return;
    } while(send_all_message_fragments(client) >= 0);
}

static inline void mark_message_for_gc(struct buffer_fragment *frag)
{
    while (frag) {
        frag->flags |= BUFFER_FRAG_GARBAGE_COLLECT;
        buffer_frag_free_data(frag);
        if (frag->flags & BUFFER_FRAG_MQTT_PACKET_TAIL)
            return;
        frag = frag->next;
    }
}

static int mark_packet_acked(struct mqtt_ng_client *client, uint16_t packet_id)
{
    LOCK_HDR_BUFFER(&client->main_buffer);
    struct buffer_fragment *frag = BUFFER_FIRST_FRAG(&client->main_buffer.hdr_buffer);
    while (frag) {
        if ( (frag->flags & BUFFER_FRAG_MQTT_PACKET_HEAD) && frag->packet_id == packet_id) {
            if (!frag->sent) {
                ERROR("Received packet_id (%" PRIu16 ") belongs to MQTT packet which was not yet sent!", packet_id);
                UNLOCK_HDR_BUFFER(&client->main_buffer);
                return 1;
            }
            mark_message_for_gc(frag);
            UNLOCK_HDR_BUFFER(&client->main_buffer);
            return 0;
        }
        frag = frag->next;
    }
    ERROR("Received packet_id (%" PRIu16 ") is unknown!", packet_id);
    UNLOCK_HDR_BUFFER(&client->main_buffer);
    return 1;
}

int handle_incoming_traffic(struct mqtt_ng_client *client)
{
    int rc;
    struct mqtt_publish *pub;
    while( (rc = parse_data(client)) == MQTT_NG_CLIENT_OK_CALL_AGAIN );
    if ( rc == MQTT_NG_CLIENT_MQTT_PACKET_DONE ) {
#ifdef MQTT_DEBUG_VERBOSE
        DEBUG("MQTT Packet Parsed Successfully!");
#endif
        switch (get_control_packet_type(client->parser.mqtt_control_packet_type)) {
            case MQTT_CPT_CONNACK:
#ifdef MQTT_DEBUG_VERBOSE
                DEBUG("Received CONNACK");
#endif
                LOCK_HDR_BUFFER(&client->main_buffer);
                mark_message_for_gc(client->connect_msg);
                UNLOCK_HDR_BUFFER(&client->main_buffer);
                client->connect_msg = NULL;
                if (client->client_state != CONNECTING) {
                    ERROR("Received unexpected CONNACK");
                    client->client_state = ERROR;
                    return MQTT_NG_CLIENT_PROTOCOL_ERROR;
                }
                if (client->connack_callback)
                    client->connack_callback(client->user_ctx, client->parser.mqtt_packet.connack.reason_code);
                if (!client->parser.mqtt_packet.connack.reason_code) {
                    INFO("MQTT Connection Accepted By Server");
                    client->client_state = CONNECTED;
                    break;
                }
                client->client_state = ERROR;
                return MQTT_NG_CLIENT_SERVER_RETURNED_ERROR;
            case MQTT_CPT_PUBACK:
#ifdef MQTT_DEBUG_VERBOSE
                DEBUG("Received PUBACK %" PRIu16, client->parser.mqtt_packet.puback.packet_id);
#endif
                if (mark_packet_acked(client, client->parser.mqtt_packet.puback.packet_id))
                    return MQTT_NG_CLIENT_PROTOCOL_ERROR;
                if (client->puback_callback)
                    client->puback_callback(client->parser.mqtt_packet.puback.packet_id);
                break;
            case MQTT_CPT_PINGRESP:
#ifdef MQTT_DEBUG_VERBOSE
                DEBUG("Received PINGRESP");
#endif
                break;
            case MQTT_CPT_SUBACK:
#ifdef MQTT_DEBUG_VERBOSE
                DEBUG("Received SUBACK %" PRIu16, client->parser.mqtt_packet.suback.packet_id);
#endif
                if (mark_packet_acked(client, client->parser.mqtt_packet.suback.packet_id))
                    return MQTT_NG_CLIENT_PROTOCOL_ERROR;
                break;
            case MQTT_CPT_PUBLISH:
#ifdef MQTT_DEBUG_VERBOSE
                DEBUG("Recevied PUBLISH");
#endif
                pub = &client->parser.mqtt_packet.publish;
                if (pub->qos > 1) {
                    mw_free(pub->topic);
                    mw_free(pub->data);
                    return MQTT_NG_CLIENT_NOT_IMPL_YET;
                }
                if ( pub->qos == 1 && (rc = mqtt_ng_puback(client, pub->packet_id, 0)) ) {
                    client->client_state = ERROR;
                    ERROR("Error generating PUBACK reply for PUBLISH");
                    return rc;
                }
                if (client->msg_callback)
                    client->msg_callback(pub->topic, pub->data, pub->data_len, pub->qos);
                mw_free(pub->topic);
                mw_free(pub->data);
                return MQTT_NG_CLIENT_WANT_WRITE;
            case MQTT_CPT_DISCONNECT:
                INFO ("Got MQTT DISCONNECT control packet from server. Reason code: %d", (int)client->parser.mqtt_packet.disconnect.reason_code);
                client->client_state = DISCONNECTED;
                break;
        }
    }

    return rc;
}

int mqtt_ng_sync(struct mqtt_ng_client *client)
{
    if (client->client_state == RAW || client->client_state == DISCONNECTED)
        return 0;
    
    if (client->client_state == ERROR)
        return 1;

    LOCK_HDR_BUFFER(&client->main_buffer);
    try_send_all(client);
    UNLOCK_HDR_BUFFER(&client->main_buffer);

    int rc;

    while ((rc = handle_incoming_traffic(client)) != MQTT_NG_CLIENT_NEED_MORE_BYTES) {
        if (rc < 0)
            break;
        if (rc == MQTT_NG_CLIENT_WANT_WRITE) {
            LOCK_HDR_BUFFER(&client->main_buffer);
            try_send_all(client);
            UNLOCK_HDR_BUFFER(&client->main_buffer);
        }
    }

    if (rc < 0)
        return rc;

    return 0;
}

time_t mqtt_ng_last_send_time(struct mqtt_ng_client *client)
{
    return client->time_of_last_send;
}

void mqtt_ng_set_max_mem(struct mqtt_ng_client *client, size_t bytes)
{
    client->max_mem_bytes = bytes;
}
