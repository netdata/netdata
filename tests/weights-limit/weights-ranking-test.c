// SPDX-License-Identifier: GPL-3.0-or-later

#include <assert.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <time.h>
#include "../../src/web/api/queries/weights-ranking.h"

typedef struct { uint32_t score; size_t identity; } ITEM;
typedef struct { bool ascending; size_t comparisons; } ORDER;

static int compare(const void *a, const void *b, void *data) {
    const ITEM *left = a, *right = b;
    ORDER *order = data;
    order->comparisons++;
    if(left->score != right->score)
        return ((left->score < right->score) == order->ascending) ? -1 : 1;
    return (left->identity > right->identity) - (left->identity < right->identity);
}

static ORDER reference_order;
static int reference_compare(const void *a, const void *b) {
    return compare(*(const ITEM *const *)a, *(const ITEM *const *)b, &reference_order);
}

int main(void) {
    uint32_t random = 123456789;
    size_t checks = 0;
    for(size_t count = 0; count < 513; count += count < 16 ? 1 : 17) {
        ITEM *items = calloc(count + 1, sizeof(*items));
        ITEM **expected = calloc(count + 1, sizeof(*expected));
        for(size_t i = 0; i < count; i++) {
            random = random * 1664525U + 1013904223U;
            items[i] = (ITEM){.score = random % 23, .identity = count - i};
            expected[i] = &items[i];
        }
        for(unsigned direction = 0; direction < 2; direction++) {
            reference_order = (ORDER){.ascending = direction};
            qsort(expected, count, sizeof(*expected), reference_compare);
            for(size_t limit = 0; limit <= count + 1; limit++) {
                ORDER order = {.ascending = direction};
                WEIGHTS_RANKING ranking = {.capacity = limit < count ? limit : count, .compare = compare, .data = &order};
                ranking.items = calloc(ranking.capacity + 1, sizeof(*ranking.items));
                for(size_t i = 0; i < count; i++)
                    weights_ranking_offer(&ranking, &items[i]);
                weights_ranking_sort(&ranking);
                assert(ranking.used == ranking.capacity);
                for(size_t i = 0; i < ranking.used; i++)
                    assert(ranking.items[i] == expected[i]);
                free(ranking.items);
                checks++;
            }
        }
        free(expected);
        free(items);
    }
    size_t count = 1000000, limit = 1000;
    ITEM *items = calloc(count, sizeof(*items));
    ORDER order = {0};
    WEIGHTS_RANKING ranking = {.capacity = limit, .compare = compare, .data = &order};
    ranking.items = calloc(limit, sizeof(*ranking.items));
    clock_t started = clock();
    for(size_t i = 0; i < count; i++) {
        random = random * 1664525U + 1013904223U;
        items[i] = (ITEM){.score = random, .identity = i};
        weights_ranking_offer(&ranking, &items[i]);
    }
    weights_ranking_sort(&ranking);
    clock_t finished = clock();
    size_t selection_comparisons = order.comparisons;
    assert(ranking.used == limit);
    assert(order.comparisons < count * 25);
    size_t stronger = 0;
    for(size_t i = 0; i < count; i++)
        stronger += compare(&items[i], ranking.items[limit - 1], &order) < 0;
    assert(stronger == limit - 1);
    printf("%zu selector property cases passed; 1000/1000000 candidates: %.3f seconds, %zu comparisons, %zu candidate bytes\n",
           checks, (double)(finished - started) / CLOCKS_PER_SEC, selection_comparisons, limit * sizeof(void *));
    free(ranking.items);
    free(items);
    return 0;
}
