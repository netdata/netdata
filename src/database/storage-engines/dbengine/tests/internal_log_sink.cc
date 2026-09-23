// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

extern "C" {
#include "database/storage-engines/dbengine/rrdengine.h"
#include "database/storage-engines/dbengine/page.h"
}

#include <atomic>
#include <cerrno>
#include <chrono>
#include <cstdio>
#include <fcntl.h>
#include <string>
#include <thread>
#include <unistd.h>
#include <vector>

// The macro layer in dbengine-log.h, family by family, on an engine object that is allocated but never started: which
// priority each family hands the sink, that the site the sink is told is the line that emitted, what errno the sink
// and the caller see, the rate-limited family's window, that an engine without a sink reaches netdata's logger, and
// where the page allocator's notice goes. Limiters here are local, not the per-site statics, and opened as of now, so
// a repeated or reordered run, on a host up for any length of time, starts every window fresh.

namespace {

struct LogRecord {
    ND_LOG_FIELD_PRIORITY priority;
    std::string file;
    unsigned long line;
    std::string text;
    int errno_seen;
};

// Never freed, as the contract asks of a sink's data.
struct LogRecorder {
    std::mutex mutex;
    std::vector<LogRecord> records;
    int errno_to_leave = 0;

    std::vector<LogRecord> take() {
        std::lock_guard<std::mutex> lock(mutex);
        std::vector<LogRecord> out;
        out.swap(records);
        return out;
    }
};

extern "C" void internal_recording_sink(void *data, ND_LOG_FIELD_PRIORITY priority, const char *file,
                                        const char *function, unsigned long line, const char *fmt, va_list ap) {
    (void)function;
    const int errno_seen = errno;
    LogRecorder *recorder = static_cast<LogRecorder *>(data);

    char message[4096];
    vsnprintf(message, sizeof(message), fmt, ap);
    {
        std::lock_guard<std::mutex> lock(recorder->mutex);
        recorder->records.push_back({priority, file ? file : "", line, message, errno_seen});
    }

    if (recorder->errno_to_leave)
        errno = recorder->errno_to_leave;
}

class LogLayer : public ::testing::Test {
protected:
    void SetUp() override {
        recorder_ = new LogRecorder;
        struct dbengine_config cfg = netdata_test_config();
        cfg.log_sink = internal_recording_sink;
        cfg.log_sink_data = recorder_;
        engine_ = dbengine_engine_alloc(&cfg);
        ASSERT_NE(engine_, nullptr);
    }

    void TearDown() override {
        if (engine_)
            dbengine_engine_free(engine_);
    }

    // the one record a single emission produced, checked for its priority and for the line that emitted it
    void expect_one(ND_LOG_FIELD_PRIORITY priority, unsigned long line, const char *text) {
        const std::vector<LogRecord> records = recorder_->take();
        ASSERT_EQ(records.size(), 1u) << "expected exactly one line for \"" << text << "\"";
        EXPECT_EQ(records[0].priority, priority) << text;
        EXPECT_EQ(records[0].line, line) << text;
        EXPECT_NE(records[0].file.find("internal_log_sink.cc"), std::string::npos) << records[0].file;
        EXPECT_NE(records[0].text.find(text), std::string::npos) << records[0].text;
    }

    LogRecorder *recorder_ = nullptr;
    struct dbengine_engine *engine_ = nullptr;
};

// Opens a local limiter's window as of now. The gate measures the window on the boot clock from last_logged, so a
// limiter left at 0 would still be inside its first window on a host up for less than log_every - a fresh CI runner
// - and drop the very line a case expects.
void open_window(ERROR_LIMIT *erl) {
    erl->last_logged = now_boottime_sec() - erl->log_every;
}

// Runs one emission with stderr sent to a file, and returns what reached stderr: netdata's logger writes there in
// this binary, so a line that did not go to a sink shows up here.
template <typename F> std::string stderr_of(F emit) {
    const char *root = getenv("TMPDIR");
    std::string path = std::string(root && *root ? root : "/tmp") + "/dbengine-test-stderr-XXXXXX";
    const int fd = mkstemp(path.data());
    if (fd < 0)
        return "<no temporary file>";

    fflush(stderr);
    const int saved = dup(STDERR_FILENO);
    dup2(fd, STDERR_FILENO);
    emit();
    fflush(stderr);
    dup2(saved, STDERR_FILENO);
    close(saved);

    std::string out;
    char buf[4096];
    lseek(fd, 0, SEEK_SET);
    for (ssize_t n; (n = read(fd, buf, sizeof(buf))) > 0;)
        out.append(buf, static_cast<size_t>(n));
    close(fd);
    unlink(path.c_str());
    return out;
}

} // namespace

