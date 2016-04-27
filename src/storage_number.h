/* vim: set ts=4 noet sw=4 : */
#ifndef NETDATA_STORAGE_NUMBER_H
#define NETDATA_STORAGE_NUMBER_H

#include <math.h>
#include "fpconv.h"

typedef double calculated_number;
#define CALCULATED_NUMBER_FORMAT "%0.7g"
//typedef long long calculated_number;
//#define CALCULATED_NUMBER_FORMAT "%lld"

typedef long long collected_number;
#define COLLECTED_NUMBER_FORMAT "%lld"

typedef struct {
	double value;
	char   flag_exists:1;
	char   flag_reset:1;
} __attribute__((packed)) storage_number;

typedef double ustorage_number;
#define STORAGE_NUMBER_FORMAT "%0.7g"

#define SN_NOT_EXISTS		0
#define SN_EXISTS			1
#define SN_EXISTS_RESET		2

// checks
#define does_storage_number_exist(v) ((v).flag_exists)
#define did_storage_number_reset(v) ((v).flag_reset)

storage_number pack_storage_number(calculated_number value, int flags);

int print_calculated_number(char *str, calculated_number value);

#define STORAGE_NUMBER_POSITIVE_MAX 167772150000000.0
#define STORAGE_NUMBER_POSITIVE_MIN 0.00001
#define STORAGE_NUMBER_NEGATIVE_MAX -0.00001
#define STORAGE_NUMBER_NEGATIVE_MIN -167772150000000.0

// accepted accuracy loss
#define ACCURACY_LOSS 0.0001
#define accuracy_loss(t1, t2) ((t1 == t2 || t1 == 0.0 || t2 == 0.0) ? 0.0 : (100.0 - ((t1 > t2) ? (t2 * 100.0 / t1 ) : (t1 * 100.0 / t2))))

#define unpack_storage_number(v) ((v).value)
#define print_calculated_number(str, value) fpconv_dtoa((value), (str))

#endif /* NETDATA_STORAGE_NUMBER_H */
