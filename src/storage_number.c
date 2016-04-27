/* vim: set ts=4 noet sw=4 : */
#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#ifdef STORAGE_WITH_MATH
#include <math.h>
#endif

#include "common.h"
#include "log.h"
#include "storage_number.h"

//#if __GNUC__
//#if __x86_64__ || __ppc64__
//#define ENVIRONMENT64
//#else
//#define ENVIRONMENT32
//#endif
//#endif
//
//// FIXME: Insted of using this awkward decimal floating point encoding,
////        we could use 'float' instead, since the precision is 24 bits!
//storage_number pack_storage_number(calculated_number value, uint32_t flags)
//{
//	// bit 31 = sign 0:positive, 1:negative
//	// bit 30 = 0:divide, 1:multiply
//	// bit 29, 28, 27 = (multiplier or divider) 0-6 (7 total)
//	// bit 26, 25, 24 flags
//	// bit 23 to bit 0 = the value
//
//	storage_number r = get_storage_number_flags(flags);
//	if(!value) return r;
//
//	int m = 0;
//	calculated_number n = value;
//
//	// if the value is negative
//	// add the sign bit and make it positive
//	if(n < 0) {
//		r |= SN_SIGN_BIT;
//		n = -n;
//	}
//
//	// FIXME: This means the mantissa is only 24 bits long.
//	//        this can fit on a single float type!
//
//	// make its integer part fit in 0x00ffffff
//	// by dividing it by 10 up to 7 times
//	// and increasing the multiplier
//	while(m < 7 && n > (calculated_number)SN_VALUE_MASK) {
//		n /= 10;
//		m++;
//	}
//
//	// If multiplier was needed...
//	if(m) {
//		// the value was too big and we divided it
//		// so we add a multiplier to unpack it
//		r |= SN_MULTIPLIER_BIT + (m << 27); // the multiplier m
//
//		// NOTE: Saturate, if necessary.
//		if(n > (calculated_number)SN_VALUE_MASK) {
//			error("Number " CALCULATED_NUMBER_FORMAT " is too big.", value);
//			r |= SN_VALUE_MASK;	// ?!?!
//			return r;
//		}
//	}
//	else {
//		// NOTE: Normalization?!
//		
//		// 0x0019999e is the number that can be multiplied
//		// by 10 to give 0x00ffffff
//		// while the value is below 0x0019999e we can
//		// multiply it by 10, up to 7 times, increasing
//		// the multiplier
//		while(m < 7 && n < (calculated_number)0x0019999e) {
//			n *= 10;
//			m++;
//		}
//
//		// the value was small enough and we multiplied it
//		// so we add a divider to unpack it
//		r |= /*(0 << 30) + */ (m << 27); // the divider m
//	}
//
//#ifdef STORAGE_WITH_MATH
//	// without this there are rounding problems
//	// example: 0.9 becomes 0.89
//	r += lrint((double)n);	// NOTE: Before 'n' was defined as long double.
//							//       this rounding to 'double' and, later to
//							//       int is proof 'long double' isn't needed!
//#else
//	r += (storage_number)n;
//#endif
//
//	return r;
//}
//
//// NOTE: Don't bother with 'flags' semantics!
//calculated_number unpack_storage_number(storage_number value)
//{
//	if(!value) return 0;
//
//	int sign = 0, exp = 0;
//
//	// FIX: It is better this way.
//	//value ^= get_storage_number_flags(value);
//	value &= ~SN_FLAGS_MASK;
//
//	if(value & SN_SIGN_BIT)) {
//		sign = 1;
//		value ^= SN_SIGN_BIT;
//	}
//
//	if(value & SN_MULTIPLIER_BIT) {
//		exp = 1;
//		value ^= SN_MULTIPLIER_BIT;
//	}
//
//	int mul = value >> 27;
//	/* FIX: It's better this way. */
//	//value ^= mul << 27;
//	value &= SN_VALUE_MASK;
//
//	calculated_number n = value;
//
//	// NOTE: Point floating...
//	while(mul > 0) {
//		if(exp) 
//			n *= 10; 
//		else 
//			n /= 10;
//		mul--;
//	}
//
//	// NOTE: Adjust sign.
//	if(sign) n = -n;
//
//	return n;
//}
//
//#ifdef ENVIRONMENT32
//// This trick seems to give an 80% speed increase in 32bit systems
//// print_calculated_number_llu_r() will just print the digits up to the
//// point the remaining value fits in 32 bits, and then calls
//// print_calculated_number_lu_r() to print the rest with 32 bit arithmetic.
//
//static char *print_calculated_number_lu_r(char *str, unsigned long uvalue) {
//	char *wstr = str;
//
//	// print each digit
//	do *wstr++ = (char)('0' + (uvalue % 10)); while(uvalue /= 10);
//	return wstr;
//}
//
//static char *print_calculated_number_llu_r(char *str, unsigned long long uvalue) {
//	char *wstr = str;
//
//	// print each digit
//	do *wstr++ = (char)('0' + (uvalue % 10)); while((uvalue /= 10) && uvalue > (unsigned long long)0xffffffff);
//	if(uvalue) return print_calculated_number_lu_r(wstr, uvalue);
//	return wstr;
//}
//#endif
//
//int print_calculated_number(char *str, calculated_number value)
//{
//	char *wstr = str;
//
//	int sign = (value < 0) ? 1 : 0;
//	if(sign) value = -value;
//
//#ifdef STORAGE_WITH_MATH
//	// without llrint() there are rounding problems
//	// for example 0.9 becomes 0.89
//	unsigned long long uvalue = (unsigned long long int) llrint(value * (calculated_number)100000);
//#else
//	unsigned long long uvalue = value * (calculated_number)100000;
//#endif
//
//#ifdef ENVIRONMENT32
//	if(uvalue > (unsigned long long)0xffffffff)
//		wstr = print_calculated_number_llu_r(str, uvalue);
//	else
//		wstr = print_calculated_number_lu_r(str, uvalue);
//#else
//	do *wstr++ = (char)('0' + (uvalue % 10)); while(uvalue /= 10);
//#endif
//
//	// make sure we have 6 bytes at least
//	while((wstr - str) < 6) *wstr++ = '0';
//
//	// put the sign back
//	if(sign) *wstr++ = '-';
//
//	// reverse it
//	char *begin = str, *end = --wstr, aux;
//	while (end > begin) aux = *end, *end-- = *begin, *begin++ = aux;
//	// wstr--;
//	// strreverse(str, wstr);
//
//	// remove trailing zeros
//	int decimal = 5;
//	while(decimal > 0 && *wstr == '0') {
//		*wstr-- = '\0';
//		decimal--;
//	}
//
//	// terminate it, one position to the right
//	// to let space for a dot
//	wstr[2] = '\0';
//
//	// make space for the dot
//	int i;
//	for(i = 0; i < decimal ;i++) {
//		wstr[1] = wstr[0];
//		wstr--;
//	}
//
//	// put the dot
//	if(wstr[2] == '\0') { wstr[1] = '\0'; decimal--; }
//	else wstr[1] = '.';
//
//	// return the buffer length
//	return (int) ((wstr - str) + 2 + decimal );
//}


storage_number pack_storage_number(calculated_number value, int flags)
{
	storage_number tmp = { fabs(value), flags == SN_EXISTS, flags == SN_EXISTS_RESET };

	if (tmp.value > STORAGE_NUMBER_POSITIVE_MAX)
		tmp.value = STORAGE_NUMBER_POSITIVE_MAX;
	if (tmp.value != 0.0 && tmp.value < STORAGE_NUMBER_POSITIVE_MIN)
		tmp.value = STORAGE_NUMBER_POSITIVE_MIN;
	if (value < 0.0) tmp.value = -tmp.value;

	return tmp;
}
