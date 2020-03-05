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

#else // NETDATA_WITHOUT_LONG_DOUBLE

typedef long double calculated_number;
#define CALCULATED_NUMBER_FORMAT "%0.7Lf"
#define CALCULATED_NUMBER_FORMAT_ZERO "%0.0Lf"
#define CALCULATED_NUMBER_FORMAT_AUTO "%Lf"

#define LONG_DOUBLE_MODIFIER "Lf"
typedef long double LONG_DOUBLE;

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

#define SN_EXISTS           (1 << 24) // the value exists
#define SN_EXISTS_RESET     (1 << 25) // the value has been overflown
#define SN_EXISTS_100       (1 << 26) // very large value (multipler is 100 instead of 10)

// extract the flags
#define get_storage_number_flags(value) ((((storage_number)(value)) & (1 << 24)) | (((storage_number)(value)) & (1 << 25)) | (((storage_number)(value)) & (1 << 26)))
#define SN_EMPTY_SLOT 0x00000000

// checks
#define does_storage_number_exist(value) ((get_storage_number_flags(value) != 0)?1:0)
#define did_storage_number_reset(value)  ((get_storage_number_flags(value) == SN_EXISTS_RESET)?1:0)

storage_number pack_storage_number(calculated_number value, uint32_t flags);
calculated_number unpack_storage_number(storage_number value);

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

#endif /* NETDATA_STORAGE_NUMBER_H */