TEST_F(LogLayer, EachFamilyHandsTheSinkItsPriorityAndTheLineThatEmitted) {
    unsigned long line;

    line = __LINE__; dbengine_log_error(engine_, "family error %d", 1);
    expect_one(NDLP_ERR, line, "family error 1");

    line = __LINE__; dbengine_log_info(engine_, "family info %s", "x");
    expect_one(NDLP_INFO, line, "family info x");

    line = __LINE__; dbengine_log(engine_, NDLP_WARNING, "family log %d", 3);
    expect_one(NDLP_WARNING, line, "family log 3");

    line = __LINE__; dbengine_error_report(engine_, "family error_report");
    expect_one(NDLP_ERR, line, "family error_report");

    ERROR_LIMIT erl = {
        .spinlock = SPINLOCK_INITIALIZER, .log_every = 3600, .count = 0, .last_logged = 0, .sleep_ut = 0 };
    open_window(&erl);
    line = __LINE__; dbengine_log_limit(engine_, &erl, NDLP_NOTICE, "family limit");
    expect_one(NDLP_NOTICE, line, "family limit");

    // the teardown narration keeps the trailing newline its no-sink arm, an fprintf, needs
    line = __LINE__; dbengine_progress(engine_, "family progress\n");
    expect_one(NDLP_INFO, line, "family progress\n");

#ifdef NETDATA_INTERNAL_CHECKS
    line = __LINE__; dbengine_internal_error(engine_, true, "family internal_error");
    expect_one(NDLP_DEBUG, line, "family internal_error");

    dbengine_internal_error(engine_, false, "family internal_error not taken");
    EXPECT_TRUE(recorder_->take().empty()) << "an internal_error whose condition is false emitted";

    const uint64_t saved_flags = debug_flags;
    debug_flags |= D_RRDENGINE;
    line = __LINE__; dbengine_log_debug(engine_, D_RRDENGINE, "family debug");
    debug_flags = saved_flags;
    expect_one(NDLP_DEBUG, line, "family debug");

    debug_flags &= ~D_RRDENGINE;
    dbengine_log_debug(engine_, D_RRDENGINE, "family debug not asked for");
    debug_flags = saved_flags;
    EXPECT_TRUE(recorder_->take().empty()) << "a debug line the flags did not ask for emitted";
#endif
}

TEST_F(LogLayer, TheSinkSeesTheSitesErrnoAndTheCallerKeepsItsOwn) {
    recorder_->errno_to_leave = 4242;

    errno = 11;
    dbengine_log_error(engine_, "errno plain");
    EXPECT_EQ(errno, 11) << "the sink's errno leaked to the caller";

    errno = 13;
    ERROR_LIMIT erl = {
        .spinlock = SPINLOCK_INITIALIZER, .log_every = 3600, .count = 0, .last_logged = 0, .sleep_ut = 0 };
    open_window(&erl);
    dbengine_log_limit(engine_, &erl, NDLP_ERR, "errno limited");
    EXPECT_EQ(errno, 13) << "the sink's errno leaked to the caller through the rate-limited family";

    // error_report() clears errno before it logs, with a sink or without one
    errno = 12;
    dbengine_error_report(engine_, "errno error_report");

    const std::vector<LogRecord> records = recorder_->take();
    ASSERT_EQ(records.size(), 3u);
    EXPECT_EQ(records[0].errno_seen, 11) << "the sink did not see the emitting site's errno";
    EXPECT_EQ(records[1].errno_seen, 13) << "the sink did not see the emitting site's errno on the limited path";
    EXPECT_EQ(records[2].errno_seen, 0) << "error_report's errno clear did not happen before the sink";
}

