#include "libnetdata/libnetdata.h"
#include "connlist.h"

conn_list_t conn_list = { NULL, NULL, 0, 0, PTHREAD_MUTEX_INITIALIZER };

static h2o_stream_conn_t **conn_list_get_null_element_unsafe(conn_list_t *list)
{
    struct conn_list_leaf *leaf = list->head;
    while (leaf != NULL) {
        for (int i = 0; i < CONN_LIST_MEMPOOL_SIZE; i++) {
            if (leaf->conn[i] == NULL)
                return &leaf->conn[i];
        }
        leaf = leaf->next;
    }
    return NULL;
}

void conn_list_insert(conn_list_t *list, h2o_stream_conn_t *conn)
{
    pthread_mutex_lock(&list->lock);

    // in case the allocated capacity is not used up
    // we can reuse the null element
    if (list->capacity != list->size) {
        h2o_stream_conn_t **null_element = conn_list_get_null_element_unsafe(list);
        if (unlikely(null_element == NULL)) {
            pthread_mutex_unlock(&list->lock);
            error_report("conn_list_insert: capacity != size but no null element found");
            return;
        }
        *null_element = conn;
        list->size++;
        pthread_mutex_unlock(&list->lock);
        return;
    }

    // if not, we need to allocate a new leaf
    struct conn_list_leaf *old_tail = list->tail;
    list->tail = callocz(1, sizeof(struct conn_list_leaf));
    if (unlikely(old_tail == NULL))
        list->head = list->tail;
    else
        old_tail->next = list->tail;
    
    list->tail->conn[0] = conn;
    list->size++;
    list->capacity += CONN_LIST_MEMPOOL_SIZE;

    pthread_mutex_unlock(&list->lock);
}

typedef struct {
    conn_list_t *list;
    struct conn_list_leaf *leaf;
    int idx;
} conn_list_iter_t;

static inline void conn_list_iter_create_unsafe(conn_list_iter_t *iter, conn_list_t *list)
{
    iter->list = list;
    iter->leaf = list->head;
    iter->idx = 0;
}

static inline int conn_list_iter_next_unsafe(conn_list_iter_t *iter, h2o_stream_conn_t **conn)
{
    if (unlikely(iter->idx == iter->list->capacity))
        return 0;

    if (iter->idx && iter->idx % CONN_LIST_MEMPOOL_SIZE == 0) {
        iter->leaf = iter->leaf->next;
    }

    *conn = iter->leaf->conn[iter->idx++ % CONN_LIST_MEMPOOL_SIZE];
    return 1;
}

void conn_list_iter_all(conn_list_t *list, void (*cb)(h2o_stream_conn_t *conn))
{
    pthread_mutex_lock(&list->lock);
    conn_list_iter_t iter;
    conn_list_iter_create_unsafe(&iter, list);
    h2o_stream_conn_t *conn;
    while (conn_list_iter_next_unsafe(&iter, &conn)) {
        if (conn == NULL)
            continue;
        cb(conn);
    }
    pthread_mutex_unlock(&list->lock);
}

static void conn_list_garbage_collect_unsafe(conn_list_t *list)
{
    if (list->capacity - list->size > CONN_LIST_MEMPOOL_SIZE) {
        struct conn_list_leaf *new_tail = list->head;
        while (new_tail->next != list->tail)
            new_tail = new_tail->next;

        // check if the tail leaf is empty and move the data if not
        for (int i = 0; i < CONN_LIST_MEMPOOL_SIZE; i++) {
            if (list->tail->conn[i] != NULL) {
                h2o_stream_conn_t **null_element = conn_list_get_null_element_unsafe(list);
                if (unlikely(null_element == NULL)) {
                    error_report("conn_list_garbage_collect_unsafe: list->capacity - list->size > CONN_LIST_MEMPOOL_SIZE but no null element found?");
                    return;
                }
                *null_element = list->tail->conn[i];
                list->tail->conn[i] = NULL;
            }
        }

        freez(list->tail);
        new_tail->next = NULL;
        list->tail = new_tail;
        list->capacity -= CONN_LIST_MEMPOOL_SIZE;
    }
}

static inline int conn_list_iter_remove(conn_list_iter_t *iter, h2o_stream_conn_t *conn)
{
    if (unlikely(iter->idx == iter->list->capacity))
        return -1;

    if (iter->idx && iter->idx % CONN_LIST_MEMPOOL_SIZE == 0) {
        iter->leaf = iter->leaf->next;
    }

    if(conn == iter->leaf->conn[iter->idx % CONN_LIST_MEMPOOL_SIZE]) {
        iter->leaf->conn[iter->idx % CONN_LIST_MEMPOOL_SIZE] = NULL;

        iter->idx++;
        return 1;
    }

    iter->idx++;
    return 0;
}

int conn_list_remove_conn(conn_list_t *list, h2o_stream_conn_t *conn)
{
    pthread_mutex_lock(&list->lock);
    conn_list_iter_t iter;
    conn_list_iter_create_unsafe(&iter, list);
    int rc;
    while (!(rc = conn_list_iter_remove(&iter, conn)));
    if (rc == -1) {
        pthread_mutex_unlock(&list->lock);
        error_report("conn_list_remove_conn: conn not found");
        return 0;
    }
    list->size--;
    conn_list_garbage_collect_unsafe(list);
    pthread_mutex_unlock(&list->lock);
    return 1;
}

