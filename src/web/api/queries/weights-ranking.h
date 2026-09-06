// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEIGHTS_RANKING_H
#define NETDATA_WEIGHTS_RANKING_H

#include <stddef.h>

typedef int (*weights_rank_compare_t)(const void *, const void *, void *);

// Negative comparisons mean stronger; the weakest retained candidate stays at the root.
typedef struct {
    void **items;
    size_t used;
    size_t capacity;
    weights_rank_compare_t compare;
    void *data;
} WEIGHTS_RANKING;

static inline void weights_ranking_sift(WEIGHTS_RANKING *ranking, size_t root, size_t count) {
    while(root < count / 2) {
        size_t child = root * 2 + 1;
        if(child + 1 < count && ranking->compare(ranking->items[child + 1], ranking->items[child], ranking->data) > 0)
            child++;
        if(ranking->compare(ranking->items[root], ranking->items[child], ranking->data) >= 0)
            break;
        void *tmp = ranking->items[root];
        ranking->items[root] = ranking->items[child];
        ranking->items[child] = tmp;
        root = child;
    }
}

static inline void weights_ranking_offer(WEIGHTS_RANKING *ranking, void *item) {
    if(!ranking->capacity)
        return;
    if(ranking->used < ranking->capacity) {
        size_t pos = ranking->used++;
        ranking->items[pos] = item;
        while(pos) {
            size_t parent = (pos - 1) / 2;
            if(ranking->compare(ranking->items[parent], item, ranking->data) >= 0)
                break;
            ranking->items[pos] = ranking->items[parent];
            ranking->items[parent] = item;
            pos = parent;
        }
    }
    else if(ranking->compare(item, ranking->items[0], ranking->data) < 0) {
        ranking->items[0] = item;
        weights_ranking_sift(ranking, 0, ranking->used);
    }
}

static inline void weights_ranking_sort(WEIGHTS_RANKING *ranking) {
    for(size_t count = ranking->used; count > 1; count--) {
        void *tmp = ranking->items[0];
        ranking->items[0] = ranking->items[count - 1];
        ranking->items[count - 1] = tmp;
        weights_ranking_sift(ranking, 0, count - 1);
    }
}

#endif
