// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDPUSH_H
#define NETDATA_RRDPUSH_H 1

#include "libnetdata/libnetdata.h"
#include "daemon/common.h"
#include "web/server/web_client.h"
#include "database/rrd.h"

#define CONNECTED_TO_SIZE 100
#define CBUFFER_INITIAL_SIZE (16 * 1024)
#define THREAD_BUFFER_INITIAL_SIZE (CBUFFER_INITIAL_SIZE / 2)

// ----------------------------------------------------------------------------
// obsolete versions - do not use anymore

#define STREAM_OLD_VERSION_CLAIM 3
#define STREAM_OLD_VERSION_CLABELS 4
#define STREAM_OLD_VERSION_COMPRESSION 5 // this is production

// ----------------------------------------------------------------------------
// capabilities negotiation

typedef enum {
    // do not use the first 3 bits
    // they used to be versions 1, 2 and 3
    // before we introduce capabilities

    STREAM_CAP_V1               = (1 << 3), // v1 = the oldest protocol
    STREAM_CAP_V2               = (1 << 4), // v2 = the second version of the protocol (with host labels)
    STREAM_CAP_VN               = (1 << 5), // version negotiation supported (for versions 3, 4, 5 of the protocol)
                                            // v3 = claiming supported
                                            // v4 = chart labels supported
                                            // v5 = lz4 compression supported
    STREAM_CAP_VCAPS            = (1 << 6), // capabilities negotiation supported
    STREAM_CAP_HLABELS          = (1 << 7), // host labels supported
    STREAM_CAP_CLAIM            = (1 << 8), // claiming supported
    STREAM_CAP_CLABELS          = (1 << 9), // chart labels supported
    STREAM_CAP_COMPRESSION      = (1 << 10), // lz4 compression supported
    STREAM_CAP_FUNCTIONS        = (1 << 11), // plugin functions supported
    STREAM_CAP_REPLICATION      = (1 << 12), // replication supported
    STREAM_CAP_BINARY           = (1 << 13), // streaming supports binary data
    STREAM_CAP_INTERPOLATED     = (1 << 14), // streaming supports interpolated streaming of values
    STREAM_CAP_IEEE754          = (1 << 15), // streaming supports binary/hex transfer of double values
    STREAM_CAP_DATA_WITH_ML     = (1 << 16), // streaming supports transferring anomaly bit

    STREAM_CAP_INVALID          = (1 << 30), // used as an invalid value for capabilities when this is set
    // this must be signed int, so don't use the last bit
    // needed for negotiating errors between parent and child
} STREAM_CAPABILITIES;

#ifdef  ENABLE_RRDPUSH_COMPRESSION
#define STREAM_HAS_COMPRESSION STREAM_CAP_COMPRESSION
#else
#define STREAM_HAS_COMPRESSION 0
#endif  // ENABLE_RRDPUSH_COMPRESSION

STREAM_CAPABILITIES stream_our_capabilities(RRDHOST *host, bool sender);

#define stream_has_capability(rpt, capability) ((rpt) && ((rpt)->capabilities & (capability)) == (capability))

// ----------------------------------------------------------------------------
// stream handshake

#define HTTP_HEADER_SIZE 8192

#define STREAMING_PROTOCOL_VERSION "1.1"
#define START_STREAMING_PROMPT_V1 "Hit me baby, push them over..."
#define START_STREAMING_PROMPT_V2 "Hit me baby, push them over and bring the host labels..."
#define START_STREAMING_PROMPT_VN "Hit me baby, push them over with the version="

#define START_STREAMING_ERROR_SAME_LOCALHOST "Don't hit me baby, you are trying to stream my localhost back"
#define START_STREAMING_ERROR_ALREADY_STREAMING "This GUID is already streaming to this server"
#define START_STREAMING_ERROR_NOT_PERMITTED "You are not permitted to access this. Check the logs for more info."
#define START_STREAMING_ERROR_BUSY_TRY_LATER "The server is too busy now to accept this request. Try later."
#define START_STREAMING_ERROR_INTERNAL_ERROR "The server encountered an internal error. Try later."
#define START_STREAMING_ERROR_INITIALIZATION "The server is initializing. Try later."

