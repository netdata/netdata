// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

// page.h is the only private header with an extern "C" of its own, and it wraps the rest of them; cache.h has none,
// so it is wrapped here. See internal_compression.cc for why this file is named internal_.
extern "C" {
#include "database/storage-engines/dbengine/page.h"
#include "database/storage-engines/dbengine/cache.h"
}

#include <utility>
#include <vector>

// The page cache, PGC. The existing test in cache.c carries a FIXME listing twelve behaviours nobody wrote cases
// for; these are those twelve. The cache needs no engine - pgc_create() takes a configuration and a NULL engine is
// explicitly the tests' case - so nothing here brings one up.
//
// Sizes are deliberate: the eviction case needs a cache small enough to fill, and every other case needs one large
// enough that eviction never interferes with what it is measuring.

namespace {

// What the cache did to the pages it owns. The cache calls these from its own threads as well as inline, so they
// are counted atomically and read after the operation that should have caused them.
std::atomic<size_t> g_freed_clean{0};
std::atomic<size_t> g_saved_dirty{0};
std::atomic<size_t> g_save_init_calls{0};

void free_clean_page(PGC *, PGC_ENTRY) {
    g_freed_clean++;
}

// Counts and returns. The saver must NOT call back into the cache for the pages it was handed: the cache is
// holding its own locks while it calls this, and re-entering deadlocks it. The cache's own test callback does
// nothing for the same reason; the engine's real savers do their I/O here and hand the pages back from elsewhere.
void save_dirty_pages(PGC *, PGC_ENTRY *, PGC_PAGE **, size_t entries) {
    g_saved_dirty += entries;
}

void save_init(PGC *, Word_t) {
    g_save_init_calls++;
}

void reset_counters() {
    g_freed_clean = 0;
    g_saved_dirty = 0;
    g_save_init_calls = 0;
}

struct pgc_config test_config(const char *name, size_t clean_size_bytes, size_t max_dirty_pages_per_flush) {
    struct pgc_config cfg = {};

    cfg.name = name;
    cfg.clean_size_bytes = clean_size_bytes;
    cfg.free_clean_cb = free_clean_page;
    cfg.max_dirty_pages_per_flush = max_dirty_pages_per_flush;
    cfg.save_init_cb = save_init;
    cfg.save_dirty_cb = save_dirty_pages;
    cfg.max_pages_per_inline_eviction = 10;
    cfg.max_inline_evictors = 10;
    cfg.max_skip_pages_per_inline_eviction = 1000;
    cfg.max_flushes_inline = 10;
    // The default is a bit-or of two enumerators, which is an int in C++ and needs saying so explicitly.
    cfg.options = static_cast<PGC_OPTIONS>(PGC_OPTIONS_DEFAULT);
    cfg.partitions = 1;
    cfg.additional_bytes_per_page = 0;
    cfg.statistics = true;
    cfg.use_all_ram = false;
    cfg.out_of_memory_protection_bytes = 0;
    cfg.cpus = 2;
    cfg.engine = nullptr; // a cache that belongs to no engine: the tests' case, by the header's own words

    return cfg;
}

// One cache per case, destroyed with it, so no case can be made to pass or fail by another's leftovers.
class PgcTest : public ::testing::Test {
protected:
    void SetUp() override { reset_counters(); }

    void TearDown() override {
        if (cache_) {
            // false means pages were still referenced and the cache had to stay allocated. A case that forgets a
            // release would otherwise leak quietly and still pass.
            EXPECT_TRUE(pgc_destroy(cache_, true)) << "the cache could not be destroyed: pages are still referenced";
            cache_ = nullptr;
        }
    }

    void make_cache(const char *name, size_t clean_size_bytes = 32 * 1024 * 1024,
                    size_t max_dirty_pages_per_flush = 64) {
        const struct pgc_config cfg = test_config(name, clean_size_bytes, max_dirty_pages_per_flush);
        cache_ = pgc_create(&cfg);
        ASSERT_NE(cache_, nullptr);
    }

    PGC_ENTRY entry(Word_t metric_id, time_t start_time_s, time_t end_time_s, bool hot, size_t size = 4096) {
        PGC_ENTRY e = {};

        e.section = 1;
        e.metric_id = metric_id;
        e.start_time_s = start_time_s;
        e.end_time_s = end_time_s;
        e.size = size;
        e.data = nullptr;
        e.update_every_s = 1;
        e.hot = hot;
        e.custom_data = nullptr;

        return e;
    }

    PGC *cache_ = nullptr;
};

} // namespace

// 1. add clean page

