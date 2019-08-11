// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKEND_MONGODB_H
#define NETDATA_BACKEND_MONGODB_H

#include "backends/backends.h"

#define MONGODB_THREADS_NUMBER 10
#define MONGODB_THREAD_INDEX_UNDEFINED -1

struct mongodb_thread {
    netdata_thread_t thread;
    netdata_mutex_t mutex;

    BUFFER *buffer;
    size_t n_bytes;
    size_t n_metrics;

    int busy;
    int finished;
    int error;
};

extern int mongodb_init(const char *uri_string, const char *database_string, const char *collection_string, const int32_t socket_timeout);

extern void *mongodb_insert(void *mongodb_thread);

extern void mongodb_cleanup();

extern int read_mongodb_conf(const char *path, char **uri_p, char **database_p, char **collection_p);

#endif //NETDATA_BACKEND_MONGODB_H