typedef enum {
    STREAM_HANDSHAKE_OK_V3 = 3, // v3+
    STREAM_HANDSHAKE_OK_V2 = 2, // v2
    STREAM_HANDSHAKE_OK_V1 = 1, // v1
    STREAM_HANDSHAKE_NEVER = 0, // never tried to connect
    STREAM_HANDSHAKE_ERROR_BAD_HANDSHAKE = -1,
    STREAM_HANDSHAKE_ERROR_LOCALHOST = -2,
    STREAM_HANDSHAKE_ERROR_ALREADY_CONNECTED = -3,
    STREAM_HANDSHAKE_ERROR_DENIED = -4,
    STREAM_HANDSHAKE_ERROR_SEND_TIMEOUT = -5,
    STREAM_HANDSHAKE_ERROR_RECEIVE_TIMEOUT = -6,
    STREAM_HANDSHAKE_ERROR_INVALID_CERTIFICATE = -7,
    STREAM_HANDSHAKE_ERROR_SSL_ERROR = -8,
    STREAM_HANDSHAKE_ERROR_CANT_CONNECT = -9,
    STREAM_HANDSHAKE_BUSY_TRY_LATER = -10,
    STREAM_HANDSHAKE_INTERNAL_ERROR = -11,
    STREAM_HANDSHAKE_INITIALIZATION = -12,
    STREAM_HANDSHAKE_DISCONNECT_HOST_CLEANUP = -13,
    STREAM_HANDSHAKE_DISCONNECT_STALE_RECEIVER = -14,
    STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN = -15,
    STREAM_HANDSHAKE_DISCONNECT_NETDATA_EXIT = -16,
    STREAM_HANDSHAKE_DISCONNECT_PARSER_EXIT = -17,
    STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_ERROR = -18,
    STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED = -19,
    STREAM_HANDSHAKE_DISCONNECT_RECEIVER_LEFT = -20,
    STREAM_HANDSHAKE_DISCONNECT_ORPHAN_HOST = -21,
    STREAM_HANDSHAKE_NON_STREAMABLE_HOST = -22,

} STREAM_HANDSHAKE;


// ----------------------------------------------------------------------------

typedef struct {
    char *os_name;
    char *os_id;
    char *os_version;
    char *kernel_name;
    char *kernel_version;
} stream_encoded_t;

#ifdef ENABLE_RRDPUSH_COMPRESSION
// signature MUST end with a newline
#define RRDPUSH_COMPRESSION_SIGNATURE ((uint32_t)('z' | 0x80) | (0x80 << 8) | (0x80 << 16) | ('\n' << 24))
#define RRDPUSH_COMPRESSION_SIGNATURE_MASK ((uint32_t)0xff | (0x80 << 8) | (0x80 << 16) | (0xff << 24))
#define RRDPUSH_COMPRESSION_SIGNATURE_SIZE 4

struct compressor_state {
    bool initialized;
    char *compression_result_buffer;
    size_t compression_result_buffer_size;
    struct {
        void *lz4_stream;
        char *input_ring_buffer;
        size_t input_ring_buffer_size;
        size_t input_ring_buffer_pos;
    } stream;
    size_t (*compress)(struct compressor_state *state, const char *data, size_t size, char **buffer);
    void (*destroy)(struct compressor_state **state);
};

void rrdpush_compressor_reset(struct compressor_state *state);
void rrdpush_compressor_destroy(struct compressor_state *state);
size_t rrdpush_compress(struct compressor_state *state, const char *data, size_t size, char **out);

struct decompressor_state {
    bool initialized;
    size_t signature_size;
    size_t total_compressed;
    size_t total_uncompressed;
    size_t packet_count;
    struct {
        void *lz4_stream;
        char *buffer;
        size_t size;
        size_t write_at;
        size_t read_at;
    } stream;
};

void rrdpush_decompressor_destroy(struct decompressor_state *state);
void rrdpush_decompressor_reset(struct decompressor_state *state);
size_t rrdpush_decompress(struct decompressor_state *state, const char *compressed_data, size_t compressed_size);

