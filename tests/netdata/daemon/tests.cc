#include <gtest/gtest.h>
#include "daemon/common.h"

TEST(daemon, static_threads) {
    struct netdata_static_thread *static_threads = static_threads_get();

    EXPECT_NE(static_threads, nullptr);

    size_t n;
    for (n = 0; static_threads[n].start_routine != NULL; n++) { }

    EXPECT_GT(n, 1);

    // check that each thread's start routine is unique
    for (size_t i = 0; i != n - 1; i++)
        for (size_t j = i + 1; j != n; j++)
            EXPECT_NE(static_threads[i].start_routine, static_threads[j].start_routine);

    freez(static_threads);
}
