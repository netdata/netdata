#include <daemon/common.h>
#include "testrunner.h"

#ifdef ENABLE_TESTS

#include <gtest/gtest.h>

int netdata_tests(int argc, char *argv[]) {
    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}

#else

int netdata_tests(int argc, char *argv[]) {
    UNUSED(argc);
    UNUSED(argv);
    return 0;
}

#endif