static inline size_t rrdpush_decompress_decode_header(const char *data, size_t data_size) {
    if (unlikely(!data || !data_size))
        return 0;

    if (unlikely(data_size != RRDPUSH_COMPRESSION_SIGNATURE_SIZE))
        return 0;

    uint32_t sign = *(uint32_t *)data;
    if (unlikely((sign & RRDPUSH_COMPRESSION_SIGNATURE_MASK) != RRDPUSH_COMPRESSION_SIGNATURE))
        return 0;

    size_t length = ((sign >> 8) & 0x7f) | ((sign >> 9) & (0x7f << 7));
    return length;
}

static inline size_t rrdpush_decompressor_start(struct decompressor_state *state, const char *header, size_t header_size) {
    if(unlikely(state->stream.read_at != state->stream.write_at))
        fatal("RRDPUSH DECOMPRESS: asked to decompress new data, while there are unread data in the decompression buffer!");

    return rrdpush_decompress_decode_header(header, header_size);
}

static inline size_t rrdpush_decompressed_bytes_in_buffer(struct decompressor_state *state) {
    if(unlikely(state->stream.read_at > state->stream.write_at))
        fatal("RRDPUSH DECOMPRESS: invalid read/write stream positions");

    return state->stream.write_at - state->stream.read_at;
}

static inline size_t rrdpush_decompressor_get(struct decompressor_state *state, char *dst, size_t size) {
    if (unlikely(!state || !size || !dst))
        return 0;

    size_t remaining = rrdpush_decompressed_bytes_in_buffer(state);

    if(unlikely(!remaining))
        return 0;

    size_t bytes_to_return = size;
    if(bytes_to_return > remaining)
        bytes_to_return = remaining;

    memcpy(dst, state->stream.buffer + state->stream.read_at, bytes_to_return);
    state->stream.read_at += bytes_to_return;

    if(unlikely(state->stream.read_at > state->stream.write_at))
        fatal("RRDPUSH DECOMPRESS: invalid read/write stream positions");

    return bytes_to_return;
}
#endif

// Thread-local storage
// Metric transmission: collector threads asynchronously fill the buffer, sender thread uses it.

typedef enum __attribute__((packed)) {
    STREAM_TRAFFIC_TYPE_REPLICATION = 0,
    STREAM_TRAFFIC_TYPE_FUNCTIONS,
    STREAM_TRAFFIC_TYPE_METADATA,
    STREAM_TRAFFIC_TYPE_DATA,

    // terminator
    STREAM_TRAFFIC_TYPE_MAX,
} STREAM_TRAFFIC_TYPE;

typedef enum __attribute__((packed)) {
    SENDER_FLAG_OVERFLOW     = (1 << 0), // The buffer has been overflown
    SENDER_FLAG_COMPRESSION  = (1 << 1), // The stream needs to have and has compression
} SENDER_FLAGS;

struct sender_state {
    RRDHOST *host;
    pid_t tid;                              // the thread id of the sender, from gettid()
    SENDER_FLAGS flags;
    int timeout;
    int default_port;
    uint32_t reconnect_delay;
    char connected_to[CONNECTED_TO_SIZE + 1];   // We don't know which proxy we connect to, passed back from socket.c
    size_t begin;
    size_t reconnects_counter;
    size_t sent_bytes;
    size_t sent_bytes_on_this_connection;
    size_t send_attempts;
    time_t last_traffic_seen_t;
    time_t last_state_since_t;              // the timestamp of the last state (online/offline) change
    size_t not_connected_loops;
    // Metrics are collected asynchronously by collector threads calling rrdset_done_push(). This can also trigger
    // the lazy creation of the sender thread - both cases (buffer access and thread creation) are guarded here.
    SPINLOCK spinlock;
    struct circular_buffer *buffer;
    char read_buffer[PLUGINSD_LINE_MAX + 1];
    ssize_t read_len;
    STREAM_CAPABILITIES capabilities;

    size_t sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_MAX];

    int rrdpush_sender_pipe[2];                     // collector to sender thread signaling
    int rrdpush_sender_socket;

    uint16_t hops;

