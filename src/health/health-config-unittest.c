// SPDX-License-Identifier: GPL-3.0-or-later

#include "health_internals.h"
#include "health-config-unittest.h"
#include "web/api/queries/query.h"
#include "web/api/queries/tg-expression.h"
#include "database/sqlite/sqlite_health.h"

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

// mark value as "don't care" for tests that don't use it.
//
// DC_COND has to be OUTSIDE the enum: it used to be EQUAL, which is also
// the value the parser defaults to, so every row that meant "and the
// condition must come out EQUAL" was silently unchecked.
#define DC_VALUE NAN
#define DC_COND ((ALERT_LOOKUP_TIME_GROUP_CONDITION)0xFF)

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
    { "countif(=0.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, 0.5, -600, 0, "countif explicit equal" },
    { "countif(:0.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, 0.5, -600, 0, "countif colon equal" },
    { "countif(==0.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, 0.5, -600, 0, "countif double equal" },
    { "countif(!5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_NOT_EQUAL, 5.0, -600, 0, "countif bang not equal" },

    // countif with integer values
    { "countif(>0) -5m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 0.0, -300, 0, "countif >0" },
    { "countif(>1) -5m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 1.0, -300, 0, "countif >1" },
    { "countif(>100) -5m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 100.0, -300, 0, "countif >100" },

    // countif with decimal starting with dot
    { "countif(>.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 0.5, -600, 0, "countif >.5" },
    { "countif(<.25) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_LESS, 0.25, -600, 0, "countif <.25" },
    { "countif(=.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, 0.5, -600, 0, "countif =.5" },
    { "countif(>=.1) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER_EQUAL, 0.1, -600, 0, "countif >=.1" },
    { "countif(>-.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, -0.5, -600, 0, "countif >-.5" },
    { "countif(<-.25) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_LESS, -0.25, -600, 0, "countif <-.25" },

    // countif with negative numbers
    { "countif(>-3) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, -3.0, -600, 0, "countif >-3" },
    { "countif(>=-3) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER_EQUAL, -3.0, -600, 0, "countif >=-3" },
    { "countif(<-1.5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_LESS, -1.5, -600, 0, "countif <-1.5" },
    { "countif(=-10) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, -10.0, -600, 0, "countif =-10" },

    // countif with explicit positive sign
    { "countif(>+5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 5.0, -600, 0, "countif >+5" },
    { "countif(=+0) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, 0.0, -600, 0, "countif =+0" },

    // countif with scientific notation
    { "countif(>1e-5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 1e-5, -600, 0, "countif scientific" },
    { "countif(<1.5e3) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_LESS, 1500.0, -600, 0, "countif sci positive exp" },
    { "countif(>=1E-10) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER_EQUAL, 1e-10, -600, 0, "countif sci uppercase E" },

    // countif with empty parentheses (defaults to =0)
    { "countif() -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, 0.0, -600, 0, "countif default" },

    // countif with whitespace inside parentheses
    { "countif( >5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 5.0, -600, 0, "countif space before op" },
    { "countif(> 5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 5.0, -600, 0, "countif space after op" },
    { "countif( > 5 ) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 5.0, -600, 0, "countif spaces around" },
    { "countif(  >=  0.5  ) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER_EQUAL, 0.5, -600, 0, "countif multi spaces" },

    // countif with options
    { "countif(>2.00) -10m unaligned of *", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 2.0, -600, 0, "countif with opts" },
    { "countif(>0) -1m unaligned absolute", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 0.0, -60, 0, "countif absolute" },

    // percentile variations
    { "percentile( 95 ) -10m", true, RRDR_GROUPING_PERCENTILE, DC_COND, 95.0, -600, 0, "percentile with spaces" },
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

    // Malformed numeric values (lone dot, sign+dot without digits)
    { "countif(.) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "lone dot invalid" },
    { "countif(+.) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "plus dot invalid" },
    { "countif(-.) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "minus dot invalid" },
    { "countif(>.) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "greater dot invalid" },
    { "countif(<.) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "less dot invalid" },
    { "countif(>=.) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "greater-equal dot invalid" },
    { "countif(<=.) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "less-equal dot invalid" },
    { "countif(>+.) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "greater plus dot invalid" },
    { "countif(<-.) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "less minus dot invalid" },
    { "percentile(.) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "percentile lone dot invalid" },

    // Invalid operator combinations
    { "countif(===5) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "triple equals invalid" },
    { "countif(>==5) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "greater double equals invalid" },
    { "countif(<==5) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "less double equals invalid" },
    { "countif(>::5) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "colon after greater invalid" },
    { "countif(>=:5) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "colon after greater-equal invalid" },
    // `<:` is less-or-equal, the same as `<=`: the query API has always
    // read it that way and health now shares that grammar
    { "countif(<:5) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_LESS_EQUAL, 5.0, -600, 0, "colon after less" },

    // the shared grammar: gap tokens and the predecessor keywords
    { "percentage-of-time(==gap) -10m", true, RRDR_GROUPING_PERCENTAGE_OF_TIME, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, DC_VALUE, -600, 0, "percentage-of-time equals gap" },
    { "percentage-of-time(!=nan) -10m", true, RRDR_GROUPING_PERCENTAGE_OF_TIME, ALERT_LOOKUP_TIME_GROUP_CONDITION_NOT_EQUAL, DC_VALUE, -600, 0, "percentage-of-time not nan" },
    { "number-of-times(<previous) -10m", true, RRDR_GROUPING_NUMBER_OF_TIMES, ALERT_LOOKUP_TIME_GROUP_CONDITION_LESS, DC_VALUE, -600, 0, "number-of-times less than previous" },
    { "number-of-times(<last) -10m", true, RRDR_GROUPING_NUMBER_OF_TIMES, ALERT_LOOKUP_TIME_GROUP_CONDITION_LESS, DC_VALUE, -600, 0, "number-of-times less than last" },
    { "number-of-flaps(>=10) -10m", true, RRDR_GROUPING_NUMBER_OF_FLAPS, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER_EQUAL, 10.0, -600, 0, "number-of-flaps threshold" },
    { "percentage-of-samples(>0) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 0.0, -600, 0, "percentage-of-samples canonical name" },
    { "percentage-of-time(==bogus) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "unknown word operand invalid" },

    // whitespace inside the parentheses is not part of the condition: it is
    // trimmed off both ends before the condition is stored, so writing it
    // out spaced cannot make a different alert out of the same rule
    { "countif( >5 ) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 5.0, -600, 0, "spaced condition" },
    { "percentage-of-time( ==gap ) -10m", true, RRDR_GROUPING_PERCENTAGE_OF_TIME, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, DC_VALUE, -600, 0, "spaced gap token" },

    // the condition grammar belongs to the four condition groupings ONLY.
    // A numeric grouping takes a number, so a WORD operand there is a
    // mistake - accepting it would turn percentile(gap) into percentile 95
    // and average(previous) into a plain average, silently.
    { "percentile(95) -10m", true, RRDR_GROUPING_PERCENTILE, DC_COND, 95, -600, 0, "percentile still takes its number" },
    { "percentile(gap) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "percentile rejects a gap token" },
    { "percentile(previous) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "percentile rejects the predecessor" },
    { "trimmed-mean(gap) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "trimmed-mean rejects a gap token" },
    { "average(previous) -10m", false, RRDR_GROUPING_UNDEFINED, DC_COND, DC_VALUE, 0, 0, "average rejects the predecessor" },

    // an operator in FRONT of the number is a different case: it means
    // nothing to these groupings and this parser has always dropped it, so
    // `percentile(>95)` has always run as percentile 95. Refusing it now
    // would disable alerts that work today, so it is accepted and logged.
    // and the condition it wrote is still recorded, exactly as upstream
    // recorded it: it is hashed into the alert's identity, so dropping it
    // would hand every such alert a new config hash on upgrade
    { "percentile(>95) -10m", true, RRDR_GROUPING_PERCENTILE, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 95, -600, 0, "percentile ignores an operator, keeps the number" },
    { "trimmed-mean(>=10) -10m", true, RRDR_GROUPING_TRIMMED_MEAN, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER_EQUAL, 10, -600, 0, "trimmed-mean ignores an operator, keeps the number" },

    // an empty condition compares equal to zero, the way countif always
    // has. It MUST NOT fall through to the unset legacy value: formatting
    // that produces "nan", which the grammar reads as a GAP token, so the
    // alert would silently become "the share of time with no data"
    { "percentage-of-time() -10m", true, RRDR_GROUPING_PERCENTAGE_OF_TIME, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, 0, -600, 0, "empty condition compares equal to zero" },
    { "number-of-flaps() -10m", true, RRDR_GROUPING_NUMBER_OF_FLAPS, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, 0, -600, 0, "empty flaps condition compares equal to zero" },
    { "number-of-times() -10m", true, RRDR_GROUPING_NUMBER_OF_TIMES, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, 0, -600, 0, "empty times condition compares equal to zero" },

    // Operators with no value (should default to 0)
    { "countif(=) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, 0.0, -600, 0, "equals no value" },
    { "countif(==) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, 0.0, -600, 0, "double equals no value" },
    { "countif(:) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, 0.0, -600, 0, "colon no value" },
    { "countif(>) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, 0.0, -600, 0, "greater no value" },
    { "countif(<) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_LESS, 0.0, -600, 0, "less no value" },
    { "countif(!) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_NOT_EQUAL, 0.0, -600, 0, "bang no value" },
    { "countif(!=) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_NOT_EQUAL, 0.0, -600, 0, "not-equal no value" },
    { "countif(<>) -10m", true, RRDR_GROUPING_COUNTIF, ALERT_LOOKUP_TIME_GROUP_CONDITION_NOT_EQUAL, 0.0, -600, 0, "less-greater no value" },

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

    // every exit frees what the parser allocated, so a run under a leak
    // checker reports only real leaks
    #define run_db_lookup_test_return(rc) do {                              \
        string_freez(ac.dimensions);                                        \
        string_freez(ac.time_group_options);                                \
        return (rc);                                                        \
    } while(0)

    // check if success/failure matches expectation
    if(succeeded != test->should_succeed) {
        fprintf(stderr, "FAILED [%s]: expected %s but got %s\n",
                test->description,
                test->should_succeed ? "success" : "failure",
                succeeded ? "success" : "failure");
        run_db_lookup_test_return(1);
    }

    // if test should fail, we're done
    if(!test->should_succeed)
        run_db_lookup_test_return(0);

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

    run_db_lookup_test_return(errors);
    #undef run_db_lookup_test_return
}

static int test_db_lookup_frequency_boundaries(int *passed) {
    static const struct {
        int after;
        const char *every;
        bool expected_result;
        int expected_every;
        const char *description;
    } tests[] = {
        { INT_MIN, NULL, true, INT_MIN < -INT_MAX ? 0 : INT_MAX, "minimum after uses a representable frequency" },
        { INT_MIN, "1s", true, 1, "minimum after accepts an explicit frequency" },
        { INT_MIN + 1, NULL, true, -(INT_MIN + 1), "adjacent minimum after keeps its magnitude" },
        { -600, "-1s", false, 600, "negative frequency preserves the derived default" },
        { -600, "0s", true, 0, "zero frequency remains the missing-frequency sentinel" },
        { -600, "1s", true, 1, "minimum positive frequency is accepted" },
        { -600, "10s", true, 10, "ordinary positive frequency is accepted" },
        { -1, NULL, true, 1, "ordinary negative after keeps its magnitude" },
        { INT_MAX, NULL, true, INT_MAX, "maximum after keeps its magnitude" },
    };

    int failed = 0;
    for(size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        char buffer[128];
        snprintfz(buffer, sizeof(buffer), "average %ds%s%s",
                  tests[i].after, tests[i].every ? " every " : "", tests[i].every ? tests[i].every : "");

        struct rrd_alert_config ac = { 0 };
        ac.time_group_value = NAN;

        int result = health_parse_db_lookup(1, "unittest", buffer, &ac);
        if((bool)result != tests[i].expected_result || ac.after != tests[i].after ||
           ac.update_every != tests[i].expected_every) {
            fprintf(stderr,
                    "FAILED [%s]: result=%d after=%d update_every=%d\n",
                    tests[i].description, result, ac.after, ac.update_every);
            failed++;
        }
        else
            (*passed)++;

        string_freez(ac.dimensions);
        string_freez(ac.time_group_options);
    }

    return failed;
}

// The condition is STORED as written, and the alert's configuration hash is
// computed over it, so the same rule spaced differently must not become a
// different alert. Whitespace inside the parentheses is trimmed off both
// ends; everything between the operator and the operand is the condition.
static int test_db_lookup_condition_is_trimmed(int *passed) {
    static const struct {
        const char *input;
        const char *expected_options;
        const char *description;
    } tests[] = {
        { "countif( >5 ) -10m", ">5", "spaces on both sides" },
        { "countif(>5) -10m", ">5", "no spaces at all" },
        { "countif(\t>5\t) -10m", ">5", "tabs on both sides" },
        { "percentage-of-time( ==gap ) -10m", "==gap", "a spaced gap token" },
        { "number-of-times( <previous ) -10m", "<previous", "a spaced predecessor" },
        { "countif(> 5) -10m", "> 5", "the space INSIDE the condition is kept" },
    };

    int failed = 0;
    for(size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        char buffer[128];
        strncpyz(buffer, tests[i].input, sizeof(buffer) - 1);

        struct rrd_alert_config ac = { 0 };
        ac.time_group_value = NAN;

        const char *got = NULL;
        if(!health_parse_db_lookup(1, "unittest", buffer, &ac))
            fprintf(stderr, "FAILED [%s]: '%s' did not parse\n", tests[i].description, tests[i].input);

        else if(!(got = ac.time_group_options ? string2str(ac.time_group_options) : NULL) ||
                strcmp(got, tests[i].expected_options) != 0)
            fprintf(stderr, "FAILED [%s]: stored condition '%s', want '%s'\n",
                    tests[i].description, got ? got : "(none)", tests[i].expected_options);

        else {
            (*passed)++;
            string_freez(ac.dimensions);
            string_freez(ac.time_group_options);
            continue;
        }

        failed++;
        string_freez(ac.dimensions);
        string_freez(ac.time_group_options);
    }

    return failed;
}

// A condition the parser cannot read must leave the DEFAULT behind, never
// the part of itself it managed to read. Health refuses such an alert, but
// the query API has always accepted one and answered `==0` - and a half-read
// `>=gap junk` that kept its gap token would silently answer a different
// question from the one `==0` answers.
static int test_expression_rejects_leave_the_default(int *passed) {
    static const struct {
        const char *condition;
        bool valid;
        const char *description;
    } tests[] = {
        { ">=gap junk", false, "a gap token followed by junk" },
        { "<previous and >5", false, "there are no and/or compounds" },
        { ">5 5", false, "a number followed by junk" },
        { ">nonsense", false, "a word that is not an operand" },
        { ".", false, "a lone dot is not a number" },
        { ">=gap", true, "the same condition without the junk (control)" },
        { "!:5", true, "the colon spellings countif has always accepted" },
    };

    int failed = 0;
    for(size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        TG_EXPRESSION e;
        bool ok = tg_expression_parse(&e, tests[i].condition);

        if(ok != tests[i].valid) {
            fprintf(stderr, "FAILED [%s]: '%s' parsed=%s, want %s\n",
                    tests[i].description, tests[i].condition,
                    ok ? "true" : "false", tests[i].valid ? "true" : "false");
            failed++;
            continue;
        }

        if(!ok && (e.cmp != TG_EXPRESSION_EQUAL ||
                   e.operand != TG_EXPRESSION_OPERAND_NUMBER ||
                   e.target != 0.0 ||
                   tg_expression_wants_gaps(&e))) {
            fprintf(stderr, "FAILED [%s]: rejected '%s' left cmp=%d operand=%d target=%f behind, want the ==0 default\n",
                    tests[i].description, tests[i].condition,
                    (int)e.cmp, (int)e.operand, (double)e.target);
            failed++;
            continue;
        }

        (*passed)++;
    }

    return failed;
}

static int test_dyncfg_update_every_boundaries(int *passed) {
    static const struct {
        const char *value_member;
        int expected_code;
        bool expected_every;
        const char *expected_error;
        const char *description;
    } tests[] = {
        { "", HTTP_RESP_OK, false, NULL, "omitted frequency remains optional for user config conversion" },
        { "\"update_every\":0", HTTP_RESP_OK, false, NULL, "zero frequency remains the missing-frequency sentinel" },
        { "\"update_every\":-1", HTTP_RESP_BAD_REQUEST, false, "negative", "negative integer frequency is rejected" },
        { "\"update_every\":\"-1\"", HTTP_RESP_BAD_REQUEST, false, "negative", "negative string frequency is rejected" },
        { "\"update_every\":-4294967295", HTTP_RESP_BAD_REQUEST, false, "negative", "large negative integer cannot wrap positive" },
        { "\"update_every\":\"-4294967295\"", HTTP_RESP_BAD_REQUEST, false, "negative", "large negative string cannot wrap positive" },
        { "\"update_every\":9223372036854775807", HTTP_RESP_BAD_REQUEST, false, "maximum", "out-of-range positive frequency is rejected" },
        { "\"update_every\":1", HTTP_RESP_OK, true, NULL, "minimum positive frequency is accepted" },
        { "\"update_every\":10", HTTP_RESP_OK, true, NULL, "ordinary positive frequency is accepted" },
    };

    int failed = 0;
    for(size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        CLEAN_BUFFER *payload = buffer_create(0, NULL);
        CLEAN_BUFFER *result = buffer_create(0, NULL);

        buffer_sprintf(payload,
                       "{\"format_version\":1,\"rules\":[{\"enabled\":true,\"type\":\"instance\","
                       "\"config\":{\"value\":{%s},\"match\":{\"on\":\"chart\"}}}]}",
                       tests[i].value_member);

        int code = dyncfg_health_cb(NULL, "health:alert:prototype", DYNCFG_CMD_USERCONFIG, "unittest",
                                    payload, NULL, NULL, result, HTTP_ACCESS_NONE, NULL, NULL);
        bool has_every = strstr(buffer_tostring(result), "every: ") != NULL;
        bool has_expected_error = !tests[i].expected_error ||
                                  strstr(buffer_tostring(result), tests[i].expected_error) != NULL;

        if(code != tests[i].expected_code || has_every != tests[i].expected_every ||
           !has_expected_error) {
            fprintf(stderr,
                    "FAILED [%s]: code=%d has_every=%d response='%s'\n",
                    tests[i].description, code, has_every, buffer_tostring(result));
            failed++;
        }
        else
            (*passed)++;
    }

    CLEAN_BUFFER *payload = buffer_create(0, NULL);
    CLEAN_BUFFER *result = buffer_create(0, NULL);
    buffer_strcat(payload,
                  "{\"format_version\":1,\"rules\":["
                  "{\"enabled\":true,\"type\":\"instance\",\"config\":{"
                  "\"value\":{\"update_every\":1},\"match\":{\"on\":\"first\"}}},"
                  "{\"enabled\":true,\"type\":\"instance\",\"config\":{"
                  "\"value\":{\"update_every\":-1},\"match\":{\"on\":\"second\"}}}]}" );

    int code = dyncfg_health_cb(NULL, "health:alert:prototype", DYNCFG_CMD_USERCONFIG, "unittest",
                                payload, NULL, NULL, result, HTTP_ACCESS_NONE, NULL, NULL);
    if(code != HTTP_RESP_BAD_REQUEST || !strstr(buffer_tostring(result), "negative")) {
        fprintf(stderr,
                "FAILED [negative frequency in a later rule is rejected]: code=%d response='%s'\n",
                code, buffer_tostring(result));
        failed++;
    }
    else
        (*passed)++;

    return failed;
}

typedef enum {
    DYNCFG_INTEGER_DB_LOOKUP,
    DYNCFG_INTEGER_DELAY,
    DYNCFG_INTEGER_REPEAT,
} DYNCFG_INTEGER_SECTION;

static int test_dyncfg_integer_destination_boundaries(int *passed) {
    static const struct {
        DYNCFG_INTEGER_SECTION section;
        const char *member;
        const char *value;
        int expected_code;
        const char *description;
        const char *expected_text;
        const char *expected_absent_text;
    } tests[] = {
        { DYNCFG_INTEGER_DB_LOOKUP, "after", "-2147483648", HTTP_RESP_OK, "after accepts INT_MIN", "-2147483648s" },
        { DYNCFG_INTEGER_DB_LOOKUP, "after", "2147483647", HTTP_RESP_OK, "after accepts INT_MAX", "2147483647s" },
        { DYNCFG_INTEGER_DB_LOOKUP, "before", "-2147483648", HTTP_RESP_OK, "before accepts INT_MIN", "at -2147483648s" },
        { DYNCFG_INTEGER_DB_LOOKUP, "before", "2147483647", HTTP_RESP_OK, "before accepts INT_MAX", "at 2147483647s" },
        { DYNCFG_INTEGER_DELAY, "up", "-2147483648", HTTP_RESP_OK, "delay up accepts INT_MIN", "-2147483648s" },
        { DYNCFG_INTEGER_DELAY, "up", "2147483647", HTTP_RESP_OK, "delay up accepts INT_MAX", "2147483647s" },
        { DYNCFG_INTEGER_DELAY, "down", "-2147483648", HTTP_RESP_OK, "delay down accepts INT_MIN", "down -2147483648s" },
        { DYNCFG_INTEGER_DELAY, "down", "2147483647", HTTP_RESP_OK, "delay down accepts INT_MAX", "down 2147483647s" },
        { DYNCFG_INTEGER_DELAY, "max", "-2147483648", HTTP_RESP_OK, "delay max accepts INT_MIN", "max -2147483648s" },
        { DYNCFG_INTEGER_DELAY, "max", "2147483647", HTTP_RESP_OK, "delay max accepts INT_MAX", "max 2147483647s" },
        { DYNCFG_INTEGER_REPEAT, "warning", "0", HTTP_RESP_OK, "warning repeat accepts zero", " off" },
        { DYNCFG_INTEGER_REPEAT, "warning", "2147483647", HTTP_RESP_OK, "warning repeat accepts INT_MAX", "warning 2147483647s" },
        { DYNCFG_INTEGER_REPEAT, "critical", "0", HTTP_RESP_OK, "critical repeat accepts zero", " off" },
        { DYNCFG_INTEGER_REPEAT, "critical", "2147483647", HTTP_RESP_OK, "critical repeat accepts INT_MAX", "critical 2147483647s" },

        { DYNCFG_INTEGER_DB_LOOKUP, "after", "1.5", HTTP_RESP_OK, "after preserves fractional-double truncation", "1s" },
        { DYNCFG_INTEGER_DB_LOOKUP, "after", "true", HTTP_RESP_OK, "after preserves boolean conversion", "1s" },
        { DYNCFG_INTEGER_DB_LOOKUP, "after", "null", HTTP_RESP_OK, "after preserves null conversion", NULL, "lookup" },
        { DYNCFG_INTEGER_DELAY, "up", "-1.5", HTTP_RESP_OK, "delay preserves negative fractional-double truncation", "-1s" },
        { DYNCFG_INTEGER_DELAY, "up", "false", HTTP_RESP_OK, "delay preserves false conversion", NULL, "delay" },
        { DYNCFG_INTEGER_DELAY, "up", "null", HTTP_RESP_OK, "delay preserves null conversion", NULL, "delay" },
        { DYNCFG_INTEGER_REPEAT, "warning", "1.5", HTTP_RESP_OK, "repeat preserves fractional-double truncation", "warning 1s" },
        { DYNCFG_INTEGER_REPEAT, "warning", "true", HTTP_RESP_OK, "repeat preserves boolean conversion", "warning 1s" },
        { DYNCFG_INTEGER_REPEAT, "warning", "null", HTTP_RESP_OK, "repeat preserves null conversion", " off" },

        { DYNCFG_INTEGER_DB_LOOKUP, "after", "2147483648", HTTP_RESP_BAD_REQUEST, "after rejects integer above INT_MAX" },
        { DYNCFG_INTEGER_DB_LOOKUP, "after", "\"2147483648\"", HTTP_RESP_BAD_REQUEST, "after rejects string above INT_MAX" },
        { DYNCFG_INTEGER_DB_LOOKUP, "after", "-2147483649", HTTP_RESP_BAD_REQUEST, "after rejects integer below INT_MIN" },
        { DYNCFG_INTEGER_DB_LOOKUP, "after", "\"-2147483649\"", HTTP_RESP_BAD_REQUEST, "after rejects string below INT_MIN" },
        { DYNCFG_INTEGER_DB_LOOKUP, "before", "2147483648", HTTP_RESP_BAD_REQUEST, "before rejects integer above INT_MAX" },
        { DYNCFG_INTEGER_DB_LOOKUP, "before", "\"2147483648\"", HTTP_RESP_BAD_REQUEST, "before rejects string above INT_MAX" },
        { DYNCFG_INTEGER_DB_LOOKUP, "before", "-2147483649", HTTP_RESP_BAD_REQUEST, "before rejects integer below INT_MIN" },
        { DYNCFG_INTEGER_DB_LOOKUP, "before", "\"-2147483649\"", HTTP_RESP_BAD_REQUEST, "before rejects string below INT_MIN" },

        { DYNCFG_INTEGER_DELAY, "up", "2147483648", HTTP_RESP_BAD_REQUEST, "delay up rejects integer above INT_MAX" },
        { DYNCFG_INTEGER_DELAY, "up", "\"2147483648\"", HTTP_RESP_BAD_REQUEST, "delay up rejects string above INT_MAX" },
        { DYNCFG_INTEGER_DELAY, "up", "-2147483649", HTTP_RESP_BAD_REQUEST, "delay up rejects integer below INT_MIN" },
        { DYNCFG_INTEGER_DELAY, "up", "\"-2147483649\"", HTTP_RESP_BAD_REQUEST, "delay up rejects string below INT_MIN" },
        { DYNCFG_INTEGER_DELAY, "down", "2147483648", HTTP_RESP_BAD_REQUEST, "delay down rejects integer above INT_MAX" },
        { DYNCFG_INTEGER_DELAY, "down", "\"2147483648\"", HTTP_RESP_BAD_REQUEST, "delay down rejects string above INT_MAX" },
        { DYNCFG_INTEGER_DELAY, "down", "-2147483649", HTTP_RESP_BAD_REQUEST, "delay down rejects integer below INT_MIN" },
        { DYNCFG_INTEGER_DELAY, "down", "\"-2147483649\"", HTTP_RESP_BAD_REQUEST, "delay down rejects string below INT_MIN" },
        { DYNCFG_INTEGER_DELAY, "max", "2147483648", HTTP_RESP_BAD_REQUEST, "delay max rejects integer above INT_MAX" },
        { DYNCFG_INTEGER_DELAY, "max", "\"2147483648\"", HTTP_RESP_BAD_REQUEST, "delay max rejects string above INT_MAX" },
        { DYNCFG_INTEGER_DELAY, "max", "-2147483649", HTTP_RESP_BAD_REQUEST, "delay max rejects integer below INT_MIN" },
        { DYNCFG_INTEGER_DELAY, "max", "\"-2147483649\"", HTTP_RESP_BAD_REQUEST, "delay max rejects string below INT_MIN" },

        { DYNCFG_INTEGER_REPEAT, "warning", "-1", HTTP_RESP_BAD_REQUEST, "warning repeat rejects negative integer" },
        { DYNCFG_INTEGER_REPEAT, "warning", "\"-1\"", HTTP_RESP_BAD_REQUEST, "warning repeat rejects negative string" },
        { DYNCFG_INTEGER_REPEAT, "warning", "2147483648", HTTP_RESP_BAD_REQUEST, "warning repeat rejects integer above INT_MAX" },
        { DYNCFG_INTEGER_REPEAT, "warning", "\"2147483648\"", HTTP_RESP_BAD_REQUEST, "warning repeat rejects string above INT_MAX" },
        { DYNCFG_INTEGER_REPEAT, "critical", "-1", HTTP_RESP_BAD_REQUEST, "critical repeat rejects negative integer" },
        { DYNCFG_INTEGER_REPEAT, "critical", "\"-1\"", HTTP_RESP_BAD_REQUEST, "critical repeat rejects negative string" },
        { DYNCFG_INTEGER_REPEAT, "critical", "2147483648", HTTP_RESP_BAD_REQUEST, "critical repeat rejects integer above INT_MAX" },
        { DYNCFG_INTEGER_REPEAT, "critical", "\"2147483648\"", HTTP_RESP_BAD_REQUEST, "critical repeat rejects string above INT_MAX" },

        { DYNCFG_INTEGER_DB_LOOKUP, "after", "2147483648.0", HTTP_RESP_BAD_REQUEST, "after rejects double above INT_MAX" },
        { DYNCFG_INTEGER_DB_LOOKUP, "after", "-2147483649.0", HTTP_RESP_BAD_REQUEST, "after rejects double below INT_MIN" },
        { DYNCFG_INTEGER_DELAY, "up", "2147483648.0", HTTP_RESP_BAD_REQUEST, "delay rejects double above INT_MAX" },
        { DYNCFG_INTEGER_DELAY, "up", "-2147483649.0", HTTP_RESP_BAD_REQUEST, "delay rejects double below INT_MIN" },
        { DYNCFG_INTEGER_REPEAT, "warning", "-1.0", HTTP_RESP_BAD_REQUEST, "repeat rejects negative double" },
        { DYNCFG_INTEGER_REPEAT, "warning", "2147483648.0", HTTP_RESP_BAD_REQUEST, "repeat rejects double above INT_MAX" },
    };

    int failed = 0;
    for(size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        CLEAN_BUFFER *payload = buffer_create(0, NULL);
        CLEAN_BUFFER *result = buffer_create(0, NULL);

        buffer_strcat(payload, "{\"format_version\":1,\"rules\":[{\"enabled\":true,\"type\":\"instance\",\"config\":{");
        switch(tests[i].section) {
            case DYNCFG_INTEGER_DB_LOOKUP:
                if(strcmp(tests[i].member, "before") == 0)
                    buffer_sprintf(payload, "\"value\":{\"database_lookup\":{\"after\":1,\"before\":%s}}",
                                   tests[i].value);
                else
                    buffer_sprintf(payload, "\"value\":{\"database_lookup\":{\"%s\":%s}}",
                                   tests[i].member, tests[i].value);
                break;
            case DYNCFG_INTEGER_DELAY:
                if(strcmp(tests[i].member, "max") == 0)
                    buffer_sprintf(payload, "\"value\":{},\"action\":{\"delay\":{\"up\":1,\"max\":%s}}",
                                   tests[i].value);
                else
                    buffer_sprintf(payload, "\"value\":{},\"action\":{\"delay\":{\"%s\":%s}}",
                                   tests[i].member, tests[i].value);
                break;
            case DYNCFG_INTEGER_REPEAT:
                buffer_sprintf(payload, "\"value\":{},\"action\":{\"repeat\":{\"enabled\":true,\"%s\":%s}}",
                               tests[i].member, tests[i].value);
                break;
        }
        buffer_strcat(payload, ",\"match\":{\"on\":\"chart\"}}}]}" );

        int code = dyncfg_health_cb(NULL, "health:alert:prototype", DYNCFG_CMD_USERCONFIG, "unittest",
                                    payload, NULL, NULL, result, HTTP_ACCESS_NONE, NULL, NULL);
        bool has_field_error = tests[i].expected_code == HTTP_RESP_OK ||
                               (strstr(buffer_tostring(result), tests[i].member) &&
                                strstr(buffer_tostring(result), "range"));
        bool has_expected_text = !tests[i].expected_text ||
                                 strstr(buffer_tostring(result), tests[i].expected_text);
        bool lacks_unexpected_text = !tests[i].expected_absent_text ||
                                     !strstr(buffer_tostring(result), tests[i].expected_absent_text);

        if(code != tests[i].expected_code || !has_field_error || !has_expected_text || !lacks_unexpected_text) {
            fprintf(stderr, "FAILED [%s]: code=%d response='%s'\n",
                    tests[i].description, code, buffer_tostring(result));
            failed++;
        }
        else
            (*passed)++;
    }

    CLEAN_BUFFER *payload = buffer_create(0, NULL);
    CLEAN_BUFFER *result = buffer_create(0, NULL);
    buffer_strcat(payload,
                  "{\"format_version\":1,\"rules\":["
                  "{\"enabled\":true,\"type\":\"instance\",\"config\":{"
                  "\"value\":{},\"action\":{\"delay\":{\"up\":1}},\"match\":{\"on\":\"first\"}}},"
                  "{\"enabled\":true,\"type\":\"instance\",\"config\":{"
                  "\"value\":{},\"action\":{\"repeat\":{\"enabled\":true,\"warning\":-1}},"
                  "\"match\":{\"on\":\"second\"}}}]}" );

    int code = dyncfg_health_cb(NULL, "health:alert:prototype", DYNCFG_CMD_USERCONFIG, "unittest",
                                payload, NULL, NULL, result, HTTP_ACCESS_NONE, NULL, NULL);
    if(code != HTTP_RESP_BAD_REQUEST || !strstr(buffer_tostring(result), "warning") ||
       !strstr(buffer_tostring(result), "range")) {
        fprintf(stderr,
                "FAILED [invalid repeat in a later rule rejects the complete prototype]: code=%d response='%s'\n",
                code, buffer_tostring(result));
        failed++;
    }
    else
        (*passed)++;

    return failed;
}

static int test_dyncfg_delay_multiplier_boundaries(int *passed) {
    static const struct {
        const char *multiplier;
        int expected_code;
        const char *description;
    } tests[] = {
        { "0", HTTP_RESP_OK, "zero multiplier remains accepted" },
        { "-1", HTTP_RESP_OK, "negative multiplier remains accepted" },
        { "0.5", HTTP_RESP_OK, "fractional multiplier remains accepted" },
        { "9223372036854775807", HTTP_RESP_OK, "maximum int64 multiplier remains accepted" },
        { "-9223372036854775808", HTTP_RESP_OK, "minimum int64 multiplier remains accepted" },
        { "3.4028234663852886e38", HTTP_RESP_OK, "FLT_MAX multiplier remains accepted" },
        { "3.4028236e38", HTTP_RESP_BAD_REQUEST, "value above FLT_MAX is rejected before narrowing" },
        { "1e100", HTTP_RESP_BAD_REQUEST, "large finite double is rejected before narrowing" },
        { "\"nan\"", HTTP_RESP_BAD_REQUEST, "NaN string is rejected" },
        { "\"inf\"", HTTP_RESP_BAD_REQUEST, "infinity string is rejected" },
        { "null", HTTP_RESP_BAD_REQUEST, "null multiplier is rejected" },
    };

    int failed = 0;
    for(size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        CLEAN_BUFFER *payload = buffer_create(0, NULL);
        CLEAN_BUFFER *result = buffer_create(0, NULL);

        buffer_sprintf(payload,
                       "{\"format_version\":1,\"rules\":[{\"enabled\":true,\"type\":\"instance\","
                       "\"config\":{\"value\":{},\"action\":{\"delay\":{\"up\":1,\"down\":0,\"max\":10,"
                       "\"multiplier\":%s}},\"match\":{\"on\":\"chart\"}}}]}",
                       tests[i].multiplier);

        int code = dyncfg_health_cb(NULL, "health:alert:prototype", DYNCFG_CMD_USERCONFIG, "unittest",
                                    payload, NULL, NULL, result, HTTP_ACCESS_NONE, NULL, NULL);
        bool has_expected_error = code == HTTP_RESP_OK || strstr(buffer_tostring(result), "multiplier") != NULL;

        if(code != tests[i].expected_code || !has_expected_error) {
            fprintf(stderr, "FAILED [%s]: code=%d response='%s'\n",
                    tests[i].description, code, buffer_tostring(result));
            failed++;
        }
        else
            (*passed)++;
    }

    return failed;
}

static int run_delay_parse_case(
    const char *input, int initial_max,
    int expected_up, int expected_down, int expected_max, float expected_multiplier,
    const char *description) {
    char buffer[256];
    strncpyz(buffer, input, sizeof(buffer) - 1);

    int up = 123;
    int down = 456;
    int max = initial_max;
    float multiplier = 7.0f;

    int result = health_parse_delay(1, "unittest", buffer, &up, &down, &max, &multiplier);
    if(!result || up != expected_up || down != expected_down || max != expected_max ||
       multiplier != expected_multiplier) {
        fprintf(stderr,
                "FAILED [%s]: result=%d up=%d down=%d max=%d multiplier=%g\n",
                description, result, up, down, max, (double)multiplier);
        return 1;
    }

    return 0;
}

static int test_delay_parser_multiplier_boundaries(int *passed) {
    static const struct {
        const char *input;
        int initial_max;
        int expected_up;
        int expected_down;
        int expected_max;
        float expected_multiplier;
        const char *description;
    } tests[] = {
        { "up 10s", 0, 10, 0, 10, 1.0f, "omitted values keep documented defaults" },
        { "down 15m multiplier 1.5", 0, 0, 900, 1350, 1.5f, "ordinary fractional multiplier" },
        { "up 3s multiplier 0.5", 0, 3, 0, 1, 0.5f, "fractional product truncates toward zero" },
        { "up -3s multiplier 1.5", 0, -3, 0, 0, 1.5f, "negative duration preserves zero default max" },
        { "up 10s multiplier 2 max 7s", 0, 10, 0, 7, 2.0f, "explicit maximum remains authoritative" },
        { "up 10s multiplier 0", 0, 10, 0, 10, 1.0f, "zero multiplier uses text default" },
        { "up 10s multiplier -2", 0, 10, 0, 10, 1.0f, "negative multiplier uses text default" },
        { "up 10s multiplier nan", 0, 10, 0, 10, 1.0f, "NaN multiplier uses text default" },
        { "up 10s multiplier inf", 0, 10, 0, 10, 1.0f, "infinite multiplier uses text default" },
        { "max 7s", 0, 0, 0, 7, 1.0f, "explicit maximum with omitted delays" },
        { "up 2s", 5, 2, 0, 5, 1.0f, "prior larger maximum remains unchanged" },
        { "up 16777220s", 16777219, 16777220, 0, 16777219, 1.0f,
          "prior maximum preserves the existing float comparison" },
    };

    int failed = 0;
    for(size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        int rc = run_delay_parse_case(
            tests[i].input, tests[i].initial_max,
            tests[i].expected_up, tests[i].expected_down, tests[i].expected_max,
            tests[i].expected_multiplier, tests[i].description);
        failed += rc;
        if(!rc)
            (*passed)++;
    }

    char input[256];
    snprintfz(input, sizeof(input), "up %ds", INT_MAX);
    int rc = run_delay_parse_case(input, 0, INT_MAX, 0, INT_MAX, 1.0f,
                                  "INT_MAX duration has a representable default maximum");
    failed += rc;
    if(!rc)
        (*passed)++;

    snprintfz(input, sizeof(input), "up %ds", INT_MAX);
    rc = run_delay_parse_case(input, INT_MAX - 1, INT_MAX, 0, INT_MAX - 1, 1.0f,
                              "rounded INT_MAX product preserves the prior maximum comparison");
    failed += rc;
    if(!rc)
        (*passed)++;

    snprintfz(input, sizeof(input), "down %ds multiplier 2", INT_MIN);
    rc = run_delay_parse_case(input, 0, 0, INT_MIN, 0, 2.0f,
                              "INT_MIN negative product does not change the default maximum");
    failed += rc;
    if(!rc)
        (*passed)++;

    snprintfz(input, sizeof(input), "up 1s multiplier %a", (double)FLT_MAX);
    rc = run_delay_parse_case(input, 0, 1, 0, INT_MAX, FLT_MAX,
                              "largest finite multiplier bounds the omitted maximum");
    failed += rc;
    if(!rc)
        (*passed)++;

    return failed;
}

static int test_delay_multiplier_runtime_boundaries(int *passed) {
    static const struct {
        int delay;
        float multiplier;
        int maximum;
        int expected;
        const char *description;
    } tests[] = {
        { 10, 1.5f, 100, 15, "ordinary multiplication" },
        { 3, 1.5f, 100, 4, "positive fractional truncation" },
        { -3, 1.5f, 100, -4, "negative fractional truncation" },
        { 10, 2.0f, 15, 15, "configured maximum clamps product" },
        { 10, 2.0f, -5, -5, "negative configured maximum keeps existing clamp semantics" },
        { 10, 0.0f, 100, 0, "zero Dynamic Configuration multiplier" },
        { 10, -2.0f, 100, -20, "negative Dynamic Configuration multiplier" },
        { INT_MAX, 1.0f, INT_MAX, INT_MAX, "INT_MAX float rounding is bounded" },
        { INT_MAX, 0.5f, INT_MAX, 1073741824, "large representable product keeps float rounding" },
        { INT_MIN, 2.0f, INT_MAX, INT_MIN, "negative out-of-range product uses lower endpoint" },
        { INT_MAX, -1.0f, INT_MAX, INT_MIN, "negative boundary product remains representable" },
        { INT_MIN, -1.0f, 17, 17, "positive out-of-range product uses configured maximum" },
        { 1, FLT_MAX, 123, 123, "largest positive finite multiplier uses configured maximum" },
        { -1, FLT_MAX, INT_MAX, INT_MIN, "largest negative product uses lower endpoint" },
    };

    int failed = 0;
    for(size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        int actual = health_delay_apply_multiplier(tests[i].delay, tests[i].multiplier, tests[i].maximum);
        if(actual != tests[i].expected) {
            fprintf(stderr, "FAILED [%s]: expected=%d actual=%d\n",
                    tests[i].description, tests[i].expected, actual);
            failed++;
        }
        else
            (*passed)++;
    }

    return failed;
}

static int test_prototype_rejects_non_finite_delay_multiplier(int *passed) {
    static const float tests[] = { NAN, INFINITY, -INFINITY };
    int failed = 0;

    for(size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        RRD_ALERT_PROTOTYPE ap = { 0 };
        ap.match.on.chart = string_strdupz("chart");
        ap.config.name = string_strdupz("unittest");
        ap.config.source = string_strdupz("unittest");
        ap.config.update_every = 1;
        ap.config.delay_multiplier = tests[i];

        const char *failed_at = NULL;
        int error = 0;
        ap.config.calculation = expression_parse("1", &failed_at, &error);

        char *msg = NULL;
        if(!ap.config.calculation || health_prototype_add(&ap, &msg) || !msg ||
           strcmp(msg, "non-finite delay multiplier") != 0) {
            fprintf(stderr,
                    "FAILED [prototype rejects non-finite delay multiplier %g]: msg='%s'\n",
                    (double)tests[i], msg ? msg : "");
            failed++;
        }
        else
            (*passed)++;

        health_prototype_cleanup(&ap);
    }

    RRD_ALERT_PROTOTYPE ap = { 0 };
    ap.match.on.chart = string_strdupz("chart");
    ap.config.name = string_strdupz("unittest");
    ap.config.source = string_strdupz("unittest");
    ap.config.update_every = 1;
    ap.config.delay_multiplier = 1.0f;

    const char *failed_at = NULL;
    int error = 0;
    ap.config.calculation = expression_parse("1", &failed_at, &error);

    RRD_ALERT_PROTOTYPE *second = callocz(1, sizeof(*second));
    second->config.name = string_strdupz("unittest");
    second->config.source = string_strdupz("unittest");
    second->config.delay_multiplier = NAN;
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(ap._internal.next, second, _internal.prev, _internal.next);

    char *msg = NULL;
    if(!ap.config.calculation || health_prototype_add(&ap, &msg) || !msg ||
       strcmp(msg, "non-finite delay multiplier") != 0) {
        fprintf(stderr, "FAILED [prototype rejects non-finite multiplier in later rule]: msg='%s'\n",
                msg ? msg : "");
        failed++;
    }
    else
        (*passed)++;

    health_prototype_cleanup(&ap);

    return failed;
}

static int test_prototype_rejects_non_positive_update_every(int *passed) {
    static const int tests[] = { -1, 0 };
    int failed = 0;

    for(size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        RRD_ALERT_PROTOTYPE ap = { 0 };
        ap.match.on.chart = string_strdupz("chart");
        ap.config.name = string_strdupz("unittest");
        ap.config.source = string_strdupz("unittest");
        ap.config.update_every = tests[i];

        char *msg = NULL;
        if(health_prototype_add(&ap, &msg) || !msg || strcmp(msg, "missing update frequency") != 0) {
            fprintf(stderr,
                    "FAILED [prototype rejects update_every=%d]: msg='%s'\n",
                    tests[i], msg ? msg : "");
            failed++;
        }
        else
            (*passed)++;

        health_prototype_cleanup(&ap);
    }

    return failed;
}

// An alert reaches the agent as JSON (dyncfg) and leaves it as a `.conf`
// line, so the condition has to survive that round trip byte for byte -
// otherwise exporting and re-importing an alert silently rewrites it, and
// its configuration hash churns on every restart.
static int test_dyncfg_time_group_round_trip(int *passed) {
    static const struct {
        const char *time_group;
        const char *members;             // condition members of database_lookup
        const char *expected_lookup;     // the `lookup:` value the export must produce
        RRDR_TIME_GROUPING expected_group;
        const char *expected_options;    // NULL when the grouping keeps no expression
        ALERT_LOOKUP_TIME_GROUP_CONDITION expected_condition;
        NETDATA_DOUBLE expected_value;
        const char *description;
    } tests[] = {
        { "percentage-of-time", "\"time_group_condition\":\">=\",\"time_group_value\":1,\"time_group_options\":\">=1\"",
          "percentage-of-time(>=1) -1m", RRDR_GROUPING_PERCENTAGE_OF_TIME, ">=1",
          ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER_EQUAL, 1,
          "percentage-of-time keeps its threshold" },

        { "number-of-flaps", "\"time_group_condition\":\"==\",\"time_group_value\":0,\"time_group_options\":\"==gap\"",
          "number-of-flaps(==gap) -1m", RRDR_GROUPING_NUMBER_OF_FLAPS, "==gap",
          ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, NAN,
          "a gap token survives the round trip" },

        { "number-of-times", "\"time_group_condition\":\"<\",\"time_group_value\":0,\"time_group_options\":\"<previous\"",
          "number-of-times(<previous) -1m", RRDR_GROUPING_NUMBER_OF_TIMES, "<previous",
          ALERT_LOOKUP_TIME_GROUP_CONDITION_LESS, NAN,
          "the predecessor keyword survives the round trip" },

        // the legacy shape: alerts written before the expression existed
        // carry only the condition/value pair. The export normalises the
        // name to the canonical one and writes the pair out as the
        // equivalent expression; the alert MUST come back as the same
        // grouping running the same condition
        { "countif", "\"time_group_condition\":\"!=\",\"time_group_value\":0",
          "percentage-of-samples(!=0.00) -1m", RRDR_GROUPING_COUNTIF, "!=0.00",
          ALERT_LOOKUP_TIME_GROUP_CONDITION_NOT_EQUAL, 0,
          "a legacy countif alert round trips on its condition pair" },

        // untouched by the expression grammar - proves we did not widen it.
        // `percentile95` is the canonical echo of the percentile enum and
        // predates this work
        { "percentile", "\"time_group_value\":95",
          "percentile95(95.00) -1m", RRDR_GROUPING_PERCENTILE, NULL,
          DC_COND, 95,
          "percentile still round trips on its numeric argument" },
    };

    int failed = 0;
    for(size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        CLEAN_BUFFER *payload = buffer_create(0, NULL);
        CLEAN_BUFFER *result = buffer_create(0, NULL);

        buffer_sprintf(payload,
                       "{\"format_version\":1,\"rules\":[{\"enabled\":true,\"type\":\"instance\","
                       "\"config\":{\"value\":{\"update_every\":1,\"database_lookup\":{"
                       "\"after\":-60,\"time_group\":\"%s\",%s}},"
                       "\"match\":{\"on\":\"chart\"}}}]}",
                       tests[i].time_group, tests[i].members);

        int code = dyncfg_health_cb(NULL, "health:alert:prototype", DYNCFG_CMD_USERCONFIG, "unittest",
                                    payload, NULL, NULL, result, HTTP_ACCESS_NONE, NULL, NULL);
        if(code != HTTP_RESP_OK) {
            fprintf(stderr, "FAILED [%s]: export failed with code=%d response='%s'\n",
                    tests[i].description, code, buffer_tostring(result));
            failed++;
            continue;
        }

        // pull the `lookup:` line back out of the exported configuration
        const char *found = strstr(buffer_tostring(result), "lookup: ");
        if(!found) {
            fprintf(stderr, "FAILED [%s]: no lookup line in the exported config:\n%s\n",
                    tests[i].description, buffer_tostring(result));
            failed++;
            continue;
        }

        char lookup[512];
        found += strlen("lookup: ");
        const char *eol = strchr(found, '\n');
        size_t len = eol ? (size_t)(eol - found) : strlen(found);
        if(len >= sizeof(lookup)) len = sizeof(lookup) - 1;
        memcpy(lookup, found, len);
        lookup[len] = '\0';

        if(strcmp(lookup, tests[i].expected_lookup) != 0) {
            fprintf(stderr, "FAILED [%s]: exported 'lookup: %s', want 'lookup: %s'\n",
                    tests[i].description, lookup, tests[i].expected_lookup);
            failed++;
            continue;
        }

        // and feed it back through the config parser, the way a restart does
        struct rrd_alert_config ac = { 0 };
        ac.time_group_value = NAN;

        if(!health_parse_db_lookup(1, "unittest", lookup, &ac)) {
            fprintf(stderr, "FAILED [%s]: the exported lookup does not parse back: '%s'\n",
                    tests[i].description, tests[i].expected_lookup);
            failed++;
        }
        else {
            int errors = 0;

            if(ac.time_group != tests[i].expected_group) {
                fprintf(stderr, "FAILED [%s]: re-parsed group %u, want %u\n",
                        tests[i].description, (unsigned)ac.time_group, (unsigned)tests[i].expected_group);
                errors++;
            }

            const char *options = ac.time_group_options ? string2str(ac.time_group_options) : NULL;
            if((options == NULL) != (tests[i].expected_options == NULL) ||
               (options && strcmp(options, tests[i].expected_options) != 0)) {
                fprintf(stderr, "FAILED [%s]: re-parsed options '%s', want '%s'\n",
                        tests[i].description, options ? options : "(none)",
                        tests[i].expected_options ? tests[i].expected_options : "(none)");
                errors++;
            }

            if(tests[i].expected_condition != DC_COND && ac.time_group_condition != tests[i].expected_condition) {
                fprintf(stderr, "FAILED [%s]: re-parsed condition %d, want %d\n",
                        tests[i].description, ac.time_group_condition, tests[i].expected_condition);
                errors++;
            }

            if(!isnan(tests[i].expected_value) &&
               (isnan(ac.time_group_value) || fabsl(ac.time_group_value - tests[i].expected_value) > 0.0001)) {
                fprintf(stderr, "FAILED [%s]: re-parsed value %f, want %f\n",
                        tests[i].description, (double)ac.time_group_value, (double)tests[i].expected_value);
                errors++;
            }

            if(errors)
                failed += errors;
            else
                (*passed)++;
        }

        string_freez(ac.dimensions);
        string_freez(ac.time_group_options);
    }

    return failed;
}

// The written condition is trimmed before it is stored, and the trim must
// not put a ceiling on how long it may be: a condition padded past a fixed
// buffer used to come back empty, and the alert then ran the legacy
// condition/value pair instead of what its author wrote.
static int test_dyncfg_long_condition_is_not_truncated(int *passed) {
    CLEAN_BUFFER *payload = buffer_create(0, NULL);
    CLEAN_BUFFER *result = buffer_create(0, NULL);

    // padding wide enough that any fixed buffer would have to cut it
    char padding[512];
    memset(padding, ' ', sizeof(padding) - 1);
    padding[sizeof(padding) - 1] = '\0';

    buffer_sprintf(payload,
                   "{\"format_version\":1,\"rules\":[{\"enabled\":true,\"type\":\"instance\","
                   "\"config\":{\"value\":{\"update_every\":1,\"database_lookup\":{"
                   "\"after\":-60,\"time_group\":\"percentage-of-time\","
                   "\"time_group_condition\":\">=\",\"time_group_value\":1,"
                   "\"time_group_options\":\"%s>=1%s\"}},"
                   "\"match\":{\"on\":\"chart\"}}}]}",
                   padding, padding);

    int code = dyncfg_health_cb(NULL, "health:alert:prototype", DYNCFG_CMD_USERCONFIG, "unittest",
                                payload, NULL, NULL, result, HTTP_ACCESS_NONE, NULL, NULL);
    if(code != HTTP_RESP_OK) {
        fprintf(stderr, "FAILED [a long padded condition]: export failed with code=%d response='%s'\n",
                code, buffer_tostring(result));
        return 1;
    }

    if(!strstr(buffer_tostring(result), "lookup: percentage-of-time(>=1) -1m")) {
        fprintf(stderr, "FAILED [a long padded condition]: the exported config does not carry "
                        "'lookup: percentage-of-time(>=1) -1m':\n%s\n", buffer_tostring(result));
        return 1;
    }

    (*passed)++;
    return 0;
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

    failed += test_db_lookup_frequency_boundaries(&passed);
    failed += test_db_lookup_condition_is_trimmed(&passed);
    failed += test_dyncfg_update_every_boundaries(&passed);
    failed += test_dyncfg_integer_destination_boundaries(&passed);
    failed += test_dyncfg_delay_multiplier_boundaries(&passed);
    failed += test_delay_parser_multiplier_boundaries(&passed);
    failed += test_delay_multiplier_runtime_boundaries(&passed);
    failed += test_prototype_rejects_non_finite_delay_multiplier(&passed);
    failed += test_prototype_rejects_non_positive_update_every(&passed);
    failed += test_dyncfg_time_group_round_trip(&passed);
    failed += test_dyncfg_long_condition_is_not_truncated(&passed);
    failed += test_expression_rejects_leave_the_default(&passed);

    // the same alert configuration, taken through the metadata database
    // instead of the parser - counted BEFORE the summary, or a failure here
    // is printed above a line claiming nothing failed
    failed += sql_alert_config_unittest();

    fprintf(stderr, "\n===================================================\n");
    fprintf(stderr, "Health config parser tests: %d passed, %d failed\n\n", passed, failed);

    return failed;
}
