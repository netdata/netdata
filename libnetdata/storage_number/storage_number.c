// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

bool is_system_ieee754_double(void) {
    static bool logged = false;

    struct {
        NETDATA_DOUBLE original;

        union {
            uint64_t i;
            NETDATA_DOUBLE d;
        };
    } tests[] = {
            { .original = 1.25,                 .i = 0x3FF4000000000000 },
            { .original = 1.0,                  .i = 0x3FF0000000000000 },
            { .original = 2.0,                  .i = 0x4000000000000000 },
            { .original = 4.0,                  .i = 0x4010000000000000 },
            { .original = 8.8,                  .i = 0x402199999999999A },
            { .original = 16.16,                .i = 0x403028F5C28F5C29 },
            { .original = 32.32,                .i = 0x404028F5C28F5C29 },
            { .original = 64.64,                .i = 0x405028F5C28F5C29 },
            { .original = 128.128,              .i = 0x406004189374BC6A },
            { .original = 32768.32768,          .i = 0x40E0000A7C5AC472 },
            { .original = 65536.65536,          .i = 0x40F0000A7C5AC472 },
            { .original = -65536.65536,         .i = 0xC0F0000A7C5AC472 },
            { .original = 65535.65535,          .i = 0x40EFFFF4F8A0902E },
            { .original = -65535.65535,         .i = 0xC0EFFFF4F8A0902E },
            { .original = 4.503599627e15,       .i = 0x432FFFFFFFF4B180 },
            { .original = -4.503599627e15,      .i = 0xC32FFFFFFFF4B180 },
            { .original = 1.25e25,              .i = 0x4524ADF4B7320335 },
            { .original = 1.25e307,             .i = 0x7FB1CCF385EBC8A0 },
            { .original = 1.25e-25,             .i = 0x3AC357C299A88EA7 },
            { .original = 1.25e-100,            .i = 0x2B317F7D4ED8C33E },
            { .original = NAN,                  .i = 0x7FF8000000000000 },
            { .original = -INFINITY,            .i = 0xFFF0000000000000 },
            { .original = INFINITY,             .i = 0x7FF0000000000000 },
            { .original = 1.25e-132,            .i = 0x248C6463225AB7EC },
            { .original = 0.0,                  .i = 0x0000000000000000 },
            { .original = -0.0,                 .i = 0x8000000000000000 },
            { .original = DBL_MIN,              .i = 0x0010000000000000 },
            { .original = DBL_MAX,              .i = 0x7FEFFFFFFFFFFFFF },
            { .original = -DBL_MIN,             .i = 0x8010000000000000 },
            { .original = -DBL_MAX,             .i = 0xFFEFFFFFFFFFFFFF },
    };

    size_t errors = 0;
    size_t elements = sizeof(tests) / sizeof(tests[0]);
    for(size_t i = 0; i < elements ; i++) {
        uint64_t *ptr = (uint64_t *)&tests[i].original;

        if(*ptr != tests[i].i && (tests[i].original == tests[i].d || (isnan(tests[i].original) && isnan(tests[i].d)))) {
            if(!logged)
                info("IEEE754: test #%zu, value " NETDATA_DOUBLE_FORMAT_G " is represented in this system as %lX, but it was expected as %lX",
                     i+1, tests[i].original, *ptr, tests[i].i);
            errors++;
        }
    }

    if(!errors && sizeof(NETDATA_DOUBLE) == sizeof(uint64_t)) {
        if(!logged)
            info("IEEE754: system is using IEEE754 DOUBLE PRECISION values");

        logged = true;
        return true;
    }
    else {
        if(!logged)
            info("IEEE754: system is NOT compatible with IEEE754 DOUBLE PRECISION values");

        logged = true;
        return false;
    }
}