#ifdef ENABLE_RRDPUSH_COMPRESSION
    struct compressor_state compressor;
#endif // ENABLE_RRDPUSH_COMPRESSION

#ifdef ENABLE_HTTPS
    NETDATA_SSL ssl;                     // structure used to encrypt the connection
#endif

    struct {
        bool shutdown;
        STREAM_HANDSHAKE reason;
    } exit;

    struct {
        DICTIONARY *requests;                   // de-duplication of replication requests, per chart
        time_t oldest_request_after_t;          // the timestamp of the oldest replication request
        time_t latest_completed_before_t;       // the timestamp of the latest replication request

        struct {
            size_t pending_requests;            // the currently outstanding replication requests
            size_t charts_replicating;          // the number of unique charts having pending replication requests (on every request one is added and is removed when we finish it - it does not track completion of the replication for this chart)
            bool reached_max;                   // true when the sender buffer should not get more replication responses
        } atomic;

    } replication;

    struct {
        bool pending_data;
        size_t buffer_used_percentage;          // the current utilization of the sending buffer
        usec_t last_flush_time_ut;              // the last time the sender flushed the sending buffer in USEC
        time_t last_buffer_recreate_s;          // true when the sender buffer should be re-created
    } atomic;
};

#define sender_lock(sender) spinlock_lock(&(sender)->spinlock)
#define sender_unlock(sender) spinlock_unlock(&(sender)->spinlock)

#define rrdpush_sender_pipe_has_pending_data(sender) __atomic_load_n(&(sender)->atomic.pending_data, __ATOMIC_RELAXED)
#define rrdpush_sender_pipe_set_pending_data(sender) __atomic_store_n(&(sender)->atomic.pending_data, true, __ATOMIC_RELAXED)
#define rrdpush_sender_pipe_clear_pending_data(sender) __atomic_store_n(&(sender)->atomic.pending_data, false, __ATOMIC_RELAXED)

#define rrdpush_sender_last_buffer_recreate_get(sender) __atomic_load_n(&(sender)->atomic.last_buffer_recreate_s, __ATOMIC_RELAXED)
#define rrdpush_sender_last_buffer_recreate_set(sender, value) __atomic_store_n(&(sender)->atomic.last_buffer_recreate_s, value, __ATOMIC_RELAXED)

#define rrdpush_sender_replication_buffer_full_set(sender, value) __atomic_store_n(&((sender)->replication.atomic.reached_max), value, __ATOMIC_SEQ_CST)
#define rrdpush_sender_replication_buffer_full_get(sender) __atomic_load_n(&((sender)->replication.atomic.reached_max), __ATOMIC_SEQ_CST)

#define rrdpush_sender_set_buffer_used_percent(sender, value) __atomic_store_n(&((sender)->atomic.buffer_used_percentage), value, __ATOMIC_RELAXED)
#define rrdpush_sender_get_buffer_used_percent(sender) __atomic_load_n(&((sender)->atomic.buffer_used_percentage), __ATOMIC_RELAXED)

#define rrdpush_sender_set_flush_time(sender) __atomic_store_n(&((sender)->atomic.last_flush_time_ut), now_realtime_usec(), __ATOMIC_RELAXED)
#define rrdpush_sender_get_flush_time(sender) __atomic_load_n(&((sender)->atomic.last_flush_time_ut), __ATOMIC_RELAXED)

#define rrdpush_sender_replicating_charts(sender) __atomic_load_n(&((sender)->replication.atomic.charts_replicating), __ATOMIC_RELAXED)
#define rrdpush_sender_replicating_charts_plus_one(sender) __atomic_add_fetch(&((sender)->replication.atomic.charts_replicating), 1, __ATOMIC_RELAXED)
#define rrdpush_sender_replicating_charts_minus_one(sender) __atomic_sub_fetch(&((sender)->replication.atomic.charts_replicating), 1, __ATOMIC_RELAXED)
#define rrdpush_sender_replicating_charts_zero(sender) __atomic_store_n(&((sender)->replication.atomic.charts_replicating), 0, __ATOMIC_RELAXED)

