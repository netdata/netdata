//Includes
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
    TIME_WINDOW t_window;
} GAP;

typedef struct gaps {
    struct gap *gaps_timeline; 
} GAPS;