storage_number pack_storage_number(NETDATA_DOUBLE value, SN_FLAGS flags) {
    // bit 32 = sign 0:positive, 1:negative
    // bit 31 = 0:divide, 1:multiply
    // bit 30, 29, 28 = (multiplier or divider) 0-7 (8 total)
    // bit 27 SN_EXISTS_100
    // bit 26 SN_EXISTS_RESET
    // bit 25 SN_ANOMALY_BIT = 0: anomalous, 1: not anomalous
    // bit 24 to bit 1 = the value

    if(unlikely(fpclassify(value) == FP_NAN || fpclassify(value) == FP_INFINITE))
        return SN_EMPTY_SLOT;

    storage_number r = flags & SN_USER_FLAGS;

    if(unlikely(fpclassify(value) == FP_ZERO || fpclassify(value) == FP_SUBNORMAL))
        return r;

    int m = 0;
    NETDATA_DOUBLE n = value, factor = 10;

    // if the value is negative
    // add the sign bit and make it positive
    if(n < 0) {
        r += SN_FLAG_NEGATIVE; // the sign bit 32
        n = -n;
    }

    if(n / 10000000.0 > 0x00ffffff) {
        factor = 100;
        r |= SN_FLAG_NOT_EXISTS_MUL100;
    }

    // make its integer part fit in 0x00ffffff
    // by dividing it by 10 up to 7 times
    // and increasing the multiplier
    while(m < 7 && n > (NETDATA_DOUBLE)0x00ffffff) {
        n /= factor;
        m++;
    }

    if(m) {
        // the value was too big, and we divided it
        // so, we add a multiplier to unpack it
        r += SN_FLAG_MULTIPLY + (m << 27); // the multiplier m

        if(n > (NETDATA_DOUBLE)0x00ffffff) {
            #ifdef NETDATA_INTERNAL_CHECKS
            error("Number " NETDATA_DOUBLE_FORMAT " is too big.", value);
            #endif
            r += 0x00ffffff;
            return r;
        }
    }
    else {
        // 0x0019999e is the number that can be multiplied
        // by 10 to give 0x00ffffff
        // while the value is below 0x0019999e we can
        // multiply it by 10, up to 7 times, increasing
        // the multiplier
        while(m < 7 && n < (NETDATA_DOUBLE)0x0019999e) {
            n *= 10;
            m++;
        }

        if (unlikely(n > (NETDATA_DOUBLE)0x00ffffff)) {
            n /= 10;
            m--;
        }
        // the value was small enough, and we multiplied it
        // so, we add a divider to unpack it
        r += (m << 27); // the divider m
    }

#ifdef STORAGE_WITH_MATH
    // without this there are rounding problems
    // example: 0.9 becomes 0.89
    r += lrint((double) n);
#else
    r += (storage_number)n;
#endif

    return r;
}

// Lookup table to make storage number unpacking efficient.
NETDATA_DOUBLE unpack_storage_number_lut10x[4 * 8];

__attribute__((constructor)) void initialize_lut(void) {
    // The lookup table is partitioned in 4 subtables based on the
    // values of the factor and exp bits.
    for (int i = 0; i < 8; i++) {
        // factor = 0
        unpack_storage_number_lut10x[0 * 8 + i] = 1 / pow(10, i);    // exp = 0
        unpack_storage_number_lut10x[1 * 8 + i] = pow(10, i);        // exp = 1

        // factor = 1
        unpack_storage_number_lut10x[2 * 8 + i] = 1 / pow(100, i);   // exp = 0
        unpack_storage_number_lut10x[3 * 8 + i] = pow(100, i);       // exp = 1
    }
}

/*
int print_netdata_double(char *str, NETDATA_DOUBLE value)
{
    char *wstr = str;

    int sign = (value < 0) ? 1 : 0;
    if(sign) value = -value;

#ifdef STORAGE_WITH_MATH
    // without llrintl() there are rounding problems
    // for example 0.9 becomes 0.89
    unsigned long long uvalue = (unsigned long long int) llrintl(value * (NETDATA_DOUBLE)100000);
#else
    unsigned long long uvalue = value * (NETDATA_DOUBLE)100000;
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