#define rrdpush_sender_pending_replication_requests(sender) __atomic_load_n(&((sender)->replication.atomic.pending_requests), __ATOMIC_RELAXED)
#define rrdpush_sender_pending_replication_requests_plus_one(sender) __atomic_add_fetch(&((sender)->replication.atomic.pending_requests), 1, __ATOMIC_RELAXED)
#define rrdpush_sender_pending_replication_requests_minus_one(sender) __atomic_sub_fetch(&((sender)->replication.atomic.pending_requests), 1, __ATOMIC_RELAXED)
#define rrdpush_sender_pending_replication_requests_zero(sender) __atomic_store_n(&((sender)->replication.atomic.pending_requests), 0, __ATOMIC_RELAXED)

/*
typedef enum {
    STREAM_NODE_INSTANCE_FEATURE_CLOUD_ONLINE   = (1 << 0),
    STREAM_NODE_INSTANCE_FEATURE_VIRTUAL_HOST   = (1 << 1),
    STREAM_NODE_INSTANCE_FEATURE_HEALTH_ENABLED = (1 << 2),
    STREAM_NODE_INSTANCE_FEATURE_ML_SELF        = (1 << 3),
    STREAM_NODE_INSTANCE_FEATURE_ML_RECEIVED    = (1 << 4),
    STREAM_NODE_INSTANCE_FEATURE_SSL            = (1 << 5),
} STREAM_NODE_INSTANCE_FEATURES;

typedef struct stream_node_instance {
    uuid_t uuid;
    STRING *agent;
    STREAM_NODE_INSTANCE_FEATURES features;
    uint32_t hops;

    // receiver information on that agent
    int32_t capabilities;
    uint32_t local_port;
    uint32_t remote_port;
    STRING *local_ip;
    STRING *remote_ip;
} STREAM_NODE_INSTANCE;
*/

struct buffered_reader {
    ssize_t read_len;
    ssize_t pos;
    char read_buffer[PLUGINSD_LINE_MAX + 1];
};

char *buffered_reader_next_line(struct buffered_reader *reader, char *dst, size_t dst_size);
static inline void buffered_reader_init(struct buffered_reader *reader) {
    reader->read_buffer[0] = '\0';
    reader->read_len = 0;
    reader->pos = 0;
}

struct receiver_state {
    RRDHOST *host;
    pid_t tid;
    netdata_thread_t thread;
    int fd;
    char *key;
    char *hostname;
    char *registry_hostname;
    char *machine_guid;
    char *os;
    char *timezone;         // Unused?
    char *abbrev_timezone;
    int32_t utc_offset;
    char *tags;
    char *client_ip;        // Duplicated in pluginsd
    char *client_port;        // Duplicated in pluginsd
    char *program_name;        // Duplicated in pluginsd
    char *program_version;
    struct rrdhost_system_info *system_info;
    STREAM_CAPABILITIES capabilities;
    time_t last_msg_t;

    struct buffered_reader reader;

    uint16_t hops;

    struct {
        bool shutdown;      // signal the streaming parser to exit
        STREAM_HANDSHAKE reason;
    } exit;

    struct {
        RRD_MEMORY_MODE mode;
        int history;
        int update_every;
        int health_enabled; // CONFIG_BOOLEAN_YES, CONFIG_BOOLEAN_NO, CONFIG_BOOLEAN_AUTO
        time_t alarms_delay;
        uint32_t alarms_history;
        int rrdpush_enabled;
        char *rrdpush_api_key; // DONT FREE - it is allocated in appconfig
        char *rrdpush_send_charts_matching; // DONT FREE - it is allocated in appconfig
        bool rrdpush_enable_replication;
        time_t rrdpush_seconds_to_replicate;
        time_t rrdpush_replication_step;
        char *rrdpush_destination;  // DONT FREE - it is allocated in appconfig
        unsigned int rrdpush_compression;
    } config;

#ifdef ENABLE_HTTPS
    NETDATA_SSL ssl;
#endif

