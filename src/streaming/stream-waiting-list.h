// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_WAITING_LIST_H
#define NETDATA_STREAM_WAITING_LIST_H

void stream_thread_received_metadata(void);
void stream_thread_received_replication(void);

#ifdef STREAM_INTERNALS
#include "stream-thread.h"
void stream_thread_process_waiting_list_unsafe(struct stream_thread *sth, usec_t now_ut);
#endif

#endif //NETDATA_STREAM_WAITING_LIST_H
