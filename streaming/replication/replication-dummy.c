#include "replication.h"
#ifndef ENABLE_REPLICATION

void replication_init(void) {}
void replication_fini(void) {}

void replication_new_host(RRDHOST *RH) {
    UNUSED(RH);
}

void replication_delete_host(RRDHOST *RH) {
    UNUSED(RH);
}

void replication_thread_start(RRDHOST *RH) {
    UNUSED(RH);
}

void replication_thread_stop(RRDHOST *RH) {
    UNUSED(RH);
}

void replication_receiver_connected(RRDHOST *RH, char *Buf, size_t Len) {
    UNUSED(RH);
    UNUSED(Buf);
    UNUSED(Len);
}
void replication_sender_connected(RRDHOST *RH, const char *Buf, size_t Len) {
    UNUSED(RH);
    UNUSED(Buf);
    UNUSED(Len);
}

bool replication_receiver_fill_gap(RRDHOST *RH, const char *Buf) {
    UNUSED(RH);
    UNUSED(Buf);

    return false;
}

void replication_receiver_drop_gap(RRDHOST *RH, time_t After, time_t Before) {
    UNUSED(RH);
    UNUSED(After);
    UNUSED(Before);
}

size_t replication_receiver_number_of_pending_gaps(RRDHOST *RH) {
    UNUSED(RH);
    return 0;
}

const char *replication_logs(RRDHOST *RH) {
    UNUSED(RH);
    return NULL;
}

#endif
