// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDPUSH_H
#define NETDATA_RRDPUSH_H 1

#include "database/rrd.h"
#include "libnetdata/libnetdata.h"
#include "web/server/web_client.h"
#include "daemon/common.h"

#define CONNECTED_TO_SIZE 100

#define STREAM_VERSION_CLAIM 3
#define STREAM_VERSION_CLABELS 4
#define STREAM_VERSION_COMPRESSION 5
#define VERSION_GAP_FILLING 6

#ifdef  ENABLE_COMPRESSION
#define STREAMING_PROTOCOL_CURRENT_VERSION (uint32_t)(STREAM_VERSION_COMPRESSION)
#else
#define STREAMING_PROTOCOL_CURRENT_VERSION (uint32_t)(STREAM_VERSION_CLABELS)
#endif  //ENABLE_COMPRESSION

#define STREAMING_PROTOCOL_VERSION "1.1"
#define START_STREAMING_PROMPT "Hit me baby, push them over..."
#define START_STREAMING_PROMPT_V2  "Hit me baby, push them over and bring the host labels..."
#define START_STREAMING_PROMPT_VN "Hit me baby, push them over with the version="
#define START_COMPRESSION "stream_compression="

#define HTTP_HEADER_SIZE 8192

typedef enum {
    RRDPUSH_MULTIPLE_CONNECTIONS_ALLOW,
    RRDPUSH_MULTIPLE_CONNECTIONS_DENY_NEW
} RRDPUSH_MULTIPLE_CONNECTIONS_STRATEGY;

typedef struct {
    char *os_name;
    char *os_id;
    char *os_version;
    char *kernel_name;
    char *kernel_version;
} stream_encoded_t;

#ifdef ENABLE_COMPRESSION
struct compressor_state {
    char *buffer;
    size_t buffer_size;
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
    struct decompressor_data *data; // Deompression API specific data
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

struct sender_state {
    RRDHOST *host;
    pid_t task_id;
    unsigned int overflow:1;
    int timeout, default_port;
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
    BUFFER *build;
    char read_buffer[512];
    int read_len;
    int32_t version;
    unsigned int rrdpush_compression;
#ifdef ENABLE_COMPRESSION
    struct compressor_state *compressor;
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
    uint32_t stream_version;
    time_t last_msg_t;
    char read_buffer[1024];     // Need to allow RRD_ID_LENGTH_MAX * 4 + the other fields
    int read_len;
    unsigned int shutdown:1;    // Tell the thread to exit
    unsigned int exited;      // Indicates that the thread has exited  (NOT A BITFIELD!)
    unsigned int rrdpush_compression;
#ifdef ENABLE_HTTPS
    struct netdata_ssl ssl;
#endif
#ifdef ENABLE_COMPRESSION
    struct decompressor_state *decompressor;
#endif
};


extern unsigned int default_rrdpush_enabled;
// #ifdef ENABLE_COMPRESSION
// extern unsigned int default_compression_enabled;
// #endif
// Need to create a different streaming version in order to support and select compression.
// Then is compression is supported the agents can negotiate the compression.
extern unsigned int default_compression_enabled;

extern char *default_rrdpush_destination;
extern char *default_rrdpush_api_key;
extern char *default_rrdpush_send_charts_matching;
extern unsigned int remote_clock_resync_iterations;

extern void sender_init(struct sender_state *s, RRDHOST *parent);
void sender_start(struct sender_state *s);
void sender_commit(struct sender_state *s);
extern int rrdpush_init();
extern int configured_as_parent();
extern void rrdset_done_push(RRDSET *st);
extern void rrdset_push_chart_definition_now(RRDSET *st);
extern void *rrdpush_sender_thread(void *ptr);
extern void rrdpush_send_labels(RRDHOST *host);
extern void rrdpush_claimed_id(RRDHOST *host);

extern int rrdpush_receiver_thread_spawn(struct web_client *w, char *url);
extern void rrdpush_sender_thread_stop(RRDHOST *host);

extern void rrdpush_sender_send_this_host_variable_now(RRDHOST *host, RRDVAR *rv);
extern void log_stream_connection(const char *client_ip, const char *client_port, const char *api_key, const char *machine_guid, const char *host, const char *msg);

extern long int parse_stream_version(RRDHOST *host, char *http);
extern unsigned int parse_stream_compression(RRDHOST *host, char *http);
#ifdef ENABLE_COMPRESSION
struct compressor_state *create_compressor();
struct decompressor_state *create_decompressor();
size_t is_compressed_data(const char *data, size_t data_size);
#endif

#endif //NETDATA_RRDPUSH_H
