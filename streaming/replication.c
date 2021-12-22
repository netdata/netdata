//Includes
#include "rrdpush.h"

// Add the replication functions here

// Thread creation
// thread destroy
void replication_sender_init(RRDHOST *host){}
void replication_receiver_init(RRDHOST *host){}

void replication_init(RRDHOST *host){
    // variables, structs and mutexes/locks initialization, mem allocations, etc.
    replication_sender_init(host);
    replication_receiver_init(host);
}

void replication_sender_thread_spawn(RRDHOST *host){}
void replication_receiver_thread_spawn(RRDHOST *host){}
void replication_sender_thread_cleanup(RRDHOST *host){}
void replication_receiver_thread_cleanup(RRDHOST *host){}
// Any join, start, stop, wait, etc thread function goes here.
//void replication_sender_thread_stop(){}
//void replication_receiver_thread_stop(){}

// memory mode access
void collect_replication_gap_data(){
    // collection of gap data in cache/temporary structure
}
void update_memory_index(){
    //dbengine
    //other memory modes?
}

// Replication parser
size_t replication_parser(struct receiver_state *rpt, struct plugind *cd, FILE *fp) {
    // create or reuse the parser without interference between streaming and replication
    // support REP on/off/pause/ack
    // GAP
}

// gap processing
// FSMs for replication protocol implementation
// REP on
// REP off
// REP pause/continue
// REP ack
// GAP
typedef struct time_window{
    time_t t_start; // window start
    time_t t_first; // first sample in the time window
    time_t t_end; // window end
};
typedef struct gap {
    char *uid;
    char *uuid;
    char *status;
    struct time_window;
};
typedef struct gaps {
    struct gap *gaps_timeline; 
};
// RDATA
