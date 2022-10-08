#include <gtest/gtest.h>
#include "daemon/common.h"

TEST(bitmap, bitops) {
    size_t N = 256;
    bitmap_t BM = bitmap_new(N);

    for (size_t Idx = 0; Idx != N; Idx += 2) {
        EXPECT_FALSE(bitmap_get(BM, Idx));
        bitmap_set(BM, Idx, true);
        EXPECT_TRUE(bitmap_get(BM, Idx));
    }

    bitmap_delete(BM);
}
