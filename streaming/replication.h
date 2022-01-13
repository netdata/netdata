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
// RDATA command with arguments TBD?? probably a timewindow struct
#define RDATA_CMD "RDATA"
// GAP command with arguments TBD?? probably a timewindow struct
#define GAP_CMD "GAP"

// Replication structs
typedef struct replication_state {
    // thread variables
    netdata_thread_t thread;
    netdata_mutex_t mutex;
    unsigned int enabled; // result of configuration and negotiation. Runtime flag
    unsigned int spawned;// if the replication thread has been spawned    
    // connection variables
    int socket;
    unsigned int connected;
    char connected_to[101];
    size_t reconnects_counter;
    size_t send_attempts;
    size_t begin;
    size_t not_connected_loops;
    size_t sent_bytes_on_this_connection;
    time_t last_sent_t;
    usec_t reconnect_delay;
    // buffer variables
    //TBD: is the mutex for thread management sufficient also for handling access management to the buffers.
    struct circular_buffer *buffer;
    BUFFER *build;
    char read_buffer[512];
    int read_len;
} REPLICATION_STATE;

// GAP structs
typedef struct time_window {
    //check also struct timeval for time_t replacement
    time_t t_start; // window start
    time_t t_first; // first sample in the time window
    time_t t_end; // window end
} TIME_WINDOW;

typedef struct gap {
    char *uid;
    char *uuid;
    char *status;
    TIME_WINDOW t_window;
} GAP;

typedef struct gaps {
    struct gap *gaps_timeline; 
} GAPS;
