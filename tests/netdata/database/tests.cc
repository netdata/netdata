#include <gtest/gtest.h>
#include "daemon/common.h"
#include "ml/json/single_include/nlohmann/json.hpp"

struct feed_values {
    usec_t microseconds;
    collected_number value;
};

struct test {
    char name[100];

    int update_every;
    collected_number multiplier;
    collected_number divisor;
    RRD_ALGORITHM algorithm;

    size_t feed_entries;
    size_t result_entries;

    const struct feed_values *feed1;
    const NETDATA_DOUBLE *results1;

    const collected_number *feed2;
    const NETDATA_DOUBLE *results2;
};

// test1: absolute values stored at exactly second boundaries
static const struct feed_values test1_feed[] = {
    { 0, 10 },
    { 1000000, 20 },
    { 1000000, 30 },
    { 1000000, 40 },
    { 1000000, 50 },
    { 1000000, 60 },
    { 1000000, 70 },
    { 1000000, 80 },
    { 1000000, 90 },
    { 1000000, 100 },
};

static const NETDATA_DOUBLE test1_results[] = {
    20, 30, 40, 50, 60, 70, 80, 90, 100
};

static const struct test test1 = {
    "test1",            // name
    1,                  // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_ABSOLUTE,    // algorithm
    10,                 // feed entries
    9,                  // result entries
    test1_feed,         // feed
    test1_results,      // results
    NULL,               // feed2
    NULL                // results2
};

// test2: absolute values stored in the middle of second boundaries
static const struct feed_values test2_feed[] = {
    { 500000, 10 },
    { 1000000, 20 },
    { 1000000, 30 },
    { 1000000, 40 },
    { 1000000, 50 },
    { 1000000, 60 },
    { 1000000, 70 },
    { 1000000, 80 },
    { 1000000, 90 },
    { 1000000, 100 },
};

static const NETDATA_DOUBLE test2_results[] = {
    20, 30, 40, 50, 60, 70, 80, 90, 100
};

static const struct test test2 = {
    "test2",            // name
    1,                  // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_ABSOLUTE,    // algorithm
    10,                 // feed entries
    9,                  // result entries
    test2_feed,         // feed
    test2_results,      // results
    NULL,               // feed2
    NULL                // results2
};

// test3: incremental values stored at exactly second boundaries
static const struct feed_values test3_feed[] = {
    { 0, 10 },
    { 1000000, 20 },
    { 1000000, 30 },
    { 1000000, 40 },
    { 1000000, 50 },
    { 1000000, 60 },
    { 1000000, 70 },
    { 1000000, 80 },
    { 1000000, 90 },
    { 1000000, 100 },
};

static const NETDATA_DOUBLE test3_results[] = {
    10, 10, 10, 10, 10, 10, 10, 10, 10
};

static const struct test test3 = {
    "test3",            // name
    1,                  // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_INCREMENTAL, // algorithm
    10,                 // feed entries
    9,                  // result entries
    test3_feed,         // feed
    test3_results,      // results
    NULL,               // feed2
    NULL                // results2
};

// test4: incremental values stored in the middle of second boundaries
static const struct feed_values test4_feed[] = {
    { 500000, 10 },
    { 1000000, 20 },
    { 1000000, 30 },
    { 1000000, 40 },
    { 1000000, 50 },
    { 1000000, 60 },
    { 1000000, 70 },
    { 1000000, 80 },
    { 1000000, 90 },
    { 1000000, 100 },
};

NETDATA_DOUBLE test4_results[] = {
    10, 10, 10, 10, 10, 10, 10, 10, 10
};

struct test test4 = {
    "test4",            // name
    1,                  // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_INCREMENTAL, // algorithm
    10,                 // feed entries
    9,                  // result entries
    test4_feed,         // feed
    test4_results,      // results
    NULL,               // feed2
    NULL                // results2
};

// test5: 32-bit incremental values overflow
struct feed_values test5_feed[] = {
    { 0,       0x00000000FFFFFFFFULL / 15 * 0 },
    { 1000000, 0x00000000FFFFFFFFULL / 15 * 7 },
    { 1000000, 0x00000000FFFFFFFFULL / 15 * 14 },
    { 1000000, 0x00000000FFFFFFFFULL / 15 * 0 },
    { 1000000, 0x00000000FFFFFFFFULL / 15 * 7 },
    { 1000000, 0x00000000FFFFFFFFULL / 15 * 14 },
    { 1000000, 0x00000000FFFFFFFFULL / 15 * 0 },
    { 1000000, 0x00000000FFFFFFFFULL / 15 * 7 },
    { 1000000, 0x00000000FFFFFFFFULL / 15 * 14 },
    { 1000000, 0x00000000FFFFFFFFULL / 15 * 0 },
};

