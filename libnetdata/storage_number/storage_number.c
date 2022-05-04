// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#define get_storage_number_flags(value) \
    ((((storage_number)(value)) & (1 << 24)) | \
     (((storage_number)(value)) & (1 << 25)) | \
     (((storage_number)(value)) & (1 << 26)))

storage_number pack_storage_number(calculated_number value, uint32_t flags) {
    // bit 32 = sign 0:positive, 1:negative
    // bit 31 = 0:divide, 1:multiply
    // bit 30, 29, 28 = (multiplier or divider) 0-7 (8 total)
    // bit 27 SN_EXISTS_100
    // bit 26 SN_EXISTS_RESET
    // bit 25 SN_ANOMALY_BIT = 0: anomalous, 1: not anomalous
    // bit 24 to bit 1 = the value

    storage_number r = get_storage_number_flags(flags);
    if(!value)
        goto RET_SN;

    int m = 0;
    calculated_number n = value, factor = 10;

    // if the value is negative
    // add the sign bit and make it positive
    if(n < 0) {
        r += (1 << 31); // the sign bit 32
        n = -n;
    }

    if(n / 10000000.0 > 0x00ffffff) {
        factor = 100;
        r |= SN_EXISTS_100;
    }

    // make its integer part fit in 0x00ffffff
    // by dividing it by 10 up to 7 times
    // and increasing the multiplier
    while(m < 7 && n > (calculated_number)0x00ffffff) {
        n /= factor;
        m++;
    }

    if(m) {
        // the value was too big and we divided it
        // so we add a multiplier to unpack it
        r += (1 << 30) + (m << 27); // the multiplier m

        if(n > (calculated_number)0x00ffffff) {
            #ifdef NETDATA_INTERNAL_CHECKS
            error("Number " CALCULATED_NUMBER_FORMAT " is too big.", value);
            #endif
            r += 0x00ffffff;
            goto RET_SN;
        }
    }
    else {
        // 0x0019999e is the number that can be multiplied
        // by 10 to give 0x00ffffff
        // while the value is below 0x0019999e we can
        // multiply it by 10, up to 7 times, increasing
        // the multiplier
        while(m < 7 && n < (calculated_number)0x0019999e) {
            n *= 10;
            m++;
        }

        if (unlikely(n > (calculated_number) (0x00ffffff))) {
            n /= 10;
            m--;
        }
        // the value was small enough and we multiplied it
        // so we add a divider to unpack it
        r += (0 << 30) + (m << 27); // the divider m
    }

#ifdef STORAGE_WITH_MATH
    // without this there are rounding problems
    // example: 0.9 becomes 0.89
    r += lrint((double) n);
#else
    r += (storage_number)n;
#endif

RET_SN:
    if (r == SN_EMPTY_SLOT)
        r = SN_ANOMALOUS_ZERO;

    return r;
}

// Lookup table to make storage number unpacking efficient.
static calculated_number lut10x[4 * 8];

__attribute__((constructor)) void initialize_lut(void) {
    // The lookup table is partitioned in 4 subtables based on the
    // values of the factor and exp bits.
    for (int i = 0; i < 8; i++) {
        // factor = 0
        lut10x[0 * 8 + i] = 1 / pow(10, i);    // exp = 0
        lut10x[1 * 8 + i] = pow(10, i);        // exp = 1

        // factor = 1
        lut10x[2 * 8 + i] = 1 / pow(100, i);   // exp = 0
        lut10x[3 * 8 + i] = pow(100, i);       // exp = 1
    }
}

calculated_number unpack_storage_number(storage_number value) {
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

    return sign * lut10x[(factor * 16) + (exp * 8) + mul] * n;
}

/*
int print_calculated_number(char *str, calculated_number value)
{
    char *wstr = str;

    int sign = (value < 0) ? 1 : 0;
    if(sign) value = -value;

#ifdef STORAGE_WITH_MATH
    // without llrintl() there are rounding problems
    // for example 0.9 becomes 0.89
    unsigned long long uvalue = (unsigned long long int) llrintl(value * (calculated_number)100000);
#else
    unsigned long long uvalue = value * (calculated_number)100000;
#endif

    wstr = print_number_llu_r_smart(str, uvalue);

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
    return (int) ((wstr - str) + 2 + decimal );
}
*/

int print_calculated_number(char *str, calculated_number value) {
    // info("printing number " CALCULATED_NUMBER_FORMAT, value);
    char integral_str[50], fractional_str[50];

    char *wstr = str;

    if(unlikely(value < 0)) {
        *wstr++ = '-';
        value = -value;
    }

    calculated_number integral, fractional;

#ifdef STORAGE_WITH_MATH
    fractional = calculated_number_modf(value, &integral) * 10000000.0;
#else
    fractional = ((unsigned long long)(value * 10000000ULL) % 10000000ULL);
#endif

    unsigned long long integral_int = (unsigned long long)integral;
    unsigned long long fractional_int = (unsigned long long)calculated_number_llrint(fractional);
    if(unlikely(fractional_int >= 10000000)) {
        integral_int += 1;
        fractional_int -= 10000000;
    }

    // info("integral " CALCULATED_NUMBER_FORMAT " (%llu), fractional " CALCULATED_NUMBER_FORMAT " (%llu)", integral, integral_int, fractional, fractional_int);

    char *istre;
    if(unlikely(integral_int == 0)) {
        integral_str[0] = '0';
        istre = &integral_str[1];
    }
    else
        // convert the integral part to string (reversed)
        istre = print_number_llu_r_smart(integral_str, integral_int);

    // copy reversed the integral string
    istre--;
    while( istre >= integral_str ) *wstr++ = *istre--;

    if(likely(fractional_int != 0)) {
        // add a dot
        *wstr++ = '.';

        // convert the fractional part to string (reversed)
        char *fstre = print_number_llu_r_smart(fractional_str, fractional_int);

        // prepend zeros to reach 7 digits length
        int decimal = 7;
        int len = (int)(fstre - fractional_str);
        while(len < decimal) {
            *wstr++ = '0';
            len++;
        }

        char *begin = fractional_str;
        while(begin < fstre && *begin == '0') begin++;

        // copy reversed the fractional string
        fstre--;
        while( fstre >= begin ) *wstr++ = *fstre--;
    }

    *wstr = '\0';
    // info("printed number '%s'", str);
    return (int)(wstr - str);
}
