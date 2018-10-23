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

#else

typedef long double calculated_number;
#define CALCULATED_NUMBER_FORMAT "%0.7Lf"
#define CALCULATED_NUMBER_FORMAT_ZERO "%0.0Lf"
#define CALCULATED_NUMBER_FORMAT_AUTO "%Lf"

#define LONG_DOUBLE_MODIFIER "Lf"
typedef long double LONG_DOUBLE;

#endif

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
#define calculated_number_epsilon (calculated_number)0.0000001

#define calculated_number_equal(a, b) (calculated_number_fabs((a) - (b)) < calculated_number_epsilon)

typedef uint32_t storage_number;
#define STORAGE_NUMBER_FORMAT "%u"

#define SN_NOT_EXISTS       (0x0 << 24)
#define SN_EXISTS           (0x1 << 24)
#define SN_EXISTS_RESET     (0x2 << 24)
#define SN_EXISTS_UNDEF1    (0x3 << 24)
#define SN_EXISTS_UNDEF2    (0x4 << 24)
#define SN_EXISTS_UNDEF3    (0x5 << 24)
#define SN_EXISTS_UNDEF4    (0x6 << 24)

#define SN_FLAGS_MASK       (~(0x6 << 24))

// extract the flags
#define get_storage_number_flags(value) ((((storage_number)(value)) & (1 << 24)) | (((storage_number)(value)) & (2 << 24)) | (((storage_number)(value)) & (4 << 24)))
#define SN_EMPTY_SLOT 0x00000000

// checks
#define does_storage_number_exist(value) ((get_storage_number_flags(value) != 0)?1:0)
#define did_storage_number_reset(value)  ((get_storage_number_flags(value) == SN_EXISTS_RESET)?1:0)

storage_number pack_storage_number(calculated_number value, uint32_t flags);
calculated_number unpack_storage_number(storage_number value);

int print_calculated_number(char *str, calculated_number value);

#define STORAGE_NUMBER_POSITIVE_MAX (167772150000000.0)
#define STORAGE_NUMBER_POSITIVE_MIN (0.0000001)
#define STORAGE_NUMBER_NEGATIVE_MAX (-0.0000001)
#define STORAGE_NUMBER_NEGATIVE_MIN (-167772150000000.0)

// accepted accuracy loss
#define ACCURACY_LOSS 0.0001
#define accuracy_loss(t1, t2) (((t1) == (t2) || (t1) == 0.0 || (t2) == 0.0) ? 0.0 : (100.0 - (((t1) > (t2)) ? ((t2) * 100.0 / (t1) ) : ((t1) * 100.0 / (t2)))))

#endif /* NETDATA_STORAGE_NUMBER_H */
