// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef HTTPD_CONNLIST_H
#define HTTPD_CONNLIST_H

#include "streaming.h"

// (-1) in following macro is to keep conn list + next pointer
// be power of 2
#define CONN_LIST_MEMPOOL_SIZE ((2^5)-1) 
struct conn_list_leaf {
    h2o_stream_conn_t *conn[CONN_LIST_MEMPOOL_SIZE];
    struct conn_list_leaf *next;
};

typedef struct {
    struct conn_list_leaf *head;
    struct conn_list_leaf *tail;
    int size;
    int capacity;
    pthread_mutex_t lock;
} conn_list_t;

extern conn_list_t conn_list;

void conn_list_insert(conn_list_t *list, h2o_stream_conn_t *conn);
void conn_list_iter_all(conn_list_t *list, void (*cb)(h2o_stream_conn_t *conn));
int conn_list_remove_conn(conn_list_t *list, h2o_stream_conn_t *conn);

#endif /* HTTPD_CONNLIST_H */