TEST_F(PgcTest, AddsACleanPage) {
    make_cache("add-clean");

    bool added = false;
    PGC_PAGE *page = pgc_page_add_and_acquire(cache_, entry(10, 100, 200, false), &added);

    ASSERT_NE(page, nullptr);
    EXPECT_TRUE(added);
    EXPECT_TRUE(pgc_is_page_clean(page));
    EXPECT_EQ(pgc_page_section(page), 1u);
    EXPECT_EQ(pgc_page_metric(page), 10u);
    EXPECT_EQ(pgc_page_start_time_s(page), 100);
    EXPECT_EQ(pgc_page_end_time_s(page), 200);

    const struct dbengine_cache_stats stats = pgc_get_statistics(cache_);
    EXPECT_EQ(stats.entries, 1u);
    EXPECT_EQ(stats.added_entries, 1u);
    EXPECT_EQ(stats.referenced_entries, 1u);

    pgc_page_release(cache_, page);
}

// 2. add clean page again (should not add it)

TEST_F(PgcTest, AddingTheSameCleanPageAgainDoesNotAddIt) {
    make_cache("add-clean-twice");

    bool first_added = false;
    PGC_PAGE *first = pgc_page_add_and_acquire(cache_, entry(10, 100, 200, false), &first_added);
    ASSERT_NE(first, nullptr);
    ASSERT_TRUE(first_added);

    bool second_added = true;
    PGC_PAGE *second = pgc_page_add_and_acquire(cache_, entry(10, 100, 200, false), &second_added);
    ASSERT_NE(second, nullptr);

    EXPECT_FALSE(second_added) << "the cache reported a duplicate page as newly added";
    EXPECT_EQ(first, second) << "the cache made a second page for the same section, metric and time";

    const struct dbengine_cache_stats stats = pgc_get_statistics(cache_);
    EXPECT_EQ(stats.entries, 1u);
    EXPECT_EQ(stats.added_entries, 1u);

    pgc_page_release(cache_, second);
    pgc_page_release(cache_, first);
}

// 3. release page (should decrement counters)

TEST_F(PgcTest, ReleasingAPageDropsItsReference) {
    make_cache("release");

    PGC_PAGE *page = pgc_page_add_and_acquire(cache_, entry(10, 100, 200, false), nullptr);
    ASSERT_NE(page, nullptr);

    EXPECT_EQ(pgc_get_statistics(cache_).referenced_entries, 1u);

    pgc_page_release(cache_, page);

    const struct dbengine_cache_stats stats = pgc_get_statistics(cache_);
    EXPECT_EQ(stats.referenced_entries, 0u) << "the page is still referenced after it was released";
    EXPECT_EQ(stats.entries, 1u) << "releasing a page removed it from the cache";
}

TEST_F(PgcTest, DuplicatingAPageTakesASecondReference) {
    make_cache("dup");

    PGC_PAGE *page = pgc_page_add_and_acquire(cache_, entry(10, 100, 200, false), nullptr);
    ASSERT_NE(page, nullptr);

    PGC_PAGE *dup = pgc_page_dup(cache_, page);
    ASSERT_EQ(dup, page);

    // One entry, two references: releasing one leaves the page referenced.
    EXPECT_EQ(pgc_get_statistics(cache_).referenced_entries, 1u);
    pgc_page_release(cache_, dup);
    EXPECT_EQ(pgc_get_statistics(cache_).referenced_entries, 1u)
        << "releasing one of two references dropped the page";

    pgc_page_release(cache_, page);
    EXPECT_EQ(pgc_get_statistics(cache_).referenced_entries, 0u);
}

// 4. add hot page
// 5. add hot page again (should not add it)

TEST_F(PgcTest, AddsAHotPage) {
    make_cache("add-hot");

    bool added = false;
    PGC_PAGE *page = pgc_page_add_and_acquire(cache_, entry(10, 100, 200, true), &added);

    ASSERT_NE(page, nullptr);
    EXPECT_TRUE(added);
    EXPECT_TRUE(pgc_is_page_hot(page));
    EXPECT_FALSE(pgc_is_page_clean(page));
    EXPECT_FALSE(pgc_is_page_dirty(page));

    pgc_page_hot_to_dirty_and_release(cache_, page, false);
}

TEST_F(PgcTest, AddingTheSameHotPageAgainDoesNotAddIt) {
    make_cache("add-hot-twice");

    bool first_added = false;
    PGC_PAGE *first = pgc_page_add_and_acquire(cache_, entry(10, 100, 200, true), &first_added);
    ASSERT_NE(first, nullptr);
    ASSERT_TRUE(first_added);

    bool second_added = true;
    PGC_PAGE *second = pgc_page_add_and_acquire(cache_, entry(10, 100, 200, true), &second_added);
    ASSERT_NE(second, nullptr);

    EXPECT_FALSE(second_added);
    EXPECT_EQ(first, second);
    EXPECT_EQ(pgc_get_statistics(cache_).entries, 1u);

    pgc_page_release(cache_, second);
    pgc_page_hot_to_dirty_and_release(cache_, first, false);
}

