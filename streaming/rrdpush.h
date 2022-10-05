// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDPUSH_H
#define NETDATA_RRDPUSH_H 1

#include "database/rrd.h"
#include "libnetdata/libnetdata.h"
#include "web/server/web_client.h"
#include "daemon/common.h"

#define CONNECTED_TO_SIZE 100

// ----------------------------------------------------------------------------
// obsolete versions - do not use anymore

#define STREAM_OLD_VERSION_CLAIM 3
#define STREAM_OLD_VERSION_CLABELS 4
#define STREAM_OLD_VERSION_COMPRESSION 5 // this is production

// ----------------------------------------------------------------------------
// capabilities negotiation

typedef enum {
    // do not use the first 3 bits
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
    STREAM_CAP_GAP_FILLING      = (1 << 12), // gap filling supported

    // this must be signed int, so don't use the last bit
    // needed for negotiating errors between parent and child
} STREAM_CAPABILITIES;

#ifdef  ENABLE_COMPRESSION
#define STREAM_HAS_COMPRESSION STREAM_CAP_COMPRESSION
#else
#define STREAM_HAS_COMPRESSION 0
#endif  //ENABLE_COMPRESSION

#define STREAM_OUR_CAPABILITIES (STREAM_CAP_V1 | STREAM_CAP_V2 | STREAM_CAP_VN | STREAM_CAP_VCAPS | STREAM_CAP_HLABELS | STREAM_CAP_CLAIM | STREAM_CAP_CLABELS | STREAM_HAS_COMPRESSION | STREAM_CAP_FUNCTIONS)

#define stream_has_capability(rpt, capability) ((rpt) && ((rpt)->capabilities & (capability)))

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
    STREAM_HANDSHAKE_ERROR_CANT_CONNECT = -9
} STREAM_HANDSHAKE;


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
    char *buffer;
    size_t buffer_size;
    size_t buffer_len;
    size_t buffer_pos;
    char *out_buffer;
    size_t out_buffer_len;
    size_t out_buffer_pos;
    size_t total_compressed;
    size_t total_uncompressed;
    size_t packet_count;
    struct decompressor_data *data; // Decompression API specific data
    void (*reset)(struct decompressor_state *state);
    size_t (*start)(struct decompressor_state *state, const char *header, size_t header_size);
    size_t (*put)(struct decompressor_state *state, const char *data, size_t size);
    size_t (*decompress)(struct decompressor_state *state);
    size_t (*decompressed_bytes_in_buffer)(struct decompressor_state *state);
    size_t (*get)(struct decompressor_state *state, char *data, size_t size);
    void (*destroy)(struct decompressor_state **state);
};
#endif

// Thread-local storage
    // Metric transmission: collector threads asynchronously fill the buffer, sender thread uses it.

typedef enum {
    SENDER_FLAG_OVERFLOW    = (1 << 0), // The buffer has been overflown
    SENDER_FLAG_COMPRESSION = (1 << 1), // The stream needs to have and has compression
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
    time_t last_sent_t;
    size_t not_connected_loops;
    // Metrics are collected asynchronously by collector threads calling rrdset_done_push(). This can also trigger
    // the lazy creation of the sender thread - both cases (buffer access and thread creation) are guarded here.
    netdata_mutex_t mutex;
    struct circular_buffer *buffer;
    char read_buffer[PLUGINSD_LINE_MAX + 1];
    int read_len;
    STREAM_CAPABILITIES capabilities;

    int rrdpush_sender_pipe[2];                     // collector to sender thread signaling
    int rrdpush_sender_socket;

#ifdef ENABLE_COMPRESSION
    struct compressor_state *compressor;
#endif
#ifdef ENABLE_HTTPS
    struct netdata_ssl ssl;                  // Structure used to encrypt the connection
#endif
};

struct receiver_state {
    RRDHOST *host;
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
    int update_every;
    STREAM_CAPABILITIES capabilities;
    time_t last_msg_t;
    char read_buffer[PLUGINSD_LINE_MAX + 1];
    int read_len;
    unsigned int shutdown:1;    // Tell the thread to exit
    unsigned int exited;      // Indicates that the thread has exited  (NOT A BITFIELD!)
#ifdef ENABLE_HTTPS
    struct netdata_ssl ssl;
#endif
#ifdef ENABLE_COMPRESSION
    unsigned int rrdpush_compression;
    struct decompressor_state *decompressor;
#endif
};

struct rrdpush_destinations {
    STRING *destination;

    const char *last_error;
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
extern unsigned int remote_clock_resync_iterations;

extern void rrdpush_destinations_init(RRDHOST *host);
extern void rrdpush_destinations_free(RRDHOST *host);

extern void sender_init(RRDHOST *parent);
BUFFER *sender_start(struct sender_state *s);
void sender_commit(struct sender_state *s, BUFFER *wb);
void sender_cancel(struct sender_state *s);
extern int rrdpush_init();
extern int configured_as_parent();
extern void rrdset_done_push(RRDSET *st);
extern bool rrdset_push_chart_definition_now(RRDSET *st);
extern bool rrdpush_incremental_transmission_of_chart_definitions(RRDHOST *host, DICTFE *dictfe, bool restart, bool stop);
extern void *rrdpush_sender_thread(void *ptr);
extern void rrdpush_send_host_labels(RRDHOST *host);
extern void rrdpush_claimed_id(RRDHOST *host);

extern int rrdpush_receiver_thread_spawn(struct web_client *w, char *url);
extern void rrdpush_sender_thread_stop(RRDHOST *host);

extern void rrdpush_sender_send_this_host_variable_now(RRDHOST *host, const RRDVAR_ACQUIRED *rva);
extern void log_stream_connection(const char *client_ip, const char *client_port, const char *api_key, const char *machine_guid, const char *host, const char *msg);
extern int connect_to_one_of_destinations(
    RRDHOST *host,
    int default_port,
    struct timeval *timeout,
    size_t *reconnects_counter,
    char *connected_to,
    size_t connected_to_size,
    struct rrdpush_destinations **destination);

extern void rrdpush_signal_sender_to_wake_up(struct sender_state *s);

#ifdef ENABLE_COMPRESSION
struct compressor_state *create_compressor();
struct decompressor_state *create_decompressor();
size_t is_compressed_data(const char *data, size_t data_size);
#endif

extern void log_receiver_capabilities(struct receiver_state *rpt);
extern void log_sender_capabilities(struct sender_state *s);
extern STREAM_CAPABILITIES convert_stream_version_to_capabilities(int32_t version);
extern int32_t stream_capabilities_to_vn(uint32_t caps);

#endif //NETDATA_RRDPUSH_H
