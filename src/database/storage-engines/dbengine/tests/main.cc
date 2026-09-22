// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

#include <cstdio>
#include <cstdlib>
#include <string>

// Prints what the engine said during a case, and only for a case that failed. A passing case says nothing, so the
// run reads as a list of results; a failing one is followed by its own engine log, in its own order.
//
// The buffer is cleared when a case starts, so what an engine thread writes between two cases is dropped, and a
// line a previous case's engine thread writes late - after the next case has started - lands with that next case.
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

        // copied under the lock and printed outside it: an engine thread that logs while this runs should not
        // wait on stderr, which is the very shape the sink contract tells embedders to avoid
        std::string text;
        bool truncated;
        {
            NetdataTestLogCapture &capture = netdata_test_log_capture();
            std::lock_guard<std::mutex> lock(capture.mutex);
            text = capture.text;
            truncated = capture.truncated;
        }

        if (text.empty()) {
            std::fprintf(stderr, "  the engine logged nothing during this test\n");
            return;
        }

        std::fprintf(stderr, "  what the engine logged during this test:\n%s", text.c_str());
        if (truncated)
            std::fprintf(stderr, "    ... and more, dropped at the %d byte ceiling\n",
                         NETDATA_TEST_LOG_CAPTURE_MAX);
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
    // no engine to ask, and libnetdata's own recovery log for a failed protected read. This lifts the per-source
    // flood limit those lines would otherwise hit, which is right for a running agent and wrong here. It does not
    // lift the per-call-site limits: those live in each site's own ERROR_LIMIT and still drop repeats, on the
    // sink's side of the layer as well as the logger's.
    nd_log_limits_unlimited();

    ::testing::InitGoogleTest(&argc, argv);
    ::testing::UnitTest::GetInstance()->listeners().Append(new EngineLogListener);
    return RUN_ALL_TESTS();
}
