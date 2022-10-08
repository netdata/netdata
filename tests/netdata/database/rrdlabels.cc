#include <gtest/gtest.h>
#include "daemon/common.h"

TEST(rrdlabels, sanitize_values) {
    std::vector<std::pair<const char *, const char *>> V = {
        // 1-byte UTF-8 (ascii)
        { "", "[none]" }, { "1", "1" }, { "  hello   world   ", "hello world" },
        // 2-byte UTF-8
        { " Ελλάδα ", "Ελλάδα" }, { "aŰbŲcŴ", "aŰbŲcŴ" }, { "Ű b Ų c Ŵ", "Ű b Ų c Ŵ" },
        // 3-byte UTF-8
        { "‱", "‱" }, { "a‱b", "a‱b" }, { "a ‱ b", "a ‱ b" },
        // 4-byte UTF-8
        { "𩸽", "𩸽" }, { "a𩸽b", "a𩸽b" }, { "a 𩸽 b", "a 𩸽 b" },
        // mixed multi-byte
        { "Ű‱𩸽‱Ű", "Ű‱𩸽‱Ű" },
    };

    for (const auto &P : V) {
        char buf[1024];
        rrdlabels_sanitize_value(buf, P.first, 1024);

        EXPECT_STREQ(buf, P.second);
    }
}

TEST(rrdlabels, simple_pattern) {
    DICTIONARY *labels = rrdlabels_create();

    rrdlabels_add(labels, "tag1", "value1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels, "tag2", "value2", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels, "tag3", "value3", RRDLABEL_SRC_CONFIG);

    EXPECT_TRUE(rrdlabels_match_simple_pattern(labels, "*"));
    EXPECT_TRUE(rrdlabels_match_simple_pattern(labels, "tag*"));
    EXPECT_TRUE(rrdlabels_match_simple_pattern(labels, "*1"));
    EXPECT_TRUE(rrdlabels_match_simple_pattern(labels, "*=value*"));
    EXPECT_TRUE(rrdlabels_match_simple_pattern(labels, "*:value*"));
    EXPECT_TRUE(rrdlabels_match_simple_pattern(labels, "*2"));
    EXPECT_TRUE(rrdlabels_match_simple_pattern(labels, "*2 *3"));
    EXPECT_TRUE(rrdlabels_match_simple_pattern(labels, "!tag3 *2"));
    EXPECT_TRUE(rrdlabels_match_simple_pattern(labels, "tag1 tag2"));
    EXPECT_TRUE(rrdlabels_match_simple_pattern(labels, "invalid1 invalid2 tag3"));
    EXPECT_TRUE(rrdlabels_match_simple_pattern(labels, "tag1=value1"));
    EXPECT_TRUE(rrdlabels_match_simple_pattern(labels, "tag*=value*"));
    EXPECT_TRUE(rrdlabels_match_simple_pattern(labels, "!tag2=something2 tag2=*2"));

    EXPECT_FALSE(rrdlabels_match_simple_pattern(labels, "tag"));
    EXPECT_FALSE(rrdlabels_match_simple_pattern(labels, "value*"));
    EXPECT_FALSE(rrdlabels_match_simple_pattern(labels, "tag1tag2"));
    EXPECT_FALSE(rrdlabels_match_simple_pattern(labels, "!tag1 tag4"));
    EXPECT_FALSE(rrdlabels_match_simple_pattern(labels, "tag1=value2"));
    EXPECT_FALSE(rrdlabels_match_simple_pattern(labels, "!tag*=value*"));

    rrdlabels_destroy(labels);
}

struct TestEntry {
    const char *Input;
    const char *Key;
    const char *Value;
};

extern "C" int check_name_value_cb(const char *Name, const char *Value, RRDLABEL_SRC LS, void *Data) {
    UNUSED(LS);

    const TestEntry *TE = static_cast<const TestEntry *>(Data);

    EXPECT_STREQ(Name, TE->Key);
    EXPECT_STREQ(Value, TE->Value);

    return 1;
}

TEST(rrdlabels, add_pairs) {
    std::vector<TestEntry> TestEntries = {
        // basic test
        { "tag=value", "tag", "value" },
        { "tag:value", "tag", "value" },

        // test newlines
        { "   tag   = \t value \r\n", "tag", "value" },

        // test : in values
        { "tag=:value", "tag", ":value" },
        { "tag::value", "tag", ":value" },
        { "   tag   =   :value ", "tag", ":value" },
        { "   tag   :   :value ", "tag", ":value" },
        { "tag:5", "tag", "5" },
        { "tag:55", "tag", "55" },
        { "tag:aa", "tag", "aa" },
        { "tag:a", "tag", "a" },

        // test empty values
        { "tag", "tag", "[none]" },
        { "tag:", "tag", "[none]" },
        { "tag:\"\"", "tag", "[none]" },
        { "tag:''", "tag", "[none]" },
        { "tag:\r\n", "tag", "[none]" },
        { "tag\r\n", "tag", "[none]" },

        // test UTF-8 in values
        { "tag: country:Ελλάδα", "tag", "country:Ελλάδα" },
        { "\"tag\": \"country:Ελλάδα\"", "tag", "country:Ελλάδα" },
        { "\"tag\": country:\"Ελλάδα\"", "tag", "country:Ελλάδα" },
        { "\"tag=1\": country:\"Gre\\\"ece\"", "tag_1", "country:Gre_ece" },
        { "\"tag=1\" = country:\"Gre\\\"ece\"", "tag_1", "country:Gre_ece" },

        { "\t'LABE=L'\t=\t\"World\" peace", "labe_l", "World peace" },
        { "\t'LA\\'B:EL'\t=\tcountry:\"World\":\"Europe\":\"Greece\"", "la_b_el", "country:World:Europe:Greece" },
        { "\t'LA\\'B:EL'\t=\tcountry\\\"World\"\\\"Europe\"\\\"Greece\"", "la_b_el", "country/World/Europe/Greece" },

        { "NAME=\"VALUE\"", "name", "VALUE" },
        { "\"NAME\" : \"VALUE\"", "name", "VALUE" },
        { "NAME: \"VALUE\"", "name", "VALUE" },
    };

    for (TestEntry TE : TestEntries) {
        DICTIONARY *labels = rrdlabels_create();

        rrdlabels_add_pair(labels, TE.Input, RRDLABEL_SRC_CONFIG);

        void *Data = static_cast<void *>(&TE);
        int ret = rrdlabels_walkthrough_read(labels, check_name_value_cb, Data);
        EXPECT_EQ(ret, 1);

        rrdlabels_destroy(labels);
    }
}