    time_t replication_first_time_t;

#ifdef ENABLE_RRDPUSH_COMPRESSION
    struct decompressor_state decompressor;
#endif // ENABLE_RRDPUSH_COMPRESSION
/*
    struct {
        uint32_t count;
        STREAM_NODE_INSTANCE *array;
    } instances;
*/
};

struct rrdpush_destinations {
    STRING *destination;
    bool ssl;
    uint32_t attempts;
    time_t since;
    time_t postpone_reconnection_until;
    STREAM_HANDSHAKE reason;

    struct rrdpush_destinations *prev;
    struct rrdpush_destinations *next;
};

extern unsigned int default_rrdpush_enabled;
#ifdef ENABLE_RRDPUSH_COMPRESSION
extern unsigned int default_rrdpush_compression_enabled;
#endif // ENABLE_RRDPUSH_COMPRESSION
extern char *default_rrdpush_destination;
extern char *default_rrdpush_api_key;
extern char *default_rrdpush_send_charts_matching;
extern bool default_rrdpush_enable_replication;
extern time_t default_rrdpush_seconds_to_replicate;
extern time_t default_rrdpush_replication_step;
extern unsigned int remote_clock_resync_iterations;

void rrdpush_destinations_init(RRDHOST *host);
void rrdpush_destinations_free(RRDHOST *host);

BUFFER *sender_start(struct sender_state *s);
void sender_commit(struct sender_state *s, BUFFER *wb, STREAM_TRAFFIC_TYPE type);
int rrdpush_init();
bool rrdpush_receiver_needs_dbengine();
int configured_as_parent();

typedef struct rrdset_stream_buffer {
    STREAM_CAPABILITIES capabilities;
    bool v2;
    bool begin_v2_added;
    time_t wall_clock_time;
    uint64_t rrdset_flags; // RRDSET_FLAGS
    time_t last_point_end_time_s;
    BUFFER *wb;
} RRDSET_STREAM_BUFFER;

RRDSET_STREAM_BUFFER rrdset_push_metric_initialize(RRDSET *st, time_t wall_clock_time);
void rrdset_push_metrics_v1(RRDSET_STREAM_BUFFER *rsb, RRDSET *st);
void rrdset_push_metrics_finished(RRDSET_STREAM_BUFFER *rsb, RRDSET *st);
void rrddim_push_metrics_v2(RRDSET_STREAM_BUFFER *rsb, RRDDIM *rd, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags);

bool rrdset_push_chart_definition_now(RRDSET *st);
void *rrdpush_sender_thread(void *ptr);
void rrdpush_send_host_labels(RRDHOST *host);
void rrdpush_send_claimed_id(RRDHOST *host);
void rrdpush_send_global_functions(RRDHOST *host);

#define THREAD_TAG_STREAM_RECEIVER "RCVR" // "[host]" is appended
#define THREAD_TAG_STREAM_SENDER "SNDR" // "[host]" is appended

int rrdpush_receiver_thread_spawn(struct web_client *w, char *decoded_query_string);
void rrdpush_sender_thread_stop(RRDHOST *host, STREAM_HANDSHAKE reason, bool wait);

void rrdpush_sender_send_this_host_variable_now(RRDHOST *host, const RRDVAR_ACQUIRED *rva);
void log_stream_connection(const char *client_ip, const char *client_port, const char *api_key, const char *machine_guid, const char *host, const char *msg);
int connect_to_one_of_destinations(
    RRDHOST *host,
    int default_port,
    struct timeval *timeout,
    size_t *reconnects_counter,
    char *connected_to,
    size_t connected_to_size,
    struct rrdpush_destinations **destination);

void rrdpush_signal_sender_to_wake_up(struct sender_state *s);

#ifdef ENABLE_RRDPUSH_COMPRESSION
struct compressor_state *create_compressor();
#endif // ENABLE_RRDPUSH_COMPRESSION

void rrdpush_reset_destinations_postpone_time(RRDHOST *host);
const char *stream_handshake_error_to_string(STREAM_HANDSHAKE handshake_error);
void stream_capabilities_to_json_array(BUFFER *wb, STREAM_CAPABILITIES caps, const char *key);
void rrdpush_receive_log_status(struct receiver_state *rpt, const char *msg, const char *status);
void log_receiver_capabilities(struct receiver_state *rpt);
void log_sender_capabilities(struct sender_state *s);
STREAM_CAPABILITIES convert_stream_version_to_capabilities(int32_t version, RRDHOST *host, bool sender);
int32_t stream_capabilities_to_vn(uint32_t caps);

