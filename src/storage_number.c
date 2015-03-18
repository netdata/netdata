#include <math.h>

#include "log.h"
#include "storage_number.h"

storage_number pack_storage_number(calculated_number value)
{
	storage_number r = 0;
	if(!value) return r;

	// bit 32 = sign
	// bit 31 = 0:divide, 1:multiply
	// bit 30, 29, 28 = (multiplier or divider) 0-7
	// bit 27 to 25 = reserved for flags
	// bit 24 to bit 1 = the value

	storage_number sign = 0, exp = 0, mul;
	int m = 0;
	calculated_number n = value;

	if(n < 0) {
		sign = 1;
		n = -n;
	}

	while(m < 7 && n > (calculated_number)0x00ffffff) {
		n /= 10;
		m++;
	}
	while(m > -7 && n < (calculated_number)0x00199999) {
		n *= 10;
		m--;
	}

	if(m <= 0) {
		exp = 0;
		m = -m;
	}
	else exp = 1;

	if(n > (calculated_number)0x00ffffff) {
		error("Number " CALCULATED_NUMBER_FORMAT " is too big.", value);
		n = (calculated_number)0x00ffffff;
	}

	mul = m;

	// without this there are rounding problems
	// example: 0.9 becomes 0.89
	n = lrint(n);

	r = (sign << 31) + (exp << 30) + (mul << 27) + n;
	// fprintf(stderr, "PACK: %08X, sign = %d, exp = %d, mul = %d, n = " CALCULATED_NUMBER_FORMAT "\n", r, sign, exp, mul, n);

	return r;
}

calculated_number unpack_storage_number(storage_number value)
{
	if(!value) return 0;

	int sign = 0;
	int exp = 0;

	if(value & (1 << 31)) {
		sign = 1;
		value ^= 1 << 31;
	}

	if(value & (1 << 30)) {
		exp = 1;
		value ^= 1 << 30;
	}

	int mul = value >> 27;
	value ^= mul << 27;

	calculated_number n = value;

	// fprintf(stderr, "UNPACK: %08X, sign = %d, exp = %d, mul = %d, n = " CALCULATED_NUMBER_FORMAT "\n", value, sign, exp, mul, n);

	while(mul > 0) {
		if(exp) n *= 10;
		else n /= 10;
		mul--;
	}

	if(sign) n = -n;
	return n;
}