NETDATA_DOUBLE test5_results[] = {
    static_cast<NETDATA_DOUBLE>(0x00000000FFFFFFFFULL) / 15 * 7,
    static_cast<NETDATA_DOUBLE>(0x00000000FFFFFFFFULL) / 15 * 7,
    static_cast<NETDATA_DOUBLE>(0x00000000FFFFFFFFULL) / 15,
    static_cast<NETDATA_DOUBLE>(0x00000000FFFFFFFFULL) / 15 * 7,
    static_cast<NETDATA_DOUBLE>(0x00000000FFFFFFFFULL) / 15 * 7,
    static_cast<NETDATA_DOUBLE>(0x00000000FFFFFFFFULL) / 15,
    static_cast<NETDATA_DOUBLE>(0x00000000FFFFFFFFULL) / 15 * 7,
    static_cast<NETDATA_DOUBLE>(0x00000000FFFFFFFFULL) / 15 * 7,
    static_cast<NETDATA_DOUBLE>(0x00000000FFFFFFFFULL) / 15,
};

struct test test5 = {
    "test5",            // name
    1,                  // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_INCREMENTAL, // algorithm
    10,                 // feed entries
    9,                  // result entries
    test5_feed,         // feed
    test5_results,      // results
    NULL,               // feed2
    NULL                // results2
};

#if 1
// test5b: 64-bit incremental values overflow
static const struct feed_values test5b_feed[] = {
    { 0,       static_cast<collected_number>(0xFFFFFFFFFFFFFFFFULL / 15 * 0) },
    { 1000000, static_cast<collected_number>(0xFFFFFFFFFFFFFFFFULL / 15 * 7) },
    { 1000000, static_cast<collected_number>(0xFFFFFFFFFFFFFFFFULL / 15 * 14) },
    { 1000000, static_cast<collected_number>(0xFFFFFFFFFFFFFFFFULL / 15 * 0) },
    { 1000000, static_cast<collected_number>(0xFFFFFFFFFFFFFFFFULL / 15 * 7) },
    { 1000000, static_cast<collected_number>(0xFFFFFFFFFFFFFFFFULL / 15 * 14) },
    { 1000000, static_cast<collected_number>(0xFFFFFFFFFFFFFFFFULL / 15 * 0) },
    { 1000000, static_cast<collected_number>(0xFFFFFFFFFFFFFFFFULL / 15 * 7) },
    { 1000000, static_cast<collected_number>(0xFFFFFFFFFFFFFFFFULL / 15 * 14) },
    { 1000000, static_cast<collected_number>(0xFFFFFFFFFFFFFFFFULL / 15 * 0) },
};

static const NETDATA_DOUBLE test5b_results[] = {
    static_cast<NETDATA_DOUBLE>(0xFFFFFFFFFFFFFFFFULL / 15 * 7),
    static_cast<NETDATA_DOUBLE>(0xFFFFFFFFFFFFFFFFULL / 15 * 7),
    static_cast<NETDATA_DOUBLE>(0xFFFFFFFFFFFFFFFFULL / 15),
    static_cast<NETDATA_DOUBLE>(0xFFFFFFFFFFFFFFFFULL / 15 * 7),
    static_cast<NETDATA_DOUBLE>(0xFFFFFFFFFFFFFFFFULL / 15 * 7),
    static_cast<NETDATA_DOUBLE>(0xFFFFFFFFFFFFFFFFULL / 15),
    static_cast<NETDATA_DOUBLE>(0xFFFFFFFFFFFFFFFFULL / 15 * 7),
    static_cast<NETDATA_DOUBLE>(0xFFFFFFFFFFFFFFFFULL / 15 * 7),
    static_cast<NETDATA_DOUBLE>(0xFFFFFFFFFFFFFFFFULL / 15),
};

