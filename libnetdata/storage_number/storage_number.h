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

#define NETDATA_DOUBLE_MAX LDBL_MAX

#define strtondd(s, endptr) strtold(s, endptr)
#define powndd(x, y) powl(x, y)
#define llrintndd(x) llrintl(x)
#define roundndd(x) roundl(x)
#define sqrtndd(x) sqrtl(x)
#define copysignndd(x, y) copysignl(x, y)
#define modfndd(x, y) modfl(x, y)
#define fabsndd(x) fabsl(x)

#else // NETDATA_WITH_LONG_DOUBLE

typedef double NETDATA_DOUBLE;
#define NETDATA_DOUBLE_FORMAT "%0.7f"
#define NETDATA_DOUBLE_FORMAT_ZERO "%0.0f"
#define NETDATA_DOUBLE_FORMAT_AUTO "%f"
#define NETDATA_DOUBLE_MODIFIER "f"

#define NETDATA_DOUBLE_MAX DBL_MAX

#define strtondd(s, endptr) strtod(s, endptr)
#define powndd(x, y) pow(x, y)
#define llrintndd(x) llrint(x)
#define roundndd(x) round(x)
#define sqrtndd(x) sqrt(x)
#define copysignndd(x, y) copysign(x, y)
#define modfndd(x, y) modf(x, y)
#define fabsndd(x) fabs(x)

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

typedef uint32_t storage_number;

typedef struct storage_number_tier1 {
    storage_number  value;
    storage_number  min_value;
    storage_number  max_value;
    storage_number  sum_value;
    uint16_t        count;
} storage_number_tier1_t;

#define STORAGE_NUMBER_FORMAT "%u"

typedef enum {
    SN_ANOMALY_BIT   = (1 << 24), // the anomaly bit of the value
    SN_EXISTS_RESET  = (1 << 25), // the value has been overflown
    SN_EXISTS_100    = (1 << 26)  // very large value (multiplier is 100 instead of 10)
} SN_FLAGS;

#define SN_ALL_FLAGS (SN_ANOMALY_BIT|SN_EXISTS_RESET|SN_EXISTS_100)

#define SN_EMPTY_SLOT 0x00000000
#define SN_DEFAULT_FLAGS SN_ANOMALY_BIT

// When the calculated number is zero and the value is anomalous (ie. it's bit
// is zero) we want to return a storage_number representation that is
// different from the empty slot. We achieve this by mapping zero to
// SN_EXISTS_100. Unpacking the SN_EXISTS_100 value will return zero because
// its fraction field (as well as its exponent factor field) will be zero.
#define SN_ANOMALOUS_ZERO SN_EXISTS_100

// checks
#define does_storage_number_exist(value) (((storage_number) (value)) != SN_EMPTY_SLOT)
#define did_storage_number_reset(value)  ((((storage_number) (value)) & SN_EXISTS_RESET) != 0)

storage_number pack_storage_number(NETDATA_DOUBLE value, SN_FLAGS flags);
static inline NETDATA_DOUBLE unpack_storage_number(storage_number value) __attribute__((const));

int print_netdata_double(char *str, NETDATA_DOUBLE value);

//                                          sign       div/mul    <--- multiplier / divider --->     10/100       RESET      EXISTS     VALUE
#define STORAGE_NUMBER_POSITIVE_MAX_RAW (storage_number)( (0 << 31) | (1 << 30) | (1 << 29) | (1 << 28) | (1<<27) | (1 << 26) | (0 << 25) | (1 << 24) | 0x00ffffff )
#define STORAGE_NUMBER_POSITIVE_MIN_RAW (storage_number)( (0 << 31) | (0 << 30) | (1 << 29) | (1 << 28) | (1<<27) | (0 << 26) | (0 << 25) | (1 << 24) | 0x00000001 )
#define STORAGE_NUMBER_NEGATIVE_MAX_RAW (storage_number)( (1 << 31) | (0 << 30) | (1 << 29) | (1 << 28) | (1<<27) | (0 << 26) | (0 << 25) | (1 << 24) | 0x00000001 )
#define STORAGE_NUMBER_NEGATIVE_MIN_RAW (storage_number)( (1 << 31) | (1 << 30) | (1 << 29) | (1 << 28) | (1<<27) | (1 << 26) | (0 << 25) | (1 << 24) | 0x00ffffff )

