//Includes
#include "rrdpush.h"
#define REPLICATION_MSG "STREAM_REPLICATION"

// Replication structs
typedef struct replication_state {
    netdata_thread_t thread;
    netdata_mutex_t mutex;
    unsigned int enabled; // result of configuration and negotiation. Runtime flag
    unsigned int spawned;// if the replication thread has been spawned    
} REPLICATION_STATE;

// GAP structs
typedef struct time_window {
    time_t t_start; // window start
    time_t t_first; // first sample in the time window
    time_t t_end; // window end
} TIME_WINDOW;

typedef struct gap {
    char *uid;
    char *uuid;
    char *status;
    struct time_window;
} GAP;

typedef struct gaps {
    struct gap *gaps_timeline; 
} GAPS;

// Functions definitions
// Initialization
void replication_sender_init(struct sender_state *sender);
void replication_receiver_init(struct receiver_state *receiver, struct config *stream_config);
// Threads
void rrdpush_replication_sender_thread_spawn(RRDHOST *host);
void rrdpush_replication_receiver_thread_spawn(RRDHOST *host);
static void replication_receiver_thread_cleanup(RRDHOST *host);
static void replication_sender_thread_cleanup_callback(void *ptr);
void replication_sender_thread_stop(RRDHOST *host);
