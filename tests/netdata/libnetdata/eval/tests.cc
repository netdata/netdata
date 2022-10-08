#include <gtest/gtest.h>
#include "daemon/common.h"

TEST(eval, rrdcalc_status) {
    RRDCALC_STATUS a, b;

    memset(&a, 0, sizeof(RRDCALC_STATUS));
    EXPECT_EQ(a, RRDCALC_STATUS_UNINITIALIZED);

    a = RRDCALC_STATUS_REMOVED;
    b = RRDCALC_STATUS_UNDEFINED;
    EXPECT_LT(a, b);

    a = RRDCALC_STATUS_UNDEFINED;
    b = RRDCALC_STATUS_UNINITIALIZED;
    EXPECT_LT(a, b);

    a = RRDCALC_STATUS_UNINITIALIZED;
    b = RRDCALC_STATUS_CLEAR;
    EXPECT_LT(a, b);

    a = RRDCALC_STATUS_CLEAR;
    b = RRDCALC_STATUS_RAISED;
    EXPECT_LT(a, b);


    a = RRDCALC_STATUS_RAISED;
    b = RRDCALC_STATUS_WARNING;
    EXPECT_LT(a, b);

    a = RRDCALC_STATUS_WARNING;
    b = RRDCALC_STATUS_CRITICAL;
    EXPECT_LT(a, b);
}