static const struct test test5b = {
    "test5b",            // name
    1,                  // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_INCREMENTAL, // algorithm
    10,                 // feed entries
    9,                  // result entries
    test5b_feed,        // feed
    test5b_results,     // results
    NULL,               // feed2
    NULL                // results2
};
#endif

// test6: incremental values updated within the same second
static const struct feed_values test6_feed[] = {
    { 250000, 1000 },
    { 250000, 2000 },
    { 250000, 3000 },
    { 250000, 4000 },
    { 250000, 5000 },
    { 250000, 6000 },
    { 250000, 7000 },
    { 250000, 8000 },
    { 250000, 9000 },
    { 250000, 10000 },
    { 250000, 11000 },
    { 250000, 12000 },
    { 250000, 13000 },
    { 250000, 14000 },
    { 250000, 15000 },
    { 250000, 16000 },
};

static const NETDATA_DOUBLE test6_results[] = {
    4000, 4000, 4000, 4000
};

static const struct test test6 = {
    "test6",            // name
    1,                  // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_INCREMENTAL, // algorithm
    16,                 // feed entries
    4,                  // result entries
    test6_feed,         // feed
    test6_results,      // results
    NULL,               // feed2
    NULL                // results2
};

// test7: incremental values updated in long durations
static const struct feed_values test7_feed[] = {
    { 500000, 1000 },
    { 2000000, 2000 },
    { 2000000, 3000 },
    { 2000000, 4000 },
    { 2000000, 5000 },
    { 2000000, 6000 },
    { 2000000, 7000 },
    { 2000000, 8000 },
    { 2000000, 9000 },
    { 2000000, 10000 },
};

static const NETDATA_DOUBLE test7_results[] = {
    500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500
};

static const struct test test7 = {
    "test7",            // name
    1,                  // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_INCREMENTAL, // algorithm
    10,                 // feed entries
    18,                 // result entries
    test7_feed,         // feed
    test7_results,      // results
    NULL,               // feed2
    NULL                // results2
};

// test8: absolute values updated in long durations
static const struct feed_values test8_feed[] = {
    { 500000, 1000 },
    { 2000000, 2000 },
    { 2000000, 3000 },
    { 2000000, 4000 },
    { 2000000, 5000 },
    { 2000000, 6000 },
};

static const NETDATA_DOUBLE test8_results[] = {
    1250, 2000, 2250, 3000, 3250, 4000, 4250, 5000, 5250, 6000
};

static const struct test test8 = {
    "test8",            // name
    1,                  // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_ABSOLUTE,    // algorithm
    6,                  // feed entries
    10,                 // result entries
    test8_feed,         // feed
    test8_results,      // results
    NULL,               // feed2
    NULL                // results2
};

// test9: absolute values updated within the same second
static const struct feed_values test9_feed[] = {
    { 250000, 1000 },
    { 250000, 2000 },
    { 250000, 3000 },
    { 250000, 4000 },
    { 250000, 5000 },
    { 250000, 6000 },
    { 250000, 7000 },
    { 250000, 8000 },
    { 250000, 9000 },
    { 250000, 10000 },
    { 250000, 11000 },
    { 250000, 12000 },
    { 250000, 13000 },
    { 250000, 14000 },
    { 250000, 15000 },
    { 250000, 16000 },
};

static const NETDATA_DOUBLE test9_results[] = {
    4000, 8000, 12000, 16000
};

static const struct test test9 = {
    "test9",            // name
    1,                  // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_ABSOLUTE,    // algorithm
    16,                 // feed entries
    4,                  // result entries
    test9_feed,         // feed
    test9_results,      // results
    NULL,               // feed2
    NULL                // results2
};

// test10: incremental values updated in short and long durations
static const struct feed_values test10_feed[] = {
    { 500000,  1000 },
    { 600000,  1000 +  600 },
    { 200000,  1600 +  200 },
    { 1000000, 1800 + 1000 },
    { 200000,  2800 +  200 },
    { 2000000, 3000 + 2000 },
    { 600000,  5000 +  600 },
    { 400000,  5600 +  400 },
    { 900000,  6000 +  900 },
    { 1000000, 6900 + 1000 },
};

static const NETDATA_DOUBLE test10_results[] = {
    1000, 1000, 1000, 1000, 1000, 1000, 1000
};