// accepted accuracy loss
#define ACCURACY_LOSS_ACCEPTED_PERCENT 0.0001
#define accuracy_loss(t1, t2) (((t1) == (t2) || (t1) == 0.0 || (t2) == 0.0) ? 0.0 : (100.0 - (((t1) > (t2)) ? ((t2) * 100.0 / (t1) ) : ((t1) * 100.0 / (t2)))))

// Maximum acceptable rate of increase for counters. With a rate of 10% netdata can safely detect overflows with a
// period of at least every other 10 samples.
#define MAX_INCREMENTAL_PERCENT_RATE 10


static inline NETDATA_DOUBLE unpack_storage_number(storage_number value) {
    extern NETDATA_DOUBLE unpack_storage_number_lut10x[4 * 8];

    if(unlikely(value == SN_EMPTY_SLOT))
        return NAN;

    int sign = 1, exp = 0;
    int factor = 0;

    // bit 32 = 0:positive, 1:negative
    if(unlikely(value & (1 << 31)))
        sign = -1;

    // bit 31 = 0:divide, 1:multiply
    if(unlikely(value & (1 << 30)))
        exp = 1;

    // bit 27 SN_EXISTS_100
    if(unlikely(value & (1 << 26)))
        factor = 1;

    // bit 26 SN_EXISTS_RESET
    // bit 25 SN_ANOMALY_BIT

    // bit 30, 29, 28 = (multiplier or divider) 0-7 (8 total)
    int mul = (int)((value & ((1<<29)|(1<<28)|(1<<27))) >> 27);

    // bit 24 to bit 1 = the value, so remove all other bits
    value ^= value & ((1<<31)|(1<<30)|(1<<29)|(1<<28)|(1<<27)|(1<<26)|(1<<25)|(1<<24));

    NETDATA_DOUBLE n = value;

    // fprintf(stderr, "UNPACK: %08X, sign = %d, exp = %d, mul = %d, factor = %d, n = " CALCULATED_NUMBER_FORMAT "\n", value, sign, exp, mul, factor, n);

    return sign * unpack_storage_number_lut10x[(factor * 16) + (exp * 8) + mul] * n;
}

static inline NETDATA_DOUBLE str2ndd(const char *s, char **endptr) {
    int negative = 0;
    const char *start = s;
    unsigned long long integer_part = 0;
    unsigned long decimal_part = 0;
    size_t decimal_digits = 0;

    switch(*s) {
        case '-':
            s++;
            negative = 1;
            break;

        case '+':
            s++;
            break;

        case 'n':
            if(s[1] == 'a' && s[2] == 'n') {
                if(endptr) *endptr = (char *)&s[3];
                return NAN;
            }
            break;

        case 'i':
            if(s[1] == 'n' && s[2] == 'f') {
                if(endptr) *endptr = (char *)&s[3];
                return INFINITY;
            }
            break;

        default:
            break;
    }

    while (*s >= '0' && *s <= '9') {
        integer_part = (integer_part * 10) + (*s - '0');
        s++;
    }

    if(unlikely(*s == '.')) {
        decimal_part = 0;
        s++;

        while (*s >= '0' && *s <= '9') {
            decimal_part = (decimal_part * 10) + (*s - '0');
            s++;
            decimal_digits++;
        }
    }

    if(unlikely(*s == 'e' || *s == 'E'))
        return strtondd(start, endptr);

    if(unlikely(endptr))
        *endptr = (char *)s;

    if(unlikely(negative)) {
        if(unlikely(decimal_digits))
            return -((NETDATA_DOUBLE)integer_part + (NETDATA_DOUBLE)decimal_part / powndd(10.0, decimal_digits));
        else
            return -((NETDATA_DOUBLE)integer_part);
    }
    else {
        if(unlikely(decimal_digits))
            return (NETDATA_DOUBLE)integer_part + (NETDATA_DOUBLE)decimal_part / powndd(10.0, decimal_digits);
        else
            return (NETDATA_DOUBLE)integer_part;
    }
}

#endif /* NETDATA_STORAGE_NUMBER_H */
