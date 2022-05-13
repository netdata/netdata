#ifndef NETDATA_REPLICATION_H
#define NETDATA_REPLICATION_H 1

#define REPLICATION_MSG "REPLICATION_STREAM"
#define REPLICATE_CMD "REPLICATE"
#define REP_ACK_CMD "REP ACK"
#define REP_OFF_CMD "REP OFF"
#define REPLICATION_RX_CMD_Q_MAX_SIZE (64)
#define REPLICATION_GAP_TIME_MARGIN 0
#define REPLICATION_STREAMING_WAIT_STEP 2
#define REPLICATION_STREAMING_WAIT_STEP_COUNT 40
#ifdef  ENABLE_DBENGINE
#define MEM_PAGE_BLOCK_SIZE RRDENG_BLOCK_SIZE
#else
#define MEM_PAGE_BLOCK_SIZE 4096
#endif

typedef struct gap GAP;
typedef struct time_window TIME_WINDOW;
typedef struct gaps_queue GAPS;
typedef struct replication REPLICATION;
typedef struct rrddim_past_data RRDDIM_PAST_DATA;
typedef struct replication_state REPLICATION_STATE;
// TODO: Clean_up REPLICATION_STATE struct to fit the tx/rx variable needs.
// typedef struct tx_replication_state Tx_REPLICATION_STATE;
// typedef struct rx_replication_state Rx_REPLICATION_STATE;

struct replication {
    REPLICATION_STATE *tx_replication;
    REPLICATION_STATE *rx_replication;
};

struct replication_state {
    RRDHOST *host;
    // thread variables
    netdata_thread_t thread;
    netdata_mutex_t mutex;
    unsigned int enabled; // result of configuration and negotiation. Runtime flag
    unsigned int spawned; // replication thread has been spawned:1
    volatile unsigned int sender_thread_join; // Following the normal shutdown seq need to verify the replication sender thread shutdown.
    //state variables
    int excom;
    int timeout, default_port;
    // connection variables
    int socket;
    unsigned int connected;
    //tx thread variables
    char connected_to[101];
    size_t reconnects_counter;
    size_t send_attempts;
    size_t begin;
    size_t not_connected_loops;
    size_t sent_bytes;
    size_t sent_bytes_on_this_connection;
    time_t last_sent_t;
    usec_t reconnect_delay;
    // buffer variables
    unsigned int overflow;
    struct circular_buffer *buffer;
    BUFFER *build;
    char read_buffer[512];
    int read_len;
    uint32_t stream_version;
    //rx thread variables
    time_t last_msg_t;
    char *client_ip;
    char *client_port;
    char *key;
    FILE *fp;
#ifdef ENABLE_HTTPS
    struct netdata_ssl ssl;
#endif
    char *program_name;
    char *program_version;
    unsigned int shutdown;    // Set it to 1 command the thread to exit
    unsigned int exited;      // Indicates that the thread has exited
    unsigned int resume;      // Rising edge from pause 0 -> 1 (1). 1 -> 0 (0)
    unsigned int pause;       // 0 means paused, 1 means running
    RRDDIM_PAST_DATA *dim_past_data;
};

// GAP structs
struct time_window {
    time_t t_start; // window start
    time_t t_first; // first sample in the time window
    time_t t_end; // window end
};

struct gap {
    uuid_t gap_uuid; // unique number for the GAP
    char *host_mguid; // unique number for the host id
    char *status; // a gap can be oncreation, ontransmission, filled
    TIME_WINDOW t_window; // This is the time window variables of a gap
};

struct gaps_queue {
    queue_t gaps;      // handles the gap pointers in a queue struct
    GAP gap_data[REPLICATION_RX_CMD_Q_MAX_SIZE]; // array to hold the completed gap structs of the queue
    GAP *gap_buffer;   // a gap struct element to work as buffer to host the gap details at runtime
    time_t beginoftime;// holds the timestamp of the first sample in db for the host
};

