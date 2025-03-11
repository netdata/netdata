// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STORAGE_NUMBER_H
#define NETDATA_STORAGE_NUMBER_H 1

#include <math.h>
#include "../libnetdata.h"

#ifdef NETDATA_WITH_LONG_DOUBLE

typedef long double NETDATA_DOUBLE;
#define NETDATA_DOUBLE_FORMAT "%0.7Lf"
#define NETDATA_DOUBLE_FORMAT_ZERO "%0.0Lf"
#define NETDATA_DOUBLE_FORMAT_AUTO "%Lf"
#define NETDATA_DOUBLE_MODIFIER "Lf"
#define NETDATA_DOUBLE_FORMAT_G "%0.19Le"

#define NETDATA_DOUBLE_MAX LDBL_MAX

#define strtondd(s, endptr) strtold(s, endptr)
#define powndd(x, y) powl(x, y)
#define llrintndd(x) llrintl(x)
#define roundndd(x) roundl(x)
#define sqrtndd(x) sqrtl(x)
#define copysignndd(x, y) copysignl(x, y)
#define modfndd(x, y) modfl(x, y)
#define fabsndd(x) fabsl(x)
#define floorndd(x) floorl(x)
#define ceilndd(x) ceill(x)
#define log10ndd(x) log10l(x)

#else // NETDATA_WITH_LONG_DOUBLE

typedef double NETDATA_DOUBLE;
#define NETDATA_DOUBLE_FORMAT "%0.7f"
#define NETDATA_DOUBLE_FORMAT_ZERO "%0.0f"
#define NETDATA_DOUBLE_FORMAT_AUTO "%f"
#define NETDATA_DOUBLE_MODIFIER "f"
#define NETDATA_DOUBLE_FORMAT_G "%0.19e"

#define NETDATA_DOUBLE_MAX DBL_MAX

#define strtondd(s, endptr) strtod(s, endptr)
#define powndd(x, y) pow(x, y)
#define llrintndd(x) llrint(x)
#define roundndd(x) round(x)
#define sqrtndd(x) sqrt(x)
#define copysignndd(x, y) copysign(x, y)
#define modfndd(x, y) modf(x, y)
#define fabsndd(x) fabs(x)
#define floorndd(x) floor(x)
#define ceilndd(x) ceil(x)
#define log10ndd(x) log10(x)

#endif // NETDATA_WITH_LONG_DOUBLE

typedef long long collected_number;
#define COLLECTED_NUMBER_FORMAT "%lld"

#define epsilonndd (NETDATA_DOUBLE)0.0000001
#define considered_equal_ndd(a, b) (fabsndd((a) - (b)) < epsilonndd)

#if defined(HAVE_ISFINITE) || defined(isfinite)
// The isfinite() macro shall determine whether its argument has a
// finite value (zero, subnormal, or normal, and not infinite or NaN).
#define netdata_double_isnumber(a) (isfinite(a))
#elif defined(HAVE_FINITE) || defined(finite)
#define netdata_double_isnumber(a) (finite(a))
#else
#define netdata_double_isnumber(a) (fpclassify(a) != FP_NAN && fpclassify(a) != FP_INFINITE)
#endif

#define netdata_double_is_zero(a) (!netdata_double_isnumber(a) || considered_equal_ndd(a, 0.0))
#define netdata_double_is_nonzero(a) (!netdata_double_is_zero(a))

typedef uint32_t storage_number;

typedef struct storage_number_tier1 {
    float sum_value;
    float min_value;
    float max_value;
    uint16_t count;
    uint16_t anomaly_count;
} storage_number_tier1_t;

#define STORAGE_NUMBER_FORMAT "%u"

typedef enum {
    SN_FLAG_NONE              = 0,
    SN_FLAG_NOT_ANOMALOUS     = (1 << 24), // the anomaly bit of the value (0:anomalous, 1:not anomalous)
    SN_FLAG_RESET             = (1 << 25), // the value has been overflown
    SN_FLAG_NOT_EXISTS_MUL100 = (1 << 26), // very large value (multiplier is 100 instead of 10)
    SN_FLAG_MULTIPLY          = (1 << 30), // multiply, else divide
    SN_FLAG_NEGATIVE          = (1 << 31), // negative, else positive
} SN_FLAGS;

#define SN_USER_FLAGS (SN_FLAG_NOT_ANOMALOUS | SN_FLAG_RESET)

// default flags for all storage numbers
// anomaly bit is reversed, so we set it by default
#define SN_DEFAULT_FLAGS SN_FLAG_NOT_ANOMALOUS

// When the calculated number is zero and the value is anomalous (ie. it's bit
// is zero) we want to return a storage_number representation that is
// different from the empty slot. We achieve this by mapping zero to
// SN_EXISTS_100. Unpacking the SN_EXISTS_100 value will return zero because
// its fraction field (as well as its exponent factor field) will be zero.
#define SN_EMPTY_SLOT SN_FLAG_NOT_EXISTS_MUL100

