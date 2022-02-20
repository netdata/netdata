//Includes
#define REPLICATION_MSG "REPLICATION_STREAM"
#define REPLICATE_CMD "REPLICATE"
#define REP_CMD "REP"
#define REP_ACK_CMD "REP ack"
// REP command with arguments 
// on, off, pause/continue, ack
enum REP_ARG {
    off = 0,
    on = 1,
    next = 2,
    ack = 3
};
// RDATA command with arguments TBD?? BEGIN SET END
#define RDATA_CMD "RDATA"
// GAP command with arguments TBD?? probably a timewindow struct
#define GAP_CMD "GAP"
#define RECEIVER_CMD_Q_MAX_SIZE (32768)

typedef struct gaps_queue GAPS;
typedef struct gap GAP;
typedef struct time_window TIME_WINDOW;

// Replication structs
typedef struct replication_state {
    RRDHOST *host;
    // thread variables
    netdata_thread_t thread;
    netdata_mutex_t mutex;
    unsigned int enabled; // result of configuration and negotiation. Runtime flag
    unsigned int spawned;// if the replication thread has been spawned 
    //state variables
    int excom;   
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
    //TBD: is the mutex for thread management sufficient also for handling access management to the buffers.
    unsigned int overflow:1; //?
    struct circular_buffer *buffer;
    BUFFER *build;
    char read_buffer[512];
    int read_len;
    //rx thread variables    
    time_t last_msg_t;
    char *client_ip;
    char *client_port;
#ifdef ENABLE_HTTPS
    struct netdata_ssl ssl;
#endif
    char *program_name;
    char *program_version;
    // GAPS
    GAPS *gaps_timeline;
} REPLICATION_STATE;

// GAP structs
typedef struct time_window {
    //check also struct timeval for time_t replacement
    time_t t_start; // window start
    time_t t_first; // first sample in the time window
    time_t t_end; // window end
} TIME_WINDOW;

typedef struct gap {
    char *uid; // unique number for the GAP
    char *uuid; // unique number for the host id
    char *status; // a gap can be oncreation, ontransmission, filled
    TIME_WINDOW t_window; // This is the time window variables of a gap
} GAP;

typedef struct gaps_queue {
    unsigned head, tail;
    struct gap gaps_timeline[RECEIVER_CMD_Q_MAX_SIZE];
    uv_mutex_t gaps_mutex;
    uv_cond_t gaps_cond;
    unsigned queue_size;
    uint8_t stop_thread; // if set to 1 the thread should shut down
    time_t beginoftime;
} GAPS;
