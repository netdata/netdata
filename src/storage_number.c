#ifdef STORAGE_WITH_MATH
#include <math.h>
#endif

#include "log.h"
#include "storage_number.h"

#if __GNUC__
#if __x86_64__ || __ppc64__
#define ENVIRONMENT64
#else
#define ENVIRONMENT32
#endif
#endif

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

#ifdef STORAGE_WITH_MATH
	// without this there are rounding problems
	// example: 0.9 becomes 0.89
	n = lrint(n);
#endif

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

#ifdef ENVIRONMENT32
// This trick seems to give an 80% speed increase in 32bit systems
// print_calculated_number_llu_r() will just print the digits up to the
// point the remaining value fits in 32 bits, and then calls
// print_calculated_number_lu_r() to print the rest with 32 bit arithmetic.

static char *print_calculated_number_lu_r(char *str, unsigned long uvalue) {
	char *wstr = str;
	
	// print each digit
	do *wstr++ = (char)(48 + (uvalue % 10)); while(uvalue /= 10);
	return wstr;
}

static char *print_calculated_number_llu_r(char *str, unsigned long long uvalue) {
	char *wstr = str;

	// print each digit
	do *wstr++ = (char)(48 + (uvalue % 10)); while((uvalue /= 10) && uvalue > (unsigned long long)0xffffffff);
	if(uvalue) return print_calculated_number_lu_r(wstr, uvalue);
	return wstr;
}
#endif

int print_calculated_number(char *str, calculated_number value)
{
	char *wstr = str;

	int sign = (value < 0) ? 1 : 0;
	if(sign) value = -value;

#ifdef STORAGE_WITH_MATH
	// without llrint() there are rounding problems
	// for example 0.9 becomes 0.89
	unsigned long long uvalue = llrint(value * (calculated_number)100000);
#else
	unsigned long long uvalue = value * (calculated_number)100000;
#endif

#ifdef ENVIRONMENT32
	if(uvalue > (unsigned long long)0xffffffff)
		wstr = print_calculated_number_llu_r(str, uvalue);
	else
		wstr = print_calculated_number_lu_r(str, uvalue);
#else
	do *wstr++ = (char)(48 + (uvalue % 10)); while(uvalue /= 10);
#endif

	// make sure we have 6 bytes at least
	while((wstr - str) < 6) *wstr++ = '0';

	// put the sign back
	if(sign) *wstr++ = '-';

	// reverse it
    char *begin = str, *end = --wstr, aux;
    while (end > begin) aux = *end, *end-- = *begin, *begin++ = aux;
	// wstr--;
	// strreverse(str, wstr);

	// remove trailing zeros
	int decimal = 5;
	while(decimal > 0 && *wstr == '0') {
		*wstr-- = '\0';
		decimal--;
	}

	// terminate it, one position to the right
	// to let space for a dot
	wstr[2] = '\0';

	// make space for the dot
	int i;
	for(i = 0; i < decimal ;i++) {
		wstr[1] = wstr[0];
		wstr--;
	}

	// put the dot
	if(wstr[2] == '\0') { wstr[1] = '\0'; decimal--; }
	else wstr[1] = '.';

	// return the buffer length
	return ( (wstr - str) + 2 + decimal );
}
