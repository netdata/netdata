#ifndef NETDATA_STORAGE_NUMBER_H
#define NETDATA_STORAGE_NUMBER_H

typedef long double calculated_number;
#define CALCULATED_NUMBER_FORMAT "%0.7Lf"
//typedef long long calculated_number;
//#define CALCULATED_NUMBER_FORMAT "%lld"

typedef long long collected_number;
#define COLLECTED_NUMBER_FORMAT "%lld"

/*
typedef long double collected_number;
#define COLLECTED_NUMBER_FORMAT "%0.7Lf"
*/

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
#define get_storage_number_flags(value) ((((storage_number)value) & (1 << 24)) | (((storage_number)value) & (2 << 24)) | (((storage_number)value) & (4 << 24)))
#define SN_EMPTY_SLOT 0x00000000

// checks
#define does_storage_number_exist(value) ((get_storage_number_flags(value) != 0)?1:0)
#define did_storage_number_reset(value)  ((get_storage_number_flags(value) == SN_EXISTS_RESET)?1:0)

storage_number pack_storage_number(calculated_number value, uint32_t flags);
calculated_number unpack_storage_number(storage_number value);

int print_calculated_number(char *str, calculated_number value);

#define STORAGE_NUMBER_POSITIVE_MAX 167772150000000.0
#define STORAGE_NUMBER_POSITIVE_MIN 0.00001
#define STORAGE_NUMBER_NEGATIVE_MAX -0.00001
#define STORAGE_NUMBER_NEGATIVE_MIN -167772150000000.0

// accepted accuracy loss
#define ACCURACY_LOSS 0.0001
#define accuracy_loss(t1, t2) ((t1 == t2 || t1 == 0.0 || t2 == 0.0) ? 0.0 : (100.0 - ((t1 > t2) ? (t2 * 100.0 / t1 ) : (t1 * 100.0 / t2))))

#endif /* NETDATA_STORAGE_NUMBER_H */
