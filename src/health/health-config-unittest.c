// SPDX-License-Identifier: GPL-3.0-or-later

#include "health_internals.h"
#include "health-config-unittest.h"
#include "web/api/queries/query.h"

// test case structure for db lookup parsing
typedef struct {
    const char *input;                              // lookup string to parse
    bool should_succeed;                            // expected parsing result
    RRDR_TIME_GROUPING expected_group;              // expected grouping method
    ALERT_LOOKUP_TIME_GROUP_CONDITION expected_cond; // expected condition (for countif)
    NETDATA_DOUBLE expected_value;                  // expected value (for countif/percentile)
    int32_t expected_after;                         // expected after duration in seconds
    int32_t expected_before;                        // expected before (0 or offset)
    const char *description;                        // test description
} db_lookup_test_case_t;

// mark value as "don't care" for tests that don't use it
#define DC_VALUE NAN
#define DC_COND ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL

static const db_lookup_test_case_t test_cases[] = {
    // =========================================================================
    // STOCK CONFIG PATTERNS - These are all patterns from src/health/health.d/*.conf
    // =========================================================================

    // Basic grouping methods with duration
    { "average -10m", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -600, 0, "basic average" },
    { "sum -1m", true, RRDR_GROUPING_SUM, DC_COND, DC_VALUE, -60, 0, "basic sum" },
    { "max -10m", true, RRDR_GROUPING_MAX, DC_COND, DC_VALUE, -600, 0, "basic max" },
    { "min -5m", true, RRDR_GROUPING_MIN, DC_COND, DC_VALUE, -300, 0, "basic min" },
    { "avg -1m", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -60, 0, "avg alias" },

    // Duration variations
    { "average -5s", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -5, 0, "seconds duration" },
    { "average -1h", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -3600, 0, "hour duration" },
    { "average -30s", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -30, 0, "30 seconds" },
    { "average -2h", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -7200, 0, "2 hours" },
    { "average -20m", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -1200, 0, "20 minutes" },

    // With 'at' offset (before parameter)
    { "max -2h at -15m", true, RRDR_GROUPING_MAX, DC_COND, DC_VALUE, -7200, -900, "with at offset" },
    { "min -10m at -50m", true, RRDR_GROUPING_MIN, DC_COND, DC_VALUE, -600, -3000, "min with offset" },
    { "average -1m at -10s", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -60, -10, "avg with small offset" },
    { "max -2m at -1m", true, RRDR_GROUPING_MAX, DC_COND, DC_VALUE, -120, -60, "max with offset" },
    { "average -5m at -5m", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -300, -300, "avg with equal offset" },

    // With 'unaligned' option
    { "average -5s unaligned", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -5, 0, "with unaligned" },
    { "sum -1m unaligned", true, RRDR_GROUPING_SUM, DC_COND, DC_VALUE, -60, 0, "sum unaligned" },
    { "max -10s unaligned", true, RRDR_GROUPING_MAX, DC_COND, DC_VALUE, -10, 0, "max unaligned" },

    // With 'absolute' option
    { "average -1m unaligned absolute", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -60, 0, "with absolute" },
    { "sum -10m unaligned absolute", true, RRDR_GROUPING_SUM, DC_COND, DC_VALUE, -600, 0, "sum absolute" },

    // With 'percentage' option
    { "average -1m unaligned percentage", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -60, 0, "with percentage" },

    // With 'of' dimension filter
    { "max -10m every 1m of read_errs", true, RRDR_GROUPING_MAX, DC_COND, DC_VALUE, -600, 0, "with of dimension" },
    { "average -10m unaligned of yellow", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -600, 0, "of single dim" },
    { "average -1m unaligned of anomaly_rate", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -60, 0, "of anomaly_rate" },
    { "average -10m unaligned of user,system,softirq,irq,guest", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -600, 0, "of multiple dims" },
    { "average -1m unaligned absolute of !success,*", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -60, 0, "of negated pattern" },
    { "sum -1m unaligned of success", true, RRDR_GROUPING_SUM, DC_COND, DC_VALUE, -60, 0, "sum of dim" },

    // With 'match-names' option
    { "max -1s unaligned match-names of BT,NG", true, RRDR_GROUPING_MAX, DC_COND, DC_VALUE, -1, 0, "with match-names" },
    { "average -10m unaligned match-names of used", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -600, 0, "avg match-names" },
    { "average -60s unaligned absolute match-names of overwritten", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -60, 0, "absolute match-names" },

    // With 'every' option
    { "max -10m every 1m of read_errs", true, RRDR_GROUPING_MAX, DC_COND, DC_VALUE, -600, 0, "with every" },

    // Complex combinations from stock configs
    { "average -10m unaligned of iowait", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -600, 0, "cpu iowait" },
    { "sum -30m unaligned", true, RRDR_GROUPING_SUM, DC_COND, DC_VALUE, -1800, 0, "ram 30m sum" },
    { "sum -30m unaligned absolute of out", true, RRDR_GROUPING_SUM, DC_COND, DC_VALUE, -1800, 0, "swap out" },
    { "sum -10m unaligned absolute of received", true, RRDR_GROUPING_SUM, DC_COND, DC_VALUE, -600, 0, "net received" },
    { "average -60s unaligned absolute of ListenOverflows", true, RRDR_GROUPING_AVERAGE, DC_COND, DC_VALUE, -60, 0, "tcp listen" },
    { "sum -1m unaligned absolute", true, RRDR_GROUPING_SUM, DC_COND, DC_VALUE, -60, 0, "bcache errors" },

    // =========================================================================
    // PARAMETERIZED AGGREGATION FUNCTIONS
    // =========================================================================

    // countif with comparison operators
    { "countif(>0.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 0.5, -600, 0, "countif greater" },
    { "countif(>=0.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER_EQUAL, 0.5, -600, 0, "countif greater equal" },
    { "countif(<0.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_LESS, 0.5, -600, 0, "countif less" },
    { "countif(<=0.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_LESS_EQUAL, 0.5, -600, 0, "countif less equal" },
    { "countif(!=0.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_NOT_EQUAL, 0.5, -600, 0, "countif not equal" },
    { "countif(<>0.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_NOT_EQUAL, 0.5, -600, 0, "countif not equal alt" },
    { "countif(0.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, 0.5, -600, 0, "countif equal (default)" },

    // countif with integer values
    { "countif(>0) -5m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 0.0, -300, 0, "countif >0" },
    { "countif(>1) -5m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 1.0, -300, 0, "countif >1" },
    { "countif(>100) -5m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 100.0, -300, 0, "countif >100" },

    // countif with decimal starting with dot
    { "countif(>.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 0.5, -600, 0, "countif >.5" },
    { "countif(<.25) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_LESS, 0.25, -600, 0, "countif <.25" },

    // countif with empty parentheses (defaults to =0)
    { "countif() -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, 0.0, -600, 0, "countif default" },

    // countif with options
    { "countif(>2.00) -10m unaligned of *", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 2.0, -600, 0, "countif with opts" },
    { "countif(>0) -1m unaligned absolute", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 0.0, -60, 0, "countif absolute" },

    // percentile variations
    { "percentile(95) -10m", true, RRDR_GROUPING_PERCENTILE, DC_COND, 95.0, -600, 0, "percentile 95" },
    { "percentile(99) -5m", true, RRDR_GROUPING_PERCENTILE, DC_COND, 99.0, -300, 0, "percentile 99" },
    { "percentile(50) -10m", true, RRDR_GROUPING_PERCENTILE, DC_COND, 50.0, -600, 0, "percentile 50 (median)" },
    { "percentile(75) -1h", true, RRDR_GROUPING_PERCENTILE, DC_COND, 75.0, -3600, 0, "percentile 75" },
    { "percentile(90) -10m unaligned", true, RRDR_GROUPING_PERCENTILE, DC_COND, 90.0, -600, 0, "percentile unaligned" },

    // percentile short forms (predefined)
    { "percentile25 -10m", true, RRDR_GROUPING_PERCENTILE25, DC_COND, DC_VALUE, -600, 0, "percentile25" },
    { "percentile50 -10m", true, RRDR_GROUPING_PERCENTILE50, DC_COND, DC_VALUE, -600, 0, "percentile50" },
    { "percentile75 -10m", true, RRDR_GROUPING_PERCENTILE75, DC_COND, DC_VALUE, -600, 0, "percentile75" },
    { "percentile90 -10m", true, RRDR_GROUPING_PERCENTILE90, DC_COND, DC_VALUE, -600, 0, "percentile90" },
    { "percentile95 -10m", true, RRDR_GROUPING_PERCENTILE, DC_COND, DC_VALUE, -600, 0, "percentile95" },
    { "percentile97 -10m", true, RRDR_GROUPING_PERCENTILE97, DC_COND, DC_VALUE, -600, 0, "percentile97" },
    { "percentile98 -10m", true, RRDR_GROUPING_PERCENTILE98, DC_COND, DC_VALUE, -600, 0, "percentile98" },
    { "percentile99 -10m", true, RRDR_GROUPING_PERCENTILE99, DC_COND, DC_VALUE, -600, 0, "percentile99" },

    // trimmed-mean variations
    { "trimmed-mean(5) -10m", true, RRDR_GROUPING_TRIMMED_MEAN, DC_COND, 5.0, -600, 0, "trimmed-mean 5%" },
    { "trimmed-mean(10) -10m", true, RRDR_GROUPING_TRIMMED_MEAN, DC_COND, 10.0, -600, 0, "trimmed-mean 10%" },
    { "trimmed-mean(1.00) -10m", true, RRDR_GROUPING_TRIMMED_MEAN, DC_COND, 1.0, -600, 0, "trimmed-mean 1%" },

    // trimmed-mean short forms (predefined)
    { "trimmed-mean1 -10m", true, RRDR_GROUPING_TRIMMED_MEAN1, DC_COND, DC_VALUE, -600, 0, "trimmed-mean1" },
    { "trimmed-mean2 -10m", true, RRDR_GROUPING_TRIMMED_MEAN2, DC_COND, DC_VALUE, -600, 0, "trimmed-mean2" },
    { "trimmed-mean3 -10m", true, RRDR_GROUPING_TRIMMED_MEAN3, DC_COND, DC_VALUE, -600, 0, "trimmed-mean3" },
    { "trimmed-mean5 -10m", true, RRDR_GROUPING_TRIMMED_MEAN, DC_COND, DC_VALUE, -600, 0, "trimmed-mean5" },
    { "trimmed-mean10 -10m", true, RRDR_GROUPING_TRIMMED_MEAN10, DC_COND, DC_VALUE, -600, 0, "trimmed-mean10" },
    { "trimmed-mean15 -10m", true, RRDR_GROUPING_TRIMMED_MEAN15, DC_COND, DC_VALUE, -600, 0, "trimmed-mean15" },
    { "trimmed-mean20 -10m", true, RRDR_GROUPING_TRIMMED_MEAN20, DC_COND, DC_VALUE, -600, 0, "trimmed-mean20" },
    { "trimmed-mean25 -10m", true, RRDR_GROUPING_TRIMMED_MEAN25, DC_COND, DC_VALUE, -600, 0, "trimmed-mean25" },

    // trimmed-mean with value in parentheses followed by N
    { "trimmed-mean5(1.00) -10m", true, RRDR_GROUPING_TRIMMED_MEAN, DC_COND, 1.0, -600, 0, "trimmed-mean5 with value" },

    // trimmed-median variations
    { "trimmed-median(5) -10m", true, RRDR_GROUPING_TRIMMED_MEDIAN, DC_COND, 5.0, -600, 0, "trimmed-median 5%" },
    { "trimmed-median1 -10m", true, RRDR_GROUPING_TRIMMED_MEDIAN1, DC_COND, DC_VALUE, -600, 0, "trimmed-median1" },
    { "trimmed-median5 -10m", true, RRDR_GROUPING_TRIMMED_MEDIAN, DC_COND, DC_VALUE, -600, 0, "trimmed-median5" },

    // median
    { "median -10m", true, RRDR_GROUPING_MEDIAN, DC_COND, DC_VALUE, -600, 0, "median" },
    { "median -5m unaligned", true, RRDR_GROUPING_MEDIAN, DC_COND, DC_VALUE, -300, 0, "median unaligned" },

    // stddev
    { "stddev -10m", true, RRDR_GROUPING_STDDEV, DC_COND, DC_VALUE, -600, 0, "stddev" },
    { "stddev -5m unaligned", true, RRDR_GROUPING_STDDEV, DC_COND, DC_VALUE, -300, 0, "stddev unaligned" },

    // cv (coefficient of variation)
    { "cv -10m", true, RRDR_GROUPING_CV, DC_COND, DC_VALUE, -600, 0, "cv" },

    // ses (single exponential smoothing)
    { "ses -10m", true, RRDR_GROUPING_SES, DC_COND, DC_VALUE, -600, 0, "ses" },
    { "ema -10m", true, RRDR_GROUPING_SES, DC_COND, DC_VALUE, -600, 0, "ema alias" },

    // des (double exponential smoothing)
    { "des -10m", true, RRDR_GROUPING_DES, DC_COND, DC_VALUE, -600, 0, "des" },

    // incremental-sum
    { "incremental-sum -10m", true, RRDR_GROUPING_INCREMENTAL_SUM, DC_COND, DC_VALUE, -600, 0, "incremental-sum" },

    // extremes
    { "extremes -10m", true, RRDR_GROUPING_EXTREMES, DC_COND, DC_VALUE, -600, 0, "extremes" },

    // =========================================================================
    // ERROR CASES
    // =========================================================================

    // Missing duration
    { "average", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "missing duration" },
    { "sum", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "sum missing duration" },
    { "percentile(95)", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "percentile missing duration" },
    { "countif(>0.5)", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "countif missing duration" },

    // Invalid grouping method
    { "invalid -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "invalid method" },
    { "foo -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "unknown method" },

    // Invalid characters in group options
    { "countif(>abc) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "invalid char in countif" },
    { "percentile(abc) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "invalid char in percentile" },

    // Missing closing parenthesis
    { "countif(>0.5 -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "missing close paren" },
    { "percentile(95 -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "percentile missing paren" },

    // Invalid duration
    { "average -xyz", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "invalid duration" },
    { "average abc", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "non-numeric duration" },

    // Empty input
    { "", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "empty input" },

    // Sentinel
    { NULL, false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, NULL }
};

static int run_db_lookup_test(const db_lookup_test_case_t *test) {
    // create a copy of the input since parsing modifies the string
    char buffer[1024];
    strncpyz(buffer, test->input, sizeof(buffer) - 1);

    struct rrd_alert_config ac = { 0 };
    ac.time_group_value = NAN;

    int result = health_parse_db_lookup(1, "unittest", buffer, &ac);
    bool succeeded = (result != 0);

    // check if success/failure matches expectation
    if(succeeded != test->should_succeed) {
        fprintf(stderr, "FAILED [%s]: expected %s but got %s\n",
                test->description,
                test->should_succeed ? "success" : "failure",
                succeeded ? "success" : "failure");
        return 1;
    }

    // if test should fail, we're done
    if(!test->should_succeed)
        return 0;

    int errors = 0;

    // verify grouping method
    if(ac.time_group != test->expected_group) {
        fprintf(stderr, "FAILED [%s]: expected group %u but got %u\n",
                test->description, (unsigned)test->expected_group, (unsigned)ac.time_group);
        errors++;
    }

    // verify after duration
    if(ac.after != test->expected_after) {
        fprintf(stderr, "FAILED [%s]: expected after %d but got %d\n",
                test->description, test->expected_after, ac.after);
        errors++;
    }

    // verify before (offset) if specified
    if(test->expected_before != 0 && ac.before != test->expected_before) {
        fprintf(stderr, "FAILED [%s]: expected before %d but got %d\n",
                test->description, test->expected_before, ac.before);
        errors++;
    }

    // verify condition for countif
    if(test->expected_cond != DC_COND && ac.time_group_condition != test->expected_cond) {
        fprintf(stderr, "FAILED [%s]: expected condition %d but got %d\n",
                test->description, test->expected_cond, ac.time_group_condition);
        errors++;
    }

    // verify value for countif/percentile/trimmed-mean if specified
    if(!isnan(test->expected_value)) {
        NETDATA_DOUBLE actual_value = isnan(ac.time_group_value) ? 0.0 : ac.time_group_value;
        if(fabsl(actual_value - test->expected_value) > 0.0001) {
            fprintf(stderr, "FAILED [%s]: expected value %f but got %f\n",
                    test->description, test->expected_value, actual_value);
            errors++;
        }
    }

    // cleanup
    string_freez(ac.dimensions);

    return errors;
}

int health_config_unittest(void) {
    int passed = 0;
    int failed = 0;

    // initialize time grouping before running tests
    time_grouping_init();

    fprintf(stderr, "\nStarting health config db lookup parser unit tests\n");
    fprintf(stderr, "===================================================\n\n");

    for(const db_lookup_test_case_t *test = test_cases; test->input != NULL; test++) {
        int errors = run_db_lookup_test(test);
        if(errors == 0) {
            passed++;
        }
        else {
            failed += errors;
        }
    }

    fprintf(stderr, "\n===================================================\n");
    fprintf(stderr, "Health config parser tests: %d passed, %d failed\n\n", passed, failed);

    return failed;
}