// 6. turn hot page to dirty, with and without a reference counter to it

TEST_F(PgcTest, AHotPageWithNoOtherReferenceBecomesDirty) {
    make_cache("hot-to-dirty");

    PGC_PAGE *page = pgc_page_add_and_acquire(cache_, entry(10, 100, 200, true), nullptr);
    ASSERT_NE(page, nullptr);
    ASSERT_TRUE(pgc_is_page_hot(page));

    pgc_page_hot_to_dirty_and_release(cache_, page, false);

    // hot2dirty_entries is a gauge of pages in the middle of the move, not a count of moves made, so it is back to
    // zero by now. What the page became is the thing to assert, and with no reference left it has to be looked up
    // again to be asked.
    PGC_PAGE *found = pgc_page_get_and_acquire(cache_, 1, 10, 100, PGC_SEARCH_EXACT);
    ASSERT_NE(found, nullptr) << "the page left the cache when the collector let it go";
    EXPECT_FALSE(pgc_is_page_hot(found)) << "the page is still hot after the collector let it go";
    EXPECT_TRUE(pgc_is_page_dirty(found)) << "the page stopped being hot without becoming dirty";
    pgc_page_release(cache_, found);
}

TEST_F(PgcTest, AHotPageStillReferencedElsewhereBecomesDirty) {
    make_cache("hot-to-dirty-referenced");

    PGC_PAGE *page = pgc_page_add_and_acquire(cache_, entry(10, 100, 200, true), nullptr);
    ASSERT_NE(page, nullptr);

    // A second reference, as a query holding the page while the collector finishes with it.
    PGC_PAGE *held = pgc_page_dup(cache_, page);
    ASSERT_EQ(held, page);

    pgc_page_hot_to_dirty_and_release(cache_, page, false);

    // The page is no longer hot even though somebody still holds it.
    EXPECT_FALSE(pgc_is_page_hot(held)) << "a page still referenced stayed hot after the collector let it go";
    EXPECT_TRUE(pgc_is_page_dirty(held));

    pgc_page_release(cache_, held);
}

// 7. dirty pages are saved once there are enough of them

TEST_F(PgcTest, DirtyPagesAreSavedOnceThereAreEnoughOfThem) {
    // A small flush threshold so that a handful of pages reaches it.
    make_cache("flush", 32 * 1024 * 1024, 4);

    for (Word_t metric_id = 1; metric_id <= 8; metric_id++) {
        PGC_PAGE *page = pgc_page_add_and_acquire(cache_, entry(metric_id, 100, 200, true), nullptr);
        ASSERT_NE(page, nullptr);
        pgc_page_hot_to_dirty_and_release(cache_, page, false);
    }

    // Asserted before asking the cache to flush, which is the whole point: the contract is that it saves once
    // enough pages are dirty, and an assertion taken after an explicit flush would hold even if that never
    // happened. Eight pages against a threshold of four is comfortably over the line.
    EXPECT_GT(g_saved_dirty.load(), 0u) << "the cache did not save on its own once enough pages were dirty";
    EXPECT_GT(g_save_init_calls.load(), 0u) << "the saver was never told a flush was starting";

    // Only now, to drain whatever is left so the teardown has nothing to do.
    pgc_flush_pages(cache_);
}

// 8. find page exact
// 9. find page (should return last)
// 10. find page (should return next)

TEST_F(PgcTest, FindsAPageExactly) {
    make_cache("find-exact");

    PGC_PAGE *added = pgc_page_add_and_acquire(cache_, entry(10, 100, 200, false), nullptr);
    ASSERT_NE(added, nullptr);
    pgc_page_release(cache_, added);

    PGC_PAGE *found = pgc_page_get_and_acquire(cache_, 1, 10, 100, PGC_SEARCH_EXACT);
    ASSERT_NE(found, nullptr);
    EXPECT_EQ(pgc_page_start_time_s(found), 100);
    pgc_page_release(cache_, found);

    // A time no page starts at is not an exact match. Released if it ever is, so that a failure here is one
    // failure rather than a failure plus a cache that cannot be destroyed.
    PGC_PAGE *unexpected = pgc_page_get_and_acquire(cache_, 1, 10, 150, PGC_SEARCH_EXACT);
    EXPECT_EQ(unexpected, nullptr);
    if (unexpected)
        pgc_page_release(cache_, unexpected);
}