void receiver_state_free(struct receiver_state *rpt);
bool stop_streaming_receiver(RRDHOST *host, STREAM_HANDSHAKE reason);

void sender_thread_buffer_free(void);

#include "replication.h"

typedef enum __attribute__((packed)) {
    RRDHOST_DB_STATUS_INITIALIZING = 0,
    RRDHOST_DB_STATUS_QUERYABLE,
} RRDHOST_DB_STATUS;

static inline const char *rrdhost_db_status_to_string(RRDHOST_DB_STATUS status) {
    switch(status) {
        default:
        case RRDHOST_DB_STATUS_INITIALIZING:
            return "initializing";

        case RRDHOST_DB_STATUS_QUERYABLE:
            return "online";
    }
}

typedef enum __attribute__((packed)) {
    RRDHOST_DB_LIVENESS_STALE = 0,
    RRDHOST_DB_LIVENESS_LIVE,
} RRDHOST_DB_LIVENESS;

static inline const char *rrdhost_db_liveness_to_string(RRDHOST_DB_LIVENESS status) {
    switch(status) {
        default:
        case RRDHOST_DB_LIVENESS_STALE:
            return "stale";

        case RRDHOST_DB_LIVENESS_LIVE:
            return "live";
    }
}

typedef enum __attribute__((packed)) {
    RRDHOST_INGEST_STATUS_ARCHIVED = 0,
    RRDHOST_INGEST_STATUS_INITIALIZING,
    RRDHOST_INGEST_STATUS_REPLICATING,
    RRDHOST_INGEST_STATUS_ONLINE,
    RRDHOST_INGEST_STATUS_OFFLINE,
} RRDHOST_INGEST_STATUS;

static inline const char *rrdhost_ingest_status_to_string(RRDHOST_INGEST_STATUS status) {
    switch(status) {
        case RRDHOST_INGEST_STATUS_ARCHIVED:
            return "archived";

        case RRDHOST_INGEST_STATUS_INITIALIZING:
            return "initializing";

        case RRDHOST_INGEST_STATUS_REPLICATING:
            return "replicating";

        case RRDHOST_INGEST_STATUS_ONLINE:
            return "online";

        default:
        case RRDHOST_INGEST_STATUS_OFFLINE:
            return "offline";
    }
}

typedef enum __attribute__((packed)) {
    RRDHOST_INGEST_TYPE_LOCALHOST = 0,
    RRDHOST_INGEST_TYPE_VIRTUAL,
    RRDHOST_INGEST_TYPE_CHILD,
    RRDHOST_INGEST_TYPE_ARCHIVED,
} RRDHOST_INGEST_TYPE;

static inline const char *rrdhost_ingest_type_to_string(RRDHOST_INGEST_TYPE type) {
    switch(type) {
        case RRDHOST_INGEST_TYPE_LOCALHOST:
            return "localhost";

        case RRDHOST_INGEST_TYPE_VIRTUAL:
            return "virtual";

        case RRDHOST_INGEST_TYPE_CHILD:
            return "child";

        default:
        case RRDHOST_INGEST_TYPE_ARCHIVED:
            return "archived";
    }
}

typedef enum __attribute__((packed)) {
    RRDHOST_STREAM_STATUS_DISABLED = 0,
    RRDHOST_STREAM_STATUS_REPLICATING,
    RRDHOST_STREAM_STATUS_ONLINE,
    RRDHOST_STREAM_STATUS_OFFLINE,
} RRDHOST_STREAMING_STATUS;

static inline const char *rrdhost_streaming_status_to_string(RRDHOST_STREAMING_STATUS status) {
    switch(status) {
        case RRDHOST_STREAM_STATUS_DISABLED:
            return "disabled";

        case RRDHOST_STREAM_STATUS_REPLICATING:
            return "replicating";

        case RRDHOST_STREAM_STATUS_ONLINE:
            return "online";

        default:
        case RRDHOST_STREAM_STATUS_OFFLINE:
            return "offline";
    }
}

