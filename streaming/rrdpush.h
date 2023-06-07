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

    STREAM_CAP_INVALID          = (1 << 30), // used as an invalid value for capabilities when this is set
    // this must be signed int, so don't use the last bit
    // needed for negotiating errors between parent and child
} STREAM_CAPABILITIES;

#ifdef  ENABLE_COMPRESSION
#define STREAM_HAS_COMPRESSION STREAM_CAP_COMPRESSION
#else
#define STREAM_HAS_COMPRESSION 0
#endif  // ENABLE_COMPRESSION

STREAM_CAPABILITIES stream_our_capabilities();

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
    STREAM_HANDSHAKE_OK_V5 = 5, // COMPRESSION
    STREAM_HANDSHAKE_OK_V4 = 4, // CLABELS
    STREAM_HANDSHAKE_OK_V3 = 3, // CLAIM
    STREAM_HANDSHAKE_OK_V2 = 2, // HLABELS
    STREAM_HANDSHAKE_OK_V1 = 1,
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
} STREAM_HANDSHAKE;


// ----------------------------------------------------------------------------

typedef enum __attribute__((packed)) {
    STREAM_TRAFFIC_TYPE_REPLICATION,
    STREAM_TRAFFIC_TYPE_FUNCTIONS,
    STREAM_TRAFFIC_TYPE_METADATA,
    STREAM_TRAFFIC_TYPE_DATA,

    // terminator
    STREAM_TRAFFIC_TYPE_MAX,
} STREAM_TRAFFIC_TYPE;

// ----------------------------------------------------------------------------

typedef struct {
    char *os_name;
    char *os_id;
    char *os_version;
    char *kernel_name;
    char *kernel_version;
} stream_encoded_t;

#ifdef ENABLE_COMPRESSION
struct compressor_state {
    char *compression_result_buffer;
    size_t compression_result_buffer_size;
    struct compressor_data *data; // Compression API specific data
    void (*reset)(struct compressor_state *state);
    size_t (*compress)(struct compressor_state *state, const char *data, size_t size, char **buffer);
    void (*destroy)(struct compressor_state **state);
};

struct decompressor_state {
    size_t signature_size;
    size_t total_compressed;
    size_t total_uncompressed;
    size_t packet_count;
    struct decompressor_stream *stream; // Decompression API specific data
    void (*reset)(struct decompressor_state *state);
    size_t (*start)(struct decompressor_state *state, const char *header, size_t header_size);
    size_t (*decompress)(struct decompressor_state *state, const char *compressed_data, size_t compressed_size);
    size_t (*decompressed_bytes_in_buffer)(struct decompressor_state *state);
    size_t (*get)(struct decompressor_state *state, char *data, size_t size);
    void (*destroy)(struct decompressor_state **state);
};
#endif

// Thread-local storage
    // Metric transmission: collector threads asynchronously fill the buffer, sender thread uses it.

typedef enum {
    SENDER_FLAG_OVERFLOW     = (1 << 0), // The buffer has been overflown
    SENDER_FLAG_COMPRESSION  = (1 << 1), // The stream needs to have and has compression
} SENDER_FLAGS;

struct sender_state {
    RRDHOST *host;
    pid_t tid;                              // the thread id of the sender, from gettid()
    SENDER_FLAGS flags;
    int timeout;
    int default_port;
    usec_t reconnect_delay;
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
    netdata_mutex_t mutex;
    struct circular_buffer *buffer;
    char read_buffer[PLUGINSD_LINE_MAX + 1];
    int read_len;
    STREAM_CAPABILITIES capabilities;

    size_t sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_MAX];

    int rrdpush_sender_pipe[2];                     // collector to sender thread signaling
    int rrdpush_sender_socket;

    uint16_t hops;

#ifdef ENABLE_COMPRESSION
    struct compressor_state *compressor;
#endif
#ifdef ENABLE_HTTPS
    NETDATA_SSL ssl;                     // structure used to encrypt the connection
#endif

    struct {
        bool shutdown;
        const char *reason;
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
    char read_buffer[PLUGINSD_LINE_MAX + 1];
    int read_len;

    uint16_t hops;

    struct {
        bool shutdown;      // signal the streaming parser to exit
        const char *reason; // the reason of disconnection to log
    } exit;

    struct {
        RRD_MEMORY_MODE mode;
        int history;
        int update_every;
        int health_enabled; // CONFIG_BOOLEAN_YES, CONFIG_BOOLEAN_NO, CONFIG_BOOLEAN_AUTO
        time_t alarms_delay;
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
#ifdef ENABLE_COMPRESSION
    unsigned int rrdpush_compression;
    struct decompressor_state *decompressor;
#endif

    time_t replication_first_time_t;
};

struct rrdpush_destinations {
    STRING *destination;
    bool ssl;

    const char *last_error;
    time_t last_attempt;
    time_t postpone_reconnection_until;
    STREAM_HANDSHAKE last_handshake;

    struct rrdpush_destinations *prev;
    struct rrdpush_destinations *next;
};

extern unsigned int default_rrdpush_enabled;
#ifdef ENABLE_COMPRESSION
extern unsigned int default_compression_enabled;
#endif
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
void rrdpush_claimed_id(RRDHOST *host);

#define THREAD_TAG_STREAM_RECEIVER "RCVR" // "[host]" is appended
#define THREAD_TAG_STREAM_SENDER "SNDR" // "[host]" is appended

int rrdpush_receiver_thread_spawn(struct web_client *w, char *decoded_query_string);
void rrdpush_sender_thread_stop(RRDHOST *host, const char *reason, bool wait);

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

#ifdef ENABLE_COMPRESSION
struct compressor_state *create_compressor();
struct decompressor_state *create_decompressor();
#endif
void rrdpush_reset_destinations_postpone_time(RRDHOST *host);
const char *stream_handshake_error_to_string(STREAM_HANDSHAKE handshake_error);
void stream_capabilities_to_json_array(BUFFER *wb, STREAM_CAPABILITIES caps, const char *key);
void rrdpush_receive_log_status(struct receiver_state *rpt, const char *msg, const char *status);
void log_receiver_capabilities(struct receiver_state *rpt);
void log_sender_capabilities(struct sender_state *s);
STREAM_CAPABILITIES convert_stream_version_to_capabilities(int32_t version);
int32_t stream_capabilities_to_vn(uint32_t caps);

void receiver_state_free(struct receiver_state *rpt);
bool stop_streaming_receiver(RRDHOST *host, const char *reason);

void sender_thread_buffer_free(void);

void rrdhost_receiver_to_json(BUFFER *wb, RRDHOST *host, const char *key, time_t now __maybe_unused);
void rrdhost_sender_to_json(BUFFER *wb, RRDHOST *host, const char *key, time_t now __maybe_unused);

#include "replication.h"

#endif //NETDATA_RRDPUSH_H