static const struct test test10 = {
    "test10",           // name
    1,                  // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_INCREMENTAL, // algorithm
    10,                 // feed entries
    7,                  // result entries
    test10_feed,        // feed
    test10_results,     // results
    NULL,               // feed2
    NULL                // results2
};

// test11: percentage-of-incremental-row with equal values
static const struct feed_values test11_feed1[] = {
    { 0, 10 },
    { 1000000, 20 },
    { 1000000, 30 },
    { 1000000, 40 },
    { 1000000, 50 },
    { 1000000, 60 },
    { 1000000, 70 },
    { 1000000, 80 },
    { 1000000, 90 },
    { 1000000, 100 },
};

static const collected_number test11_feed2[] = {
10, 20, 30, 40, 50, 60, 70, 80, 90, 100
};

static const NETDATA_DOUBLE test11_results1[] = {
    50, 50, 50, 50, 50, 50, 50, 50, 50
};

static const NETDATA_DOUBLE test11_results2[] = {
    50, 50, 50, 50, 50, 50, 50, 50, 50
};

static const struct test test11 = {
    "test11",           // name
    1,                  // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL,   // algorithm
    10,                 // feed entries
    9,                  // result entries
    test11_feed1,       // feed1
    test11_results1,    // results1
    test11_feed2,       // feed2
    test11_results2     // results2
};

// test12: percentage-of-incremental-row with equal values
static const struct feed_values test12_feed1[] = {
    { 0, 10 },
    { 1000000, 20 },
    { 1000000, 30 },
    { 1000000, 40 },
    { 1000000, 50 },
    { 1000000, 60 },
    { 1000000, 70 },
    { 1000000, 80 },
    { 1000000, 90 },
    { 1000000, 100 },
};

static const collected_number test12_feed2[] = {
10*3, 20*3, 30*3, 40*3, 50*3, 60*3, 70*3, 80*3, 90*3, 100*3
};

static const NETDATA_DOUBLE test12_results1[] = {
    25, 25, 25, 25, 25, 25, 25, 25, 25
};

static const NETDATA_DOUBLE test12_results2[] = {
    75, 75, 75, 75, 75, 75, 75, 75, 75
};

static const struct test test12 = {
    "test12",           // name
    1,                  // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL,   // algorithm
    10,                 // feed entries
    9,                  // result entries
    test12_feed1,       // feed
    test12_results1,    // results
    test12_feed2,       // feed2
    test12_results2     // results2
};

// test13: incremental values updated in short and long durations
static const struct feed_values test13_feed[] = {
    { 500000,  1000 },
    { 600000,  1000 +  600 },
    { 200000,  1600 +  200 },
    { 1000000, 1800 + 1000 },
    { 200000,  2800 +  200 },
    { 2000000, 3000 + 2000 },
    { 600000,  5000 +  600 },
    { 400000,  5600 +  400 },
    { 900000,  6000 +  900 },
    { 1000000, 6900 + 1000 },
};

static const NETDATA_DOUBLE test13_results[] = {
    83.3333300, 100, 100, 100, 100, 100, 100
};

static const struct test test13 = {
    "test13",           // name
    1,                  // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL,   // algorithm
    10,                 // feed entries
    7,                  // result entries
    test13_feed,        // feed
    test13_results,     // results
    NULL,               // feed2
    NULL                // results2
};

// test14: issue #981 with real data
static const struct feed_values test14_feed[] = {
    {        0, 0x015397dc42151c41ULL },
    { 13573000, 0x015397e612e3ff5dULL },
    { 29969000, 0x015397f905ecdaa8ULL },
    { 29958000, 0x0153980c2a6cb5e4ULL },
    { 30054000, 0x0153981f4032fb83ULL },
    { 34952000, 0x015398355efadaccULL },
    { 25046000, 0x01539845ba4b09f8ULL },
    { 29947000, 0x0153985948bf381dULL },
    { 30054000, 0x0153986c5b9c27e2ULL },
    { 29942000, 0x0153987f888982d0ULL },
};

static const NETDATA_DOUBLE test14_results[] = {
    23.1383300, 21.8515600, 21.8804600, 21.7788000, 22.0112200, 22.4386100, 22.0906100, 21.9150800
};

static const struct test test14 = {
    "test14",            // name
    30,                 // update_every
    8,                  // multiplier
    1000000000,         // divisor
    RRD_ALGORITHM_INCREMENTAL, // algorithm
    10,                 // feed entries
    8,                  // result entries
    test14_feed,        // feed
    test14_results,     // results
    NULL,               // feed2
    NULL                // results2
};