typedef enum __attribute__((packed)) {
    RRDHOST_ML_STATUS_DISABLED = 0,
    RRDHOST_ML_STATUS_OFFLINE,
    RRDHOST_ML_STATUS_RUNNING,
} RRDHOST_ML_STATUS;

static inline const char *rrdhost_ml_status_to_string(RRDHOST_ML_STATUS status) {
    switch(status) {
        case RRDHOST_ML_STATUS_RUNNING:
            return "online";

        case RRDHOST_ML_STATUS_OFFLINE:
            return "offline";

        default:
        case RRDHOST_ML_STATUS_DISABLED:
            return "disabled";
    }
}

typedef enum __attribute__((packed)) {
    RRDHOST_ML_TYPE_DISABLED = 0,
    RRDHOST_ML_TYPE_SELF,
    RRDHOST_ML_TYPE_RECEIVED,
} RRDHOST_ML_TYPE;

static inline const char *rrdhost_ml_type_to_string(RRDHOST_ML_TYPE type) {
    switch(type) {
        case RRDHOST_ML_TYPE_SELF:
            return "self";

        case RRDHOST_ML_TYPE_RECEIVED:
            return "received";

        default:
        case RRDHOST_ML_TYPE_DISABLED:
            return "disabled";
    }
}

typedef enum __attribute__((packed)) {
    RRDHOST_HEALTH_STATUS_DISABLED = 0,
    RRDHOST_HEALTH_STATUS_INITIALIZING,
    RRDHOST_HEALTH_STATUS_RUNNING,
} RRDHOST_HEALTH_STATUS;

static inline const char *rrdhost_health_status_to_string(RRDHOST_HEALTH_STATUS status) {
    switch(status) {
        default:
        case RRDHOST_HEALTH_STATUS_DISABLED:
            return "disabled";

        case RRDHOST_HEALTH_STATUS_INITIALIZING:
            return "initializing";

        case RRDHOST_HEALTH_STATUS_RUNNING:
            return "online";
    }
}

typedef struct rrdhost_status {
    RRDHOST *host;
    time_t now;

    struct {
        RRDHOST_DB_STATUS status;
        RRDHOST_DB_LIVENESS liveness;
        RRD_MEMORY_MODE mode;
        time_t first_time_s;
        time_t last_time_s;
        size_t metrics;
        size_t instances;
        size_t contexts;
    } db;

    struct {
        RRDHOST_ML_STATUS status;
        RRDHOST_ML_TYPE type;
        struct ml_metrics_statistics metrics;
    } ml;

    struct {
        size_t hops;
        RRDHOST_INGEST_TYPE  type;
        RRDHOST_INGEST_STATUS status;
        SOCKET_PEERS peers;
        bool ssl;
        STREAM_CAPABILITIES capabilities;
        uint32_t id;
        time_t since;
        STREAM_HANDSHAKE reason;

        struct {
            bool in_progress;
            NETDATA_DOUBLE completion;
            size_t instances;
        } replication;
    } ingest;

    struct {
        size_t hops;
        RRDHOST_STREAMING_STATUS status;
        SOCKET_PEERS peers;
        bool ssl;
        bool compression;
        STREAM_CAPABILITIES capabilities;
        uint32_t id;
        time_t since;
        STREAM_HANDSHAKE reason;

        struct {
            bool in_progress;
            NETDATA_DOUBLE completion;
            size_t instances;
        } replication;

        size_t sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_MAX];
    } stream;

    struct {
        RRDHOST_HEALTH_STATUS status;
        struct {
            uint32_t undefined;
            uint32_t uninitialized;
            uint32_t clear;
            uint32_t warning;
            uint32_t critical;
        } alerts;
    } health;
} RRDHOST_STATUS;

void rrdhost_status(RRDHOST *host, time_t now, RRDHOST_STATUS *s);
bool rrdhost_state_cloud_emulation(RRDHOST *host);

#endif //NETDATA_RRDPUSH_H