struct rrddim_past_data {
    char* rrdset_id;
    char* rrddim_id;
    RRDDIM *rd;
    void* page;
    uint32_t page_length;
    usec_t start_time;
    usec_t end_time;
    struct rrdeng_page_descr* descr;
    struct rrdengine_instance *ctx;
    unsigned long page_correlation_id;
};
extern struct config stream_config;
extern int netdata_use_ssl_on_replication;

void replication_gap_to_str(GAP *a_gap, char **gap_str, size_t *len);
void replication_rdata_to_str(GAP *a_gap, char **rdata_str, size_t *len, int block_id);
void print_collected_metric_past_data(RRDDIM_PAST_DATA *past_data, REPLICATION_STATE *rep_state);
void replication_collect_past_metric_init(REPLICATION_STATE *rep_state, char *rrdset_id, char *rrddim_id);
void replication_collect_past_metric(REPLICATION_STATE *rep_state, time_t timestamp, storage_number number);
void replication_collect_past_metric_done(REPLICATION_STATE *rep_state);
void flush_collected_metric_past_data(RRDDIM_PAST_DATA *dim_past_data, REPLICATION_STATE *rep_state);
void replication_send_clabels(REPLICATION_STATE *rep_state, RRDSET *st);
int save_gap(GAP *a_gap);
int save_all_host_gaps(GAPS *gap_timeline);
int remove_gap(GAP *a_gap);
int remove_all_gaps(void);
int remove_all_host_gaps(RRDHOST* host);
int load_gap(RRDHOST *host);

void replication_state_destroy(REPLICATION_STATE **state);
void rrdset_dump_debug_rep_state(RRDSET *st);
void replication_rdata_to_str(GAP *a_gap, char **rdata_str, size_t *len, int block_id);
void replication_gap_to_str(GAP *a_gap, char **gap_str, size_t *len);
void sender_chart_gap_filling(RRDSET *st, GAP a_gap);
void sender_gap_filling(REPLICATION_STATE *rep_state, GAP a_gap);
void sender_fill_gap_nolock(REPLICATION_STATE *rep_state, RRDSET *st, GAP a_gap);
void copy_gap(GAP *dst, GAP *src);
void reset_gap(GAP *a_gap);
void send_gap_for_replication(RRDHOST *host, REPLICATION_STATE *rep_state);
int finish_gap_replication(RRDHOST *host, REPLICATION_STATE *rep_state);
void cleanup_after_gap_replication(GAPS *gaps_timeline);

void replication_sender_init(RRDHOST *host);
void replication_receiver_init(RRDHOST *host, struct config *stream_config, char *key);
int replication_receiver_thread_spawn(struct web_client *w, char *url);
void replication_sender_thread_spawn(RRDHOST *host);
void replication_sender_thread_stop(RRDHOST *host);
void *replication_sender_thread(void *ptr);
void gaps_init(RRDHOST **a_host);
void gaps_destroy(RRDHOST **a_host);

// Remove these are helper functions
void print_replication_queue_gap(GAPS *gaps_timeline);
void print_replication_state(REPLICATION_STATE *state);
void print_replication_gap(GAP *a_gap);

int overlap_pages_new_gap(REPLICATION_STATE *rep_state);

#ifdef  ENABLE_DBENGINE
extern int rrdeng_store_past_metrics_page_init(RRDDIM_PAST_DATA *dim_past_data, REPLICATION_STATE *rep_state);
extern void rrdeng_store_past_metrics_page(RRDDIM_PAST_DATA *dim_past_data, REPLICATION_STATE *rep_state);
extern void rrdeng_flush_past_metrics_page(RRDDIM_PAST_DATA *dim_past_data, REPLICATION_STATE *rep_state);
extern void rrdeng_store_past_metrics_page_finalize(RRDDIM_PAST_DATA *dim_past_data, REPLICATION_STATE *rep_state);
extern int rrdeng_store_past_metrics_realtime(RRDDIM *rd, RRDDIM_PAST_DATA *dim_past_data);
#endif

#endif
