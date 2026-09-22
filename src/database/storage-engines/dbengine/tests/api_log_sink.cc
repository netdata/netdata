// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

#include <atomic>
#include <chrono>
#include <string>
#include <thread>
#include <vector>

// The log sink as an embedder sees it, through the public headers only: what dbengine_log_fn's contract
// (dbengine-config.h) promises about the lines a sink receives, which engine's sink a line goes to, and when the sink
// stops being called. The assertions use lines that are not rate limited: a site's window is process-wide, so a
// limited line may already have been spent by an earlier case. The errno promises are tested in
// internal_log_sink.cc, at the call itself: an engine verb does more work after its last line, so a verb's errno says
// nothing about what the sink left behind.

namespace {

struct LogRecord {
    void *data;
    ND_LOG_FIELD_PRIORITY priority;
    std::string file;
    std::string function;
    unsigned long line;
    std::string text;
};

// What a recording sink saw. Never freed: the contract asks for the sink's data to live as long as the process,
// because an engine left retained by a failing case could still call it.
struct LogRecorder {
    std::mutex mutex;
    std::vector<LogRecord> records;
    std::atomic<bool> closed{false};    // set once the engine is destroyed
    std::atomic<size_t> after_close{0};

    size_t size() {
        std::lock_guard<std::mutex> lock(mutex);
        return records.size();
    }

    std::vector<LogRecord> copy() {
        std::lock_guard<std::mutex> lock(mutex);
        return records;
    }
};

extern "C" void api_recording_sink(void *data, ND_LOG_FIELD_PRIORITY priority, const char *file, const char *function,
                                   unsigned long line, const char *fmt, va_list ap) {
    LogRecorder *recorder = static_cast<LogRecorder *>(data);

    char message[4096];
    vsnprintf(message, sizeof(message), fmt, ap);

    if (recorder->closed.load())
        recorder->after_close++;

    {
        std::lock_guard<std::mutex> lock(recorder->mutex);
        recorder->records.push_back({data, priority, file ? file : "", function ? function : "", line, message});
    }
}

struct dbengine_config config_with(LogRecorder *recorder) {
    struct dbengine_config cfg = netdata_test_config();
    cfg.log_sink = api_recording_sink;
    cfg.log_sink_data = recorder;
    return cfg;
}

struct dbengine_tier_config tier_config(const Scratch &scratch) {
    struct dbengine_tier_config tc = {};
    tc.tier = 0;
    tc.dbfiles_path = scratch.c_str();
    tc.page_type = DBENGINE_PAGE_TYPE_GORILLA_32BIT;
    tc.grouping = 1;
    return tc;
}

// Brings an engine up with tier 0 on the scratch directory and returns it; the assertions end the calling case when
// it does not come up.
DBENGINE_ENGINE *bring_up(const struct dbengine_config &cfg, const Scratch &scratch) {
    DBENGINE_ENGINE *engine = dbengine_create(&cfg);
    EXPECT_NE(engine, nullptr) << "the engine did not come up";
    if (!engine)
        return nullptr;

    const struct dbengine_tier_config tc = tier_config(scratch);
    EXPECT_EQ(dbengine_tier_init(engine, &tc), 0) << "the tier did not come up";
    dbengine_readiness_wait(dbengine_tier(engine, 0));
    return engine;
}

// Every tier exited, then shutdown, then destroy, as the contract requires; returns what destroy reported.
size_t take_down(DBENGINE_ENGINE *engine) {
    DBENGINE_TIER *tier = dbengine_tier(engine, 0);
    if (dbengine_tier_is_active(tier))
        dbengine_tier_exit(tier);
    dbengine_shutdown(engine);
    return dbengine_destroy(engine);
}

bool mentions(const LogRecord &record, const Scratch &scratch) {
    return record.text.find(scratch.c_str()) != std::string::npos;
}

} // namespace

TEST(EngineLogSink, ReceivesTheEnginesLinesWithTheirSites) {
    auto *recorder = new LogRecorder;
    const Scratch scratch;
    ASSERT_TRUE(scratch.valid());

    DBENGINE_ENGINE *engine = bring_up(config_with(recorder), scratch);
    ASSERT_NE(engine, nullptr);
    EXPECT_EQ(take_down(engine), 0u);

    const std::vector<LogRecord> records = recorder->copy();
    ASSERT_FALSE(records.empty()) << "bringing a tier up and down reached the sink with nothing";

    bool saw_the_tier_path = false;
    for (const LogRecord &r : records) {
        EXPECT_EQ(r.data, recorder) << "log_sink_data did not arrive verbatim";
        EXPECT_TRUE(r.priority == NDLP_ERR || r.priority == NDLP_WARNING || r.priority == NDLP_NOTICE ||
                    r.priority == NDLP_INFO || r.priority == NDLP_DEBUG)
            << "a severity outside the five the contract names: " << static_cast<int>(r.priority);
        EXPECT_NE(r.file.find("storage-engines/dbengine/"), std::string::npos)
            << "the site is not an engine file: " << r.file;
        EXPECT_GT(r.line, 0u);
        EXPECT_FALSE(r.function.empty());
        saw_the_tier_path = saw_the_tier_path || mentions(r, scratch);
    }

    // the tier's startup on an empty directory says where it is creating its files, and that line is not limited
    EXPECT_TRUE(saw_the_tier_path) << "no line named the tier's directory";
}

TEST(EngineLogSink, EachEngineHearsOnlyItsOwnLines) {
    auto *recorder_a = new LogRecorder;
    auto *recorder_b = new LogRecorder;
    const Scratch scratch_a;
    const Scratch scratch_b;
    ASSERT_TRUE(scratch_a.valid());
    ASSERT_TRUE(scratch_b.valid());

    DBENGINE_ENGINE *a = bring_up(config_with(recorder_a), scratch_a);
    ASSERT_NE(a, nullptr);
    DBENGINE_ENGINE *b = bring_up(config_with(recorder_b), scratch_b);
    ASSERT_NE(b, nullptr);
    EXPECT_EQ(take_down(a), 0u);
    EXPECT_EQ(take_down(b), 0u);

    size_t a_own = 0, b_own = 0;
    for (const LogRecord &r : recorder_a->copy()) {
        EXPECT_FALSE(mentions(r, scratch_b)) << "engine A's sink got engine B's line: " << r.text;
        a_own += mentions(r, scratch_a);
    }
    for (const LogRecord &r : recorder_b->copy()) {
        EXPECT_FALSE(mentions(r, scratch_a)) << "engine B's sink got engine A's line: " << r.text;
        b_own += mentions(r, scratch_b);
    }

    EXPECT_GT(a_own, 0u) << "engine A's sink never heard about its own directory, so this proves nothing";
    EXPECT_GT(b_own, 0u) << "engine B's sink never heard about its own directory, so this proves nothing";
}

TEST(EngineLogSink, NothingArrivesAfterDestroyReturnsWithNothingRetained) {
    auto *recorder = new LogRecorder;
    const Scratch scratch;
    ASSERT_TRUE(scratch.valid());

    DBENGINE_ENGINE *engine = bring_up(config_with(recorder), scratch);
    ASSERT_NE(engine, nullptr);
    ASSERT_EQ(take_down(engine), 0u);
    ASSERT_GT(recorder->size(), 0u) << "the sink was never called, so its silence afterwards proves nothing";
    recorder->closed = true;

    // every engine thread was joined by the time destroy returned; a late call would come from one that was not
    std::this_thread::sleep_for(std::chrono::milliseconds(200));
    EXPECT_EQ(recorder->after_close.load(), 0u) << "the sink was called after the engine was destroyed";
}
