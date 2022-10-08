#include <gtest/gtest.h>
#include "daemon/common.h"

TEST(arrayalloc, alloc_ops) {
    size_t elements = 10000;
    void *pointers[elements];

    ARAL *ar = arrayalloc_create(20, 10, NULL, NULL, false);

    for (size_t i = 0; i < elements; i++)
        pointers[i] = arrayalloc_mallocz(ar);

    for (size_t div = 5; div >= 2; div--) {
        for (size_t i = 0; i < elements / div; i++)
            arrayalloc_freez(ar, pointers[i]);

        for (size_t i = 0; i < elements / div; i++)
            pointers[i] = arrayalloc_mallocz(ar);
    }

    for (size_t step = 50; step >= 10 ;step -= 10) {
        for (size_t i = 0; i < elements; i += step)
            arrayalloc_freez(ar, pointers[i]);

        for (size_t i = 0; i < elements; i += step)
            pointers[i] = arrayalloc_mallocz(ar);
    }

    for (size_t i = 0; i < elements ;i++)
        arrayalloc_freez(ar, pointers[i]);

    EXPECT_EQ(ar->internal.pages, nullptr);

    size_t increment = elements / 10;
    size_t allocated = 0;
    for (size_t all = increment; all <= elements ; all += increment) {

        for (; allocated < all ; allocated++)
            pointers[allocated] = arrayalloc_mallocz(ar);

        size_t to_free = now_realtime_usec() % all;
        size_t free_list[to_free];

        for (size_t i = 0; i < to_free; i++) {
            size_t pos;
            do {
                pos = now_realtime_usec() % all;
            } while(!pointers[pos]);

            arrayalloc_freez(ar, pointers[pos]);
            pointers[pos] = NULL;
            free_list[i] = pos;
        }

        for (size_t i = 0; i < to_free; i++) {
            size_t pos = free_list[i];
            pointers[pos] = arrayalloc_mallocz(ar);
        }
    }

    for (size_t i = 0; i < allocated - 1; i++)
        arrayalloc_freez(ar, pointers[i]);

    arrayalloc_freez(ar, pointers[allocated - 1]);

    EXPECT_EQ(ar->internal.pages, nullptr);
}