// test14b: issue #981 with dummy data
static const struct feed_values test14b_feed[] = {
    {        0, 0 },
    { 13573000, 13573000 },
    { 29969000, 13573000 + 29969000 },
    { 29958000, 13573000 + 29969000 + 29958000 },
    { 30054000, 13573000 + 29969000 + 29958000 + 30054000 },
    { 34952000, 13573000 + 29969000 + 29958000 + 30054000 + 34952000 },
    { 25046000, 13573000 + 29969000 + 29958000 + 30054000 + 34952000 + 25046000 },
    { 29947000, 13573000 + 29969000 + 29958000 + 30054000 + 34952000 + 25046000 + 29947000 },
    { 30054000, 13573000 + 29969000 + 29958000 + 30054000 + 34952000 + 25046000 + 29947000 + 30054000 },
    { 29942000, 13573000 + 29969000 + 29958000 + 30054000 + 34952000 + 25046000 + 29947000 + 30054000 + 29942000 },
};

static const NETDATA_DOUBLE test14b_results[] = {
    1000000, 1000000, 1000000, 1000000, 1000000, 1000000, 1000000, 1000000
};

static const struct test test14b = {
    "test14b",            // name
    30,                 // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_INCREMENTAL, // algorithm
    10,                 // feed entries
    8,                  // result entries
    test14b_feed,        // feed
    test14b_results,     // results
    NULL,               // feed2
    NULL                // results2
};

// test14c: issue #981 with dummy data, checking for late start
static const struct feed_values test14c_feed[] = {
    { 29000000, 29000000 },
    {  1000000, 29000000 + 1000000 },
    { 30000000, 29000000 + 1000000 + 30000000 },
    { 30000000, 29000000 + 1000000 + 30000000 + 30000000 },
    { 30000000, 29000000 + 1000000 + 30000000 + 30000000 + 30000000 },
    { 30000000, 29000000 + 1000000 + 30000000 + 30000000 + 30000000 + 30000000 },
    { 30000000, 29000000 + 1000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 },
    { 30000000, 29000000 + 1000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 },
    { 30000000, 29000000 + 1000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 },
    { 30000000, 29000000 + 1000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 },
};

static const NETDATA_DOUBLE test14c_results[] = {
    1000000, 1000000, 1000000, 1000000, 1000000, 1000000, 1000000, 1000000, 1000000
};

static const struct test test14c = {
    "test14c",            // name
    30,                 // update_every
    1,                  // multiplier
    1,                  // divisor
    RRD_ALGORITHM_INCREMENTAL, // algorithm
    10,                 // feed entries
    9,                  // result entries
    test14c_feed,        // feed
    test14c_results,     // results
    NULL,               // feed2
    NULL                // results2
};

// test15: "test incremental with 2 dimensions",
static const struct feed_values test15_feed1[] = {
    {       0, 1068066388 },
    { 1008752, 1068822698 },
    {  993809, 1069573072 },
    {  995911, 1070324135 },
    { 1014562, 1071078166 },
    {  994684, 1071831349 },
    {  993128, 1072235739 },
    { 1010332, 1072958871 },
    { 1003394, 1073707019 },
    {  995201, 1074460255 },
};

static const collected_number test15_feed2[] = {
    178825286, 178825286, 178825286, 178825286, 178825498, 178825498, 179165652, 179202964, 179203282, 179204130
};

static const NETDATA_DOUBLE test15_results1[] = {
    5857.4080000, 5898.4540000, 5891.6590000, 5806.3160000, 5914.2640000, 3202.2630000, 5589.6560000, 5822.5260000, 5911.7520000
};

static const NETDATA_DOUBLE test15_results2[] = {
    0.0000000, 0.0000000, 0.0024944, 1.6324779, 0.0212777, 2655.1890000, 290.5387000, 5.6733610, 6.5960220
};

static const struct test test15 = {
        "test15",           // name
        1,                  // update_every
        8,                  // multiplier
        1024,               // divisor
        RRD_ALGORITHM_INCREMENTAL, // algorithm
        10,                 // feed entries
        9,                  // result entries
        test15_feed1,       // feed
        test15_results1,    // results
        test15_feed2,       // feed2
        test15_results2     // results2
};