TEST_F(PgcTest, FindsTheLastPage) {
    make_cache("find-last");

    for (time_t start : {time_t(100), time_t(300), time_t(500)}) {
        PGC_PAGE *page = pgc_page_add_and_acquire(cache_, entry(10, start, start + 100, false), nullptr);
        ASSERT_NE(page, nullptr);
        pgc_page_release(cache_, page);
    }

    // LAST is relative to the time asked for - the newest page at or before it - not the newest page there is, so
    // the search starts from a time after all of them.
    PGC_PAGE *found = pgc_page_get_and_acquire(cache_, 1, 10, 1000, PGC_SEARCH_LAST);
    ASSERT_NE(found, nullptr);
    EXPECT_EQ(pgc_page_start_time_s(found), 500) << "LAST did not return the newest page at or before the time asked";
    pgc_page_release(cache_, found);

    // And from a time between two of them, it is the older one.
    PGC_PAGE *middle = pgc_page_get_and_acquire(cache_, 1, 10, 350, PGC_SEARCH_LAST);
    ASSERT_NE(middle, nullptr);
    EXPECT_EQ(pgc_page_start_time_s(middle), 300);
    pgc_page_release(cache_, middle);
}

TEST_F(PgcTest, FindsTheNextPage) {
    make_cache("find-next");

    for (time_t start : {time_t(100), time_t(300), time_t(500)}) {
        PGC_PAGE *page = pgc_page_add_and_acquire(cache_, entry(10, start, start + 100, false), nullptr);
        ASSERT_NE(page, nullptr);
        pgc_page_release(cache_, page);
    }

    // From a time between two pages, NEXT is the one that starts after it.
    PGC_PAGE *found = pgc_page_get_and_acquire(cache_, 1, 10, 200, PGC_SEARCH_NEXT);
    ASSERT_NE(found, nullptr);
    EXPECT_EQ(pgc_page_start_time_s(found), 300) << "NEXT did not return the page that follows";
    pgc_page_release(cache_, found);
}

TEST_F(PgcTest, FindsNothingForAMetricItDoesNotHold) {
    make_cache("find-missing");

    PGC_PAGE *page = pgc_page_add_and_acquire(cache_, entry(10, 100, 200, false), nullptr);
    ASSERT_NE(page, nullptr);
    pgc_page_release(cache_, page);

    for (const auto &lookup : {std::make_pair(Word_t(1), Word_t(999)), std::make_pair(Word_t(999), Word_t(10))}) {
        SCOPED_TRACE(lookup.first);

        PGC_PAGE *unexpected = pgc_page_get_and_acquire(cache_, lookup.first, lookup.second, 100, PGC_SEARCH_EXACT);
        EXPECT_EQ(unexpected, nullptr) << "a page was found for a metric or section it does not belong to";
        if (unexpected)
            pgc_page_release(cache_, unexpected);
    }
}

// 11. page cache full (should evict)

TEST_F(PgcTest, EvictsWhenItIsFull) {
    // 1 MiB is the smallest a cache may be, and 4 KiB pages fill it quickly. Every page is released, so all of them
    // are evictable.
    make_cache("evict", 1 * 1024 * 1024);

    for (Word_t metric_id = 1; metric_id <= 1024; metric_id++) {
        PGC_PAGE *page = pgc_page_add_and_acquire(cache_, entry(metric_id, 100, 200, false), nullptr);
        ASSERT_NE(page, nullptr);
        pgc_page_release(cache_, page);
    }

    pgc_evict_pages(cache_, 0, 0);

    const struct dbengine_cache_stats stats = pgc_get_statistics(cache_);
    EXPECT_GT(stats.removed_entries, 0u) << "a cache filled well past its size evicted nothing";
    EXPECT_GT(g_freed_clean.load(), 0u) << "the cache evicted pages without telling their owner";
    EXPECT_LT(stats.entries, 1024u) << "every page is still in a cache that cannot hold them all";
}

// 12. on destroy, turn hot pages to dirty and save them

TEST_F(PgcTest, OnDestroyHotPagesAreSaved) {
    make_cache("destroy-saves");

    for (Word_t metric_id = 1; metric_id <= 4; metric_id++) {
        PGC_PAGE *page = pgc_page_add_and_acquire(cache_, entry(metric_id, 100, 200, true), nullptr);
        ASSERT_NE(page, nullptr);
        // Left hot on purpose: the cache has to deal with them itself.
        pgc_page_release(cache_, page);
    }

    ASSERT_EQ(g_saved_dirty.load(), 0u) << "something was saved before the destroy";

    // flush = true: everything still hot must be turned dirty and handed to the saver.
    EXPECT_TRUE(pgc_destroy(cache_, true));
    cache_ = nullptr;

    EXPECT_EQ(g_saved_dirty.load(), 4u) << "destroying the cache did not save the pages that were still hot";
}
