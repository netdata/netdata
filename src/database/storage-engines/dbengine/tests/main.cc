// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

#include <cstdio>
#include <cstdlib>
#include <string>

// Prints what the engine said during a case, and only for a case that failed. A passing case says nothing, so the
// run reads as a list of results; a failing one is followed by its own engine log, in its own order, with nothing
// from any other case mixed into it.
//
// The buffer is cleared when a case starts rather than when one ends, so that anything an engine thread writes
// between two cases - a teardown finishing late - lands with the case that follows rather than being dropped.
class EngineLogListener : public ::testing::EmptyTestEventListener {
    void OnTestStart(const ::testing::TestInfo &) override {
        NetdataTestLogCapture &capture = netdata_test_log_capture();
        std::lock_guard<std::mutex> lock(capture.mutex);
        capture.text.clear();
        capture.truncated = false;
    }

    void OnTestEnd(const ::testing::TestInfo &info) override {
        if (info.result()->Passed())
            return;

        NetdataTestLogCapture &capture = netdata_test_log_capture();
        std::lock_guard<std::mutex> lock(capture.mutex);

        if (capture.text.empty()) {
            std::fprintf(stderr, "  the engine logged nothing during this test\n");
            return;
        }

        std::fprintf(stderr, "  what the engine logged during this test:\n%s", capture.text.c_str());
        if (capture.truncated)
            std::fprintf(stderr, "    ... truncated at %d bytes\n", NETDATA_TEST_LOG_CAPTURE_MAX);
    }
};

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

    // What still reaches netdata's logger rather than the suite's sink: the process-wide page-data layer, which has
    // no engine to ask, and libnetdata's own recovery log for a failed protected read. The default limits drop
    // repeated messages, which is right for a running agent and wrong for the few lines that get here.
    nd_log_limits_unlimited();

    ::testing::InitGoogleTest(&argc, argv);
    ::testing::UnitTest::GetInstance()->listeners().Append(new EngineLogListener);
    return RUN_ALL_TESTS();
}