TEST_F(LogLayer, TheSinkSeesTheSitesErrnoWhenTheLimiterWasContended) {
    // Another thread inside the site's gate: the call spins on the limiter's lock, and the spin's microsleep clears
    // errno on the way. The sink must still see the errno the site had.
    ERROR_LIMIT erl = {
        .spinlock = SPINLOCK_INITIALIZER, .log_every = 3600, .count = 0, .last_logged = 0, .sleep_ut = 0 };
    open_window(&erl);
    std::atomic<bool> held{false};
    std::thread holder([&] {
        spinlock_lock(&erl.spinlock);
        held = true;
        std::this_thread::sleep_for(std::chrono::milliseconds(50));
        spinlock_unlock(&erl.spinlock);
    });
    while (!held)
        std::this_thread::yield();

    errno = 13;
    dbengine_log_limit(engine_, &erl, NDLP_ERR, "errno contended");
    holder.join();

    const std::vector<LogRecord> records = recorder_->take();
    ASSERT_EQ(records.size(), 1u);
    EXPECT_EQ(records[0].errno_seen, 13) << "the sink saw what the contended lock left in errno, not the site's";
}

TEST_F(LogLayer, ARateLimitedSiteEmitsOncePerWindow) {
    ERROR_LIMIT erl = {
        .spinlock = SPINLOCK_INITIALIZER, .log_every = 3600, .count = 0, .last_logged = 0, .sleep_ut = 0 };
    open_window(&erl);
    for (int i = 0; i < 5; i++)
        dbengine_log_limit(engine_, &erl, NDLP_ERR, "limited %d", i);

    const std::vector<LogRecord> records = recorder_->take();
    ASSERT_EQ(records.size(), 1u) << "five attempts inside one open window of an hour did not produce exactly one line";
    EXPECT_NE(records[0].text.find("limited 0"), std::string::npos);
    EXPECT_EQ(erl.count, 4u) << "the four dropped attempts were not counted";

    erl.last_logged -= erl.log_every; // the window has passed
    dbengine_log_limit(engine_, &erl, NDLP_ERR, "limited again");
    EXPECT_EQ(recorder_->take().size(), 1u) << "the next window did not let a line through";
    EXPECT_EQ(erl.count, 0u);
}

TEST_F(LogLayer, WithoutASinkTheLineGoesToTheLogger) {
    struct dbengine_config plain = netdata_test_config();
    plain.log_sink = nullptr;
    plain.log_sink_data = nullptr;
    struct dbengine_engine *no_sink = dbengine_engine_alloc(&plain);
    ASSERT_NE(no_sink, nullptr);

    const std::string out = stderr_of([&] {
        dbengine_log_error(no_sink, "no sink error line");
        dbengine_log_error(nullptr, "no engine error line");
    });
    dbengine_engine_free(no_sink);

    EXPECT_NE(out.find("no sink error line"), std::string::npos) << "netdata's logger did not get the line: " << out;
    EXPECT_NE(out.find("no engine error line"), std::string::npos) << "netdata's logger did not get the line: " << out;
}

TEST_F(LogLayer, TheAllocatorNoticeReachesTheEngineWhoseSettingsWereIgnored) {
    // the layer is process-wide and keeps what it was first given; make sure it is up, then ask for something else
    const struct dbengine_config cfg = netdata_test_config();
    pgd_init_arals(nullptr, &cfg.allocator);
    recorder_->take();

    struct dbengine_config other = netdata_test_config();
    other.allocator.partitions = cfg.allocator.partitions + 3;
    pgd_init_arals(engine_, &other.allocator);

    const std::vector<LogRecord> records = recorder_->take();
    ASSERT_EQ(records.size(), 1u) << "the engine whose allocator settings were ignored was not told";
    EXPECT_EQ(records[0].priority, NDLP_NOTICE);
}
