// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STORAGE_NUMBER_H
#define NETDATA_STORAGE_NUMBER_H 1

#include "../libnetdata.h"

#ifdef NETDATA_WITHOUT_LONG_DOUBLE

#define powl pow
#define modfl modf
#define llrintl llrint
#define roundl round
#define sqrtl sqrt
#define copysignl copysign
#define strtold strtod

typedef double calculated_number;
#define CALCULATED_NUMBER_FORMAT "%0.7f"
#define CALCULATED_NUMBER_FORMAT_ZERO "%0.0f"
#define CALCULATED_NUMBER_FORMAT_AUTO "%f"

#define LONG_DOUBLE_MODIFIER "f"
typedef double LONG_DOUBLE;

#define CALCULATED_NUMBER_MAX DBL_MAX

#else // NETDATA_WITHOUT_LONG_DOUBLE

typedef long double calculated_number;
#define CALCULATED_NUMBER_FORMAT "%0.7Lf"
#define CALCULATED_NUMBER_FORMAT_ZERO "%0.0Lf"
#define CALCULATED_NUMBER_FORMAT_AUTO "%Lf"

#define LONG_DOUBLE_MODIFIER "Lf"
typedef long double LONG_DOUBLE;

#define CALCULATED_NUMBER_MAX LDBL_MAX

#endif // NETDATA_WITHOUT_LONG_DOUBLE

//typedef long long calculated_number;
//#define CALCULATED_NUMBER_FORMAT "%lld"

typedef long long collected_number;
#define COLLECTED_NUMBER_FORMAT "%lld"

/*
typedef long double collected_number;
#define COLLECTED_NUMBER_FORMAT "%0.7Lf"
*/

#define calculated_number_modf(x, y) modfl(x, y)
#define calculated_number_llrint(x) llrintl(x)
#define calculated_number_round(x) roundl(x)
#define calculated_number_fabs(x) fabsl(x)
#define calculated_number_pow(x, y) powl(x, y)
#define calculated_number_epsilon (calculated_number)0.0000001

#define calculated_number_equal(a, b) (calculated_number_fabs((a) - (b)) < calculated_number_epsilon)

#define calculated_number_isnumber(a) (!(fpclassify(a) & (FP_NAN|FP_INFINITE)))

typedef uint32_t storage_number;
#define STORAGE_NUMBER_FORMAT "%u"

#define SN_ANOMALY_BIT      (1 << 24) // the anomaly bit of the value
#define SN_EXISTS_RESET     (1 << 25) // the value has been overflown
#define SN_EXISTS_100       (1 << 26) // very large value (multiplier is 100 instead of 10)

#define SN_DEFAULT_FLAGS    SN_ANOMALY_BIT

#define SN_EMPTY_SLOT 0x00000000

// When the calculated number is zero and the value is anomalous (ie. it's bit
// is zero) we want to return a storage_number representation that is
// different from the empty slot. We achieve this by mapping zero to
// SN_EXISTS_100. Unpacking the SN_EXISTS_100 value will return zero because
// its fraction field (as well as its exponent factor field) will be zero.
#define SN_ANOMALOUS_ZERO SN_EXISTS_100

// checks
#define does_storage_number_exist(value) (((storage_number) (value)) != SN_EMPTY_SLOT)
#define did_storage_number_reset(value)  ((((storage_number) (value)) & SN_EXISTS_RESET) != 0)

storage_number pack_storage_number(calculated_number value, uint32_t flags);
static inline calculated_number unpack_storage_number(storage_number value) __attribute__((const));

int print_calculated_number(char *str, calculated_number value);

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


static inline calculated_number unpack_storage_number(storage_number value) {
    extern calculated_number unpack_storage_number_lut10x[4 * 8];

    if(!value) return 0;

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
    int mul = (value & ((1<<29)|(1<<28)|(1<<27))) >> 27;

    // bit 24 to bit 1 = the value, so remove all other bits
    value ^= value & ((1<<31)|(1<<30)|(1<<29)|(1<<28)|(1<<27)|(1<<26)|(1<<25)|(1<<24));

    calculated_number n = value;

    // fprintf(stderr, "UNPACK: %08X, sign = %d, exp = %d, mul = %d, factor = %d, n = " CALCULATED_NUMBER_FORMAT "\n", value, sign, exp, mul, factor, n);

    return sign * unpack_storage_number_lut10x[(factor * 16) + (exp * 8) + mul] * n;
}

#endif /* NETDATA_STORAGE_NUMBER_H */