// checks
#define does_storage_number_exist(value) (((storage_number)(value)) != SN_EMPTY_SLOT)
#define did_storage_number_reset(value)  ((((storage_number)(value)) & SN_FLAG_RESET))
#define is_storage_number_anomalous(value)  (does_storage_number_exist(value) && !(((storage_number)(value)) & SN_FLAG_NOT_ANOMALOUS))

storage_number pack_storage_number(NETDATA_DOUBLE value, SN_FLAGS flags) __attribute__((const));
static inline NETDATA_DOUBLE unpack_storage_number(storage_number value) __attribute__((const));

//                                                          sign       div/mul      <--- multiplier / divider --->     10/100       RESET      EXISTS     VALUE
#define STORAGE_NUMBER_POSITIVE_MAX_RAW (storage_number)( (0U << 31) | (1U << 30) | (1U << 29) | (1U << 28) | (1U << 27) | (1U << 26) | (0U << 25) | (1U << 24) | 0x00ffffff )
#define STORAGE_NUMBER_POSITIVE_MIN_RAW (storage_number)( (0U << 31) | (0U << 30) | (1U << 29) | (1U << 28) | (1U << 27) | (0U << 26) | (0U << 25) | (1U << 24) | 0x00000001 )
#define STORAGE_NUMBER_NEGATIVE_MAX_RAW (storage_number)( (1U << 31) | (0U << 30) | (1U << 29) | (1U << 28) | (1U << 27) | (0U << 26) | (0U << 25) | (1U << 24) | 0x00000001 )
#define STORAGE_NUMBER_NEGATIVE_MIN_RAW (storage_number)( (1U << 31) | (1U << 30) | (1U << 29) | (1U << 28) | (1U << 27) | (1U << 26) | (0U << 25) | (1U << 24) | 0x00ffffff )

// accepted accuracy loss
#define ACCURACY_LOSS_ACCEPTED_PERCENT 0.0001
#define accuracy_loss(t1, t2) (((t1) == (t2) || (t1) == 0.0 || (t2) == 0.0) ? 0.0 : (100.0 - (((t1) > (t2)) ? ((t2) * 100.0 / (t1) ) : ((t1) * 100.0 / (t2)))))

// Maximum acceptable rate of increase for counters. With a rate of 10% netdata can safely detect overflows with a
// period of at least every other 10 samples.
#define MAX_INCREMENTAL_PERCENT_RATE 10


ALWAYS_INLINE_HOT_FLATTEN
static NETDATA_DOUBLE unpack_storage_number(storage_number value) {
    extern NETDATA_DOUBLE unpack_storage_number_lut10x[4 * 8];

    if(unlikely(value == SN_EMPTY_SLOT))
        return NAN;

    int sign = 1, exp = 0;
    int factor = 0;

    // bit 32 = 0:positive, 1:negative
    if(unlikely(value & SN_FLAG_NEGATIVE))
        sign = -1;

    // bit 31 = 0:divide, 1:multiply
    if(unlikely(value & SN_FLAG_MULTIPLY))
        exp = 1;

    // bit 27 SN_FLAG_NOT_EXISTS_MUL100
    if(unlikely(value & SN_FLAG_NOT_EXISTS_MUL100))
        factor = 1;

    // bit 26 SN_FLAG_RESET
    // bit 25 SN_FLAG_NOT_ANOMALOUS

    // bit 30, 29, 28 = (multiplier or divider) 0-7 (8 total)
    int mul = (int)((value & ((1U<<29)|(1U<<28)|(1U<<27))) >> 27);

    // bit 24 to bit 1 = the value, so remove all other bits
    value ^= value & ((1U <<31)|(1U <<30)|(1U <<29)|(1U <<28)|(1U <<27)|(1U <<26)|(1U <<25)|(1U<<24));

    NETDATA_DOUBLE n = value;

    // fprintf(stderr, "UNPACK: %08X, sign = %d, exp = %d, mul = %d, factor = %d, n = " CALCULATED_NUMBER_FORMAT "\n", value, sign, exp, mul, factor, n);

    return sign * unpack_storage_number_lut10x[(factor * 16) + (exp * 8) + mul] * n;
}

// all these prefixes should use characters that are not allowed in the numbers they represent
#define HEX_PREFIX "0x"               // we check 2 characters when parsing
#define IEEE754_UINT64_B64_PREFIX "#" // we check the 1st character during parsing
#define IEEE754_DOUBLE_B64_PREFIX "@" // we check the 1st character during parsing
#define IEEE754_DOUBLE_HEX_PREFIX "%" // we check the 1st character during parsing

bool is_system_ieee754_double(void);

#endif /* NETDATA_STORAGE_NUMBER_H */
