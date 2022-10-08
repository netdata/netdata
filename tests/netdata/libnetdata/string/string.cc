#include <daemon/common.h>
#include <gtest/gtest.h>

TEST(string, tests) {
    STRING *s1 = string_strdupz("hello unittest");
    STRING *s2 = string_strdupz("hello unittest");
    EXPECT_EQ(s1, s2);

    STRING *s3 = string_dup(s1);
    EXPECT_EQ(s1, s3);

    STRING *s4 = string_strdupz("world unittest");
    EXPECT_NE(s1, s4);

    string_freez(s1);
    string_freez(s2);
    string_freez(s3);
    string_freez(s4);
}

TEST(string, two_merge) {
    struct TestCase {
        const char *src1;
        const char *src2;
        const char *expected;
    };

    std::vector<TestCase> V = {
        { "", "", ""},
        { "a", "", "[x]"},
        { "", "a", "[x]"},
        { "a", "a", "a"},
        { "abcd", "abcd", "abcd"},
        { "foo_cs", "bar_cs", "[x]_cs"},
        { "cp_UNIQUE_INFIX_cs", "cp_unique_infix_cs", "cp_[x]_cs"},
        { "cp_UNIQUE_INFIX_ci_unique_infix_cs", "cp_unique_infix_ci_UNIQUE_INFIX_cs", "cp_[x]_cs"},
        { "foo[1234]", "foo[4321]", "foo[[x]]"},
    };

    for (const auto &TC : V) {
        STRING *src1 = string_strdupz(TC.src1);
        STRING *src2 = string_strdupz(TC.src2);
        STRING *expected = string_strdupz(TC.expected);

        STRING *result = string_2way_merge(src1, src2);
        EXPECT_EQ(string_cmp(result, expected), 0);

        string_freez(src1);
        string_freez(src2);
        string_freez(expected);
        string_freez(result);
    }
}

struct thread_unittest {
    int join;
    int dups;
};

static void *string_thread(void *arg) {
    struct thread_unittest *tu = static_cast<struct thread_unittest *>(arg);

    for(; 1 ;) {
        if(__atomic_load_n(&tu->join, __ATOMIC_RELAXED))
            break;

        STRING *s = string_strdupz("string thread checking 1234567890");

        for(int i = 0; i < tu->dups ; i++)
            string_dup(s);

        for(int i = 0; i < tu->dups ; i++)
            string_freez(s);

        string_freez(s);
    }

    return arg;
}

TEST(string, threads) {
    struct thread_unittest tu = {
        .join = 0,
        .dups = 1,
    };

    time_t seconds_to_run = 1;
    int threads_to_create = 2;

    struct string_statistics string_stats_before;
    string_get_statistics(&string_stats_before);

    netdata_thread_t threads[threads_to_create];
    for (int i = 0; i < threads_to_create; i++) {
        char buf[100 + 1];
        snprintf(buf, 100, "string%d", i);

        NETDATA_THREAD_OPTIONS options = static_cast<NETDATA_THREAD_OPTIONS>(
            NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_JOINABLE
        );
        netdata_thread_create(
            &threads[i], buf, options, string_thread, &tu);
    }
    sleep_usec(seconds_to_run * USEC_PER_SEC);

    __atomic_store_n(&tu.join, 1, __ATOMIC_RELAXED);
    for (int i = 0; i < threads_to_create; i++) {
        void *retval;
        netdata_thread_join(threads[i], &retval);
    }

    struct string_statistics string_stats_after;
    string_get_statistics(&string_stats_after);

    EXPECT_EQ(string_stats_after.references, string_stats_before.references);
    EXPECT_EQ(string_stats_after.memory, string_stats_before.memory);
    EXPECT_EQ(string_stats_after.entries, string_stats_before.entries);
}