static void run_test(const struct test *test) {
    default_rrd_memory_mode = RRD_MEMORY_MODE_ALLOC;
    default_rrd_update_every = test->update_every;

    char name[101];
    snprintfz(name, 100, "unittest-%s", test->name);

    RRDSET *st = rrdset_create_localhost("netdata", name, name, "netdata", NULL, "Unit Testing", "a value", "unittest", NULL, 1,
                                         test->update_every, RRDSET_TYPE_LINE);
    RRDDIM *rd1 = rrddim_add(st, "dim1", NULL, test->multiplier, test->divisor, test->algorithm);
    RRDDIM *rd2 = (test->feed2) ? rrddim_add(st, "dim2", NULL, test->multiplier, test->divisor, test->algorithm) : NULL;

    for (size_t i = 0; i < test->feed_entries; i++) {
        if (i)
            st->usec_since_last_update = test->feed1[i].microseconds;

        rrddim_set(st, "dim1", test->feed1[i].value);
        if (rd2)
            rrddim_set(st, "dim2", test->feed2[i]);

        rrdset_done(st);

        if (!i) {
            // align the first entry to second boundary
            rd1->last_collected_time.tv_usec = test->feed1[i].microseconds;
            st->last_collected_time.tv_usec = test->feed1[i].microseconds;
            st->last_updated.tv_usec = test->feed1[i].microseconds;
        }
    }
    EXPECT_EQ(st->counter, test->result_entries);

    size_t N = std::min(st->counter, test->result_entries);
    for (size_t i = 0; i != N; i++) {
        NETDATA_DOUBLE v = unpack_storage_number(rd1->db[i]);
        NETDATA_DOUBLE n = unpack_storage_number(pack_storage_number(test->results1[i], SN_DEFAULT_FLAGS));

        EXPECT_NEAR(v, n, 1.0e-7);

        if (rd2) {
            v = unpack_storage_number(rd2->db[i]);
            n = test->results2[i];

            EXPECT_NEAR(v, n, 1.0e-7);
        }
    }
}

TEST(database, round_robin) {
    run_test(&test1);
    run_test(&test2);
    run_test(&test3);
    run_test(&test4);
    run_test(&test5);
    run_test(&test5b);
    run_test(&test6);
    run_test(&test7);
    run_test(&test8);
    run_test(&test9);
    run_test(&test10);
    run_test(&test11);
    run_test(&test12);
    run_test(&test13);
    run_test(&test14);
    run_test(&test14b);
    run_test(&test14c);
    run_test(&test15);
}

TEST(database, renaming) {
   RRDSET *RS = rrdset_create_localhost("chart", "ID", NULL, "family", "context",
                                        "Unit Testing", "a value", "unittest",
                                        NULL, 1, 1, RRDSET_TYPE_LINE);

   RRDDIM *RD1 = rrddim_add(RS, "DIM1", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
   RRDDIM *RD2 = rrddim_add(RS, "DIM2", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

   rrdset_reset_name(RS, "CHARTNAME1");
   EXPECT_STREQ("chart.CHARTNAME1", rrdset_name(RS));
   rrdset_reset_name(RS, "CHARTNAME2");
   EXPECT_STREQ("chart.CHARTNAME2", rrdset_name(RS));

   rrddim_reset_name(RS, RD1, "DIM1NAME1");
   EXPECT_STREQ("DIM1NAME1", rrddim_name(RD1));
   rrddim_reset_name(RS, RD1, "DIM1NAME2");
   EXPECT_STREQ("DIM1NAME2", rrddim_name(RD1));

   rrddim_reset_name(RS, RD2, "DIM2NAME1");
   EXPECT_STREQ("DIM2NAME1", rrddim_name(RD2));
   rrddim_reset_name(RS, RD2, "DIM2NAME2");
   EXPECT_STREQ("DIM2NAME2", rrddim_name(RD2));

   BUFFER *Buf = buffer_create(1);
   health_api_v1_chart_variables2json(RS, Buf);
   nlohmann::json J = nlohmann::json::parse(buffer_tostring(Buf));
   buffer_free(Buf);

   std::string Chart = J["chart"];
   EXPECT_EQ(Chart, "chart.ID");

   std::string ChartName = J["chart_name"];
   EXPECT_EQ(ChartName, "chart.CHARTNAME2");
}
