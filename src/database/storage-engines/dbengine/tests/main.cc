// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

#include <cstdio>
#include <cstdlib>
#include <string>

// googletest ships a main of its own, but this suite cannot use it: two things have to be settled before the first
// test touches the engine, and neither can be undone afterwards.
int main(int argc, char **argv) {
    // The libuv pool is sized at its first use and never again, so this must happen before anything in the process
    // queues work. support.h explains what a wrong size does.
    //
    // Checked rather than assumed: if this fails the pool falls back to libuv's default of four while the
    // configuration still claims DBENGINE_TEST_UV_THREADS, and an over-stated pool is precisely the case that hangs
    // the process instead of failing it. Everything around this call exists to prevent that, so it says so and
    // stops rather than running the suite into a hang.
    if (setenv("UV_THREADPOOL_SIZE", std::to_string(DBENGINE_TEST_UV_THREADS).c_str(), 1) != 0) {
        std::fprintf(stderr, "could not set UV_THREADPOOL_SIZE, which the suite needs before any engine starts\n");
        return 2;
    }

    // The engine's diagnostics are what a failing test is read from. The default limits drop repeated messages,
    // which is right for a running agent and wrong here.
    nd_log_limits_unlimited();

    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}
