#include "common.h"

static int check_number_printing(void) {
    struct {
        calculated_number n;
        const char *correct;
    } values[] = {
            { .n = 0, .correct = "0" },
            { .n = 0.0000001,   .correct = "0.0000001" },
            { .n = 0.00000009,  .correct = "0.0000001" },
            { .n = 0.000000001, .correct = "0" },
            { .n = 99.99999999999999999, .correct = "100" },
            { .n = -99.99999999999999999, .correct = "-100" },
            { .n = 123.4567890123456789, .correct = "123.456789" },
            { .n = 9999.9999999, .correct = "9999.9999999" },
            { .n = -9999.9999999, .correct = "-9999.9999999" },
            { .n = 0, .correct = NULL },
    };

    char netdata[50], system[50];
    int i, failed = 0;
    for(i = 0; values[i].correct ; i++) {
        print_calculated_number(netdata, values[i].n);
        snprintfz(system, 49, "%0.12" LONG_DOUBLE_MODIFIER, (LONG_DOUBLE)values[i].n);

        int ok = 1;
        if(strcmp(netdata, values[i].correct) != 0) {
            ok = 0;
            failed++;
        }

        fprintf(stderr, "'%s' (system) printed as '%s' (netdata): %s\n", system, netdata, ok?"OK":"FAILED");
    }

    if(failed) return 1;
    return 0;
}

static int check_rrdcalc_comparisons(void) {
    RRDCALC_STATUS a, b;

    // make sure calloc() sets the status to UNINITIALIZED
    memset(&a, 0, sizeof(RRDCALC_STATUS));
    if(a != RRDCALC_STATUS_UNINITIALIZED) {
        fprintf(stderr, "%s is not zero.\n", rrdcalc_status2string(RRDCALC_STATUS_UNINITIALIZED));
        return 1;
    }

    a = RRDCALC_STATUS_REMOVED;
    b = RRDCALC_STATUS_UNDEFINED;
    if(!(a < b)) {
        fprintf(stderr, "%s is not less than %s\n", rrdcalc_status2string(a), rrdcalc_status2string(b));
        return 1;
    }

    a = RRDCALC_STATUS_UNDEFINED;
    b = RRDCALC_STATUS_UNINITIALIZED;
    if(!(a < b)) {
        fprintf(stderr, "%s is not less than %s\n", rrdcalc_status2string(a), rrdcalc_status2string(b));
        return 1;
    }

    a = RRDCALC_STATUS_UNINITIALIZED;
    b = RRDCALC_STATUS_CLEAR;
    if(!(a < b)) {
        fprintf(stderr, "%s is not less than %s\n", rrdcalc_status2string(a), rrdcalc_status2string(b));
        return 1;
    }

    a = RRDCALC_STATUS_CLEAR;
    b = RRDCALC_STATUS_RAISED;
    if(!(a < b)) {
        fprintf(stderr, "%s is not less than %s\n", rrdcalc_status2string(a), rrdcalc_status2string(b));
        return 1;
    }

    a = RRDCALC_STATUS_RAISED;
    b = RRDCALC_STATUS_WARNING;
    if(!(a < b)) {
        fprintf(stderr, "%s is not less than %s\n", rrdcalc_status2string(a), rrdcalc_status2string(b));
        return 1;
    }

    a = RRDCALC_STATUS_WARNING;
    b = RRDCALC_STATUS_CRITICAL;
    if(!(a < b)) {
        fprintf(stderr, "%s is not less than %s\n", rrdcalc_status2string(a), rrdcalc_status2string(b));
        return 1;
    }

    fprintf(stderr, "RRDCALC_STATUSes are sortable.\n");

    return 0;
}

int check_storage_number(calculated_number n, int debug) {
    char buffer[100];
    uint32_t flags = SN_EXISTS;

    storage_number s = pack_storage_number(n, flags);
    calculated_number d = unpack_storage_number(s);

    if(!does_storage_number_exist(s)) {
        fprintf(stderr, "Exists flags missing for number " CALCULATED_NUMBER_FORMAT "!\n", n);
        return 5;
    }

    calculated_number ddiff = d - n;
    calculated_number dcdiff = ddiff * 100.0 / n;

    if(dcdiff < 0) dcdiff = -dcdiff;

    size_t len = (size_t)print_calculated_number(buffer, d);
    calculated_number p = str2ld(buffer, NULL);
    calculated_number pdiff = n - p;
    calculated_number pcdiff = pdiff * 100.0 / n;
    if(pcdiff < 0) pcdiff = -pcdiff;

    if(debug) {
        fprintf(stderr,
            CALCULATED_NUMBER_FORMAT " original\n"
            CALCULATED_NUMBER_FORMAT " packed and unpacked, (stored as 0x%08X, diff " CALCULATED_NUMBER_FORMAT ", " CALCULATED_NUMBER_FORMAT "%%)\n"
            "%s printed after unpacked (%zu bytes)\n"
            CALCULATED_NUMBER_FORMAT " re-parsed from printed (diff " CALCULATED_NUMBER_FORMAT ", " CALCULATED_NUMBER_FORMAT "%%)\n\n",
            n,
            d, s, ddiff, dcdiff,
            buffer, len,
            p, pdiff, pcdiff
        );
        if(len != strlen(buffer)) fprintf(stderr, "ERROR: printed number %s is reported to have length %zu but it has %zu\n", buffer, len, strlen(buffer));
        if(dcdiff > ACCURACY_LOSS) fprintf(stderr, "WARNING: packing number " CALCULATED_NUMBER_FORMAT " has accuracy loss " CALCULATED_NUMBER_FORMAT " %%\n", n, dcdiff);
        if(pcdiff > ACCURACY_LOSS) fprintf(stderr, "WARNING: re-parsing the packed, unpacked and printed number " CALCULATED_NUMBER_FORMAT " has accuracy loss " CALCULATED_NUMBER_FORMAT " %%\n", n, pcdiff);
    }

    if(len != strlen(buffer)) return 1;
    if(dcdiff > ACCURACY_LOSS) return 3;
    if(pcdiff > ACCURACY_LOSS) return 4;
    return 0;
}

calculated_number storage_number_min(calculated_number n) {
    calculated_number r = 1, last;

    do {
        last = n;
        n /= 2.0;
        storage_number t = pack_storage_number(n, SN_EXISTS);
        r = unpack_storage_number(t);
    } while(r != 0.0 && r != last);

    return last;
}

void benchmark_storage_number(int loop, int multiplier) {
    int i, j;
    calculated_number n, d;
    storage_number s;
    unsigned long long user, system, total, mine, their;

    char buffer[100];

    struct rusage now, last;

    fprintf(stderr, "\n\nBenchmarking %d numbers, please wait...\n\n", loop);

    // ------------------------------------------------------------------------

    fprintf(stderr, "SYSTEM  LONG DOUBLE    SIZE: %zu bytes\n", sizeof(calculated_number));
    fprintf(stderr, "NETDATA FLOATING POINT SIZE: %zu bytes\n", sizeof(storage_number));

    mine = (calculated_number)sizeof(storage_number) * (calculated_number)loop;
    their = (calculated_number)sizeof(calculated_number) * (calculated_number)loop;

    if(mine > their) {
        fprintf(stderr, "\nNETDATA NEEDS %0.2" LONG_DOUBLE_MODIFIER " TIMES MORE MEMORY. Sorry!\n", (LONG_DOUBLE)(mine / their));
    }
    else {
        fprintf(stderr, "\nNETDATA INTERNAL FLOATING POINT ARITHMETICS NEEDS %0.2" LONG_DOUBLE_MODIFIER " TIMES LESS MEMORY.\n", (LONG_DOUBLE)(their / mine));
    }

    fprintf(stderr, "\nNETDATA FLOATING POINT\n");
    fprintf(stderr, "MIN POSITIVE VALUE " CALCULATED_NUMBER_FORMAT "\n", storage_number_min(1));
    fprintf(stderr, "MAX POSITIVE VALUE " CALCULATED_NUMBER_FORMAT "\n", (calculated_number)STORAGE_NUMBER_POSITIVE_MAX);
    fprintf(stderr, "MIN NEGATIVE VALUE " CALCULATED_NUMBER_FORMAT "\n", (calculated_number)STORAGE_NUMBER_NEGATIVE_MIN);
    fprintf(stderr, "MAX NEGATIVE VALUE " CALCULATED_NUMBER_FORMAT "\n", -storage_number_min(1));
    fprintf(stderr, "Maximum accuracy loss: " CALCULATED_NUMBER_FORMAT "%%\n\n\n", (calculated_number)ACCURACY_LOSS);

    // ------------------------------------------------------------------------

    fprintf(stderr, "INTERNAL LONG DOUBLE PRINTING: ");
    getrusage(RUSAGE_SELF, &last);

    // do the job
    for(j = 1; j < 11 ;j++) {
        n = STORAGE_NUMBER_POSITIVE_MIN * j;

        for(i = 0; i < loop ;i++) {
            n *= multiplier;
            if(n > STORAGE_NUMBER_POSITIVE_MAX) n = STORAGE_NUMBER_POSITIVE_MIN;

            print_calculated_number(buffer, n);
        }
    }

    getrusage(RUSAGE_SELF, &now);
    user   = now.ru_utime.tv_sec * 1000000ULL + now.ru_utime.tv_usec - last.ru_utime.tv_sec * 1000000ULL + last.ru_utime.tv_usec;
    system = now.ru_stime.tv_sec * 1000000ULL + now.ru_stime.tv_usec - last.ru_stime.tv_sec * 1000000ULL + last.ru_stime.tv_usec;
    total  = user + system;
    mine = total;

    fprintf(stderr, "user %0.5" LONG_DOUBLE_MODIFIER", system %0.5" LONG_DOUBLE_MODIFIER ", total %0.5" LONG_DOUBLE_MODIFIER "\n", (LONG_DOUBLE)(user / 1000000.0), (LONG_DOUBLE)(system / 1000000.0), (LONG_DOUBLE)(total / 1000000.0));

    // ------------------------------------------------------------------------

    fprintf(stderr, "SYSTEM   LONG DOUBLE PRINTING: ");
    getrusage(RUSAGE_SELF, &last);

    // do the job
    for(j = 1; j < 11 ;j++) {
        n = STORAGE_NUMBER_POSITIVE_MIN * j;

        for(i = 0; i < loop ;i++) {
            n *= multiplier;
            if(n > STORAGE_NUMBER_POSITIVE_MAX) n = STORAGE_NUMBER_POSITIVE_MIN;
            snprintfz(buffer, 100, CALCULATED_NUMBER_FORMAT, n);
        }
    }

    getrusage(RUSAGE_SELF, &now);
    user   = now.ru_utime.tv_sec * 1000000ULL + now.ru_utime.tv_usec - last.ru_utime.tv_sec * 1000000ULL + last.ru_utime.tv_usec;
    system = now.ru_stime.tv_sec * 1000000ULL + now.ru_stime.tv_usec - last.ru_stime.tv_sec * 1000000ULL + last.ru_stime.tv_usec;
    total  = user + system;
    their = total;

    fprintf(stderr, "user %0.5" LONG_DOUBLE_MODIFIER ", system %0.5" LONG_DOUBLE_MODIFIER ", total %0.5" LONG_DOUBLE_MODIFIER "\n", (LONG_DOUBLE)(user / 1000000.0), (LONG_DOUBLE)(system / 1000000.0), (LONG_DOUBLE)(total / 1000000.0));

    if(mine > total) {
        fprintf(stderr, "NETDATA CODE IS SLOWER %0.2" LONG_DOUBLE_MODIFIER " %%\n", (LONG_DOUBLE)(mine * 100.0 / their - 100.0));
    }
    else {
        fprintf(stderr, "NETDATA CODE IS  F A S T E R  %0.2" LONG_DOUBLE_MODIFIER " %%\n", (LONG_DOUBLE)(their * 100.0 / mine - 100.0));
    }

    // ------------------------------------------------------------------------

    fprintf(stderr, "\nINTERNAL LONG DOUBLE PRINTING WITH PACK / UNPACK: ");
    getrusage(RUSAGE_SELF, &last);

    // do the job
    for(j = 1; j < 11 ;j++) {
        n = STORAGE_NUMBER_POSITIVE_MIN * j;

        for(i = 0; i < loop ;i++) {
            n *= multiplier;
            if(n > STORAGE_NUMBER_POSITIVE_MAX) n = STORAGE_NUMBER_POSITIVE_MIN;

            s = pack_storage_number(n, 1);
            d = unpack_storage_number(s);
            print_calculated_number(buffer, d);
        }
    }

    getrusage(RUSAGE_SELF, &now);
    user   = now.ru_utime.tv_sec * 1000000ULL + now.ru_utime.tv_usec - last.ru_utime.tv_sec * 1000000ULL + last.ru_utime.tv_usec;
    system = now.ru_stime.tv_sec * 1000000ULL + now.ru_stime.tv_usec - last.ru_stime.tv_sec * 1000000ULL + last.ru_stime.tv_usec;
    total  = user + system;
    mine = total;

    fprintf(stderr, "user %0.5" LONG_DOUBLE_MODIFIER ", system %0.5" LONG_DOUBLE_MODIFIER ", total %0.5" LONG_DOUBLE_MODIFIER "\n", (LONG_DOUBLE)(user / 1000000.0), (LONG_DOUBLE)(system / 1000000.0), (LONG_DOUBLE)(total / 1000000.0));

    if(mine > their) {
        fprintf(stderr, "WITH PACKING UNPACKING NETDATA CODE IS SLOWER %0.2" LONG_DOUBLE_MODIFIER " %%\n", (LONG_DOUBLE)(mine * 100.0 / their - 100.0));
    }
    else {
        fprintf(stderr, "EVEN WITH PACKING AND UNPACKING, NETDATA CODE IS  F A S T E R  %0.2" LONG_DOUBLE_MODIFIER " %%\n", (LONG_DOUBLE)(their * 100.0 / mine - 100.0));
    }

    // ------------------------------------------------------------------------

}

static int check_storage_number_exists() {
    uint32_t flags = SN_EXISTS;


    for(flags = 0; flags < 7 ; flags++) {
        if(get_storage_number_flags(flags << 24) != flags << 24) {
            fprintf(stderr, "Flag 0x%08x is not checked correctly. It became 0x%08x\n", flags << 24, get_storage_number_flags(flags << 24));
            return 1;
        }
    }

    flags = SN_EXISTS;
    calculated_number n = 0.0;

    storage_number s = pack_storage_number(n, flags);
    calculated_number d = unpack_storage_number(s);
    if(get_storage_number_flags(s) != flags) {
        fprintf(stderr, "Wrong flags. Given %08x, Got %08x!\n", flags, get_storage_number_flags(s));
        return 1;
    }
    if(n != d) {
        fprintf(stderr, "Wrong number returned. Expected " CALCULATED_NUMBER_FORMAT ", returned " CALCULATED_NUMBER_FORMAT "!\n", n, d);
        return 1;
    }

    return 0;
}

int unit_test_storage()
{
    if(check_storage_number_exists()) return 0;

    calculated_number c, a = 0;
    int i, j, g, r = 0;

    for(g = -1; g <= 1 ; g++) {
        a = 0;

        if(!g) continue;

        for(j = 0; j < 9 ;j++) {
            a += 0.0000001;
            c = a * g;
            for(i = 0; i < 21 ;i++, c *= 10) {
                if(c > 0 && c < STORAGE_NUMBER_POSITIVE_MIN) continue;
                if(c < 0 && c > STORAGE_NUMBER_NEGATIVE_MAX) continue;

                if(check_storage_number(c, 1)) return 1;
            }
        }
    }

    benchmark_storage_number(1000000, 2);
    return r;
}

int unit_test_str2ld() {
    char *values[] = {
            "1.2345678", "-35.6", "0.00123", "23842384234234.2", ".1", "1.2e-10",
            "hello", "1wrong", "nan", "inf", NULL
    };

    int i;
    for(i = 0; values[i] ; i++) {
        char *e_mine = "hello", *e_sys = "world";
        LONG_DOUBLE mine = str2ld(values[i], &e_mine);
        LONG_DOUBLE sys = strtold(values[i], &e_sys);

        if(isnan(mine)) {
            if(!isnan(sys)) {
                fprintf(stderr, "Value '%s' is parsed as %" LONG_DOUBLE_MODIFIER ", but system believes it is %" LONG_DOUBLE_MODIFIER ".\n", values[i], mine, sys);
                return -1;
            }
        }
        else if(isinf(mine)) {
            if(!isinf(sys)) {
                fprintf(stderr, "Value '%s' is parsed as %" LONG_DOUBLE_MODIFIER ", but system believes it is %" LONG_DOUBLE_MODIFIER ".\n", values[i], mine, sys);
                return -1;
            }
        }
        else if(mine != sys && abs(mine-sys) > 0.000001) {
            fprintf(stderr, "Value '%s' is parsed as %" LONG_DOUBLE_MODIFIER ", but system believes it is %" LONG_DOUBLE_MODIFIER ", delta %" LONG_DOUBLE_MODIFIER ".\n", values[i], mine, sys, sys-mine);
            return -1;
        }

        if(e_mine != e_sys) {
            fprintf(stderr, "Value '%s' is parsed correctly, but endptr is not right\n", values[i]);
            return -1;
        }

        fprintf(stderr, "str2ld() parsed value '%s' exactly the same way with strtold(), returned %" LONG_DOUBLE_MODIFIER " vs %" LONG_DOUBLE_MODIFIER "\n", values[i], mine, sys);
    }

    return 0;
}

int unit_test_buffer() {
    BUFFER *wb = buffer_create(1);
    char string[2048 + 1];
    char final[9000 + 1];
    int i;

    for(i = 0; i < 2048; i++)
        string[i] = (char)((i % 24) + 'a');
    string[2048] = '\0';

    const char *fmt = "string1: %s\nstring2: %s\nstring3: %s\nstring4: %s";
    buffer_sprintf(wb, fmt, string, string, string, string);
    snprintfz(final, 9000, fmt, string, string, string, string);

    const char *s = buffer_tostring(wb);

    if(buffer_strlen(wb) != strlen(final) || strcmp(s, final) != 0) {
        fprintf(stderr, "\nbuffer_sprintf() is faulty.\n");
        fprintf(stderr, "\nstring  : %s (length %zu)\n", string, strlen(string));
        fprintf(stderr, "\nbuffer  : %s (length %zu)\n", s, buffer_strlen(wb));
        fprintf(stderr, "\nexpected: %s (length %zu)\n", final, strlen(final));
        buffer_free(wb);
        return -1;
    }

    fprintf(stderr, "buffer_sprintf() works as expected.\n");
    buffer_free(wb);
    return 0;
}

// --------------------------------------------------------------------------------------------------------------------

struct feed_values {
        unsigned long long microseconds;
        collected_number value;
};

struct test {
    char name[100];
    char description[1024];

    int update_every;
    unsigned long long multiplier;
    unsigned long long divisor;
    RRD_ALGORITHM algorithm;

    unsigned long feed_entries;
    unsigned long result_entries;
    struct feed_values *feed;
    calculated_number *results;

    collected_number *feed2;
    calculated_number *results2;
};

// --------------------------------------------------------------------------------------------------------------------
// test1
// test absolute values stored

struct feed_values test1_feed[] = {
        { 0, 10 },
        { 1000000, 20 },
        { 1000000, 30 },
        { 1000000, 40 },
        { 1000000, 50 },
        { 1000000, 60 },
        { 1000000, 70 },
        { 1000000, 80 },
        { 1000000, 90 },
        { 1000000, 100 },
};

calculated_number test1_results[] = {
        20, 30, 40, 50, 60, 70, 80, 90, 100
};

struct test test1 = {
        "test1",            // name
        "test absolute values stored at exactly second boundaries",
        1,                  // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_ABSOLUTE,    // algorithm
        10,                 // feed entries
        9,                  // result entries
        test1_feed,         // feed
        test1_results,      // results
        NULL,               // feed2
        NULL                // results2
};

// --------------------------------------------------------------------------------------------------------------------
// test2
// test absolute values stored in the middle of second boundaries

struct feed_values test2_feed[] = {
        { 500000, 10 },
        { 1000000, 20 },
        { 1000000, 30 },
        { 1000000, 40 },
        { 1000000, 50 },
        { 1000000, 60 },
        { 1000000, 70 },
        { 1000000, 80 },
        { 1000000, 90 },
        { 1000000, 100 },
};

calculated_number test2_results[] = {
        20, 30, 40, 50, 60, 70, 80, 90, 100
};

struct test test2 = {
        "test2",            // name
        "test absolute values stored in the middle of second boundaries",
        1,                  // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_ABSOLUTE,    // algorithm
        10,                 // feed entries
        9,                  // result entries
        test2_feed,         // feed
        test2_results,      // results
        NULL,               // feed2
        NULL                // results2
};

// --------------------------------------------------------------------------------------------------------------------
// test3

struct feed_values test3_feed[] = {
        { 0, 10 },
        { 1000000, 20 },
        { 1000000, 30 },
        { 1000000, 40 },
        { 1000000, 50 },
        { 1000000, 60 },
        { 1000000, 70 },
        { 1000000, 80 },
        { 1000000, 90 },
        { 1000000, 100 },
};

calculated_number test3_results[] = {
        10, 10, 10, 10, 10, 10, 10, 10, 10
};

struct test test3 = {
        "test3",            // name
        "test incremental values stored at exactly second boundaries",
        1,                  // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_INCREMENTAL, // algorithm
        10,                 // feed entries
        9,                  // result entries
        test3_feed,         // feed
        test3_results,      // results
        NULL,               // feed2
        NULL                // results2
};

// --------------------------------------------------------------------------------------------------------------------
// test4

struct feed_values test4_feed[] = {
        { 500000, 10 },
        { 1000000, 20 },
        { 1000000, 30 },
        { 1000000, 40 },
        { 1000000, 50 },
        { 1000000, 60 },
        { 1000000, 70 },
        { 1000000, 80 },
        { 1000000, 90 },
        { 1000000, 100 },
};

calculated_number test4_results[] = {
        10, 10, 10, 10, 10, 10, 10, 10, 10
};

struct test test4 = {
        "test4",            // name
        "test incremental values stored in the middle of second boundaries",
        1,                  // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_INCREMENTAL, // algorithm
        10,                 // feed entries
        9,                  // result entries
        test4_feed,         // feed
        test4_results,      // results
        NULL,               // feed2
        NULL                // results2
};

// --------------------------------------------------------------------------------------------------------------------
// test5

struct feed_values test5_feed[] = {
        { 500000, 1000 },
        { 1000000, 2000 },
        { 1000000, 2000 },
        { 1000000, 2000 },
        { 1000000, 3000 },
        { 1000000, 2000 },
        { 1000000, 2000 },
        { 1000000, 2000 },
        { 1000000, 2000 },
        { 1000000, 2000 },
};

calculated_number test5_results[] = {
        1000, 500, 0, 500, 500, 0, 0, 0, 0
};

struct test test5 = {
        "test5",            // name
        "test incremental values ups and downs",
        1,                  // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_INCREMENTAL, // algorithm
        10,                 // feed entries
        9,                  // result entries
        test5_feed,         // feed
        test5_results,      // results
        NULL,               // feed2
        NULL                // results2
};

// --------------------------------------------------------------------------------------------------------------------
// test6

struct feed_values test6_feed[] = {
        { 250000, 1000 },
        { 250000, 2000 },
        { 250000, 3000 },
        { 250000, 4000 },
        { 250000, 5000 },
        { 250000, 6000 },
        { 250000, 7000 },
        { 250000, 8000 },
        { 250000, 9000 },
        { 250000, 10000 },
        { 250000, 11000 },
        { 250000, 12000 },
        { 250000, 13000 },
        { 250000, 14000 },
        { 250000, 15000 },
        { 250000, 16000 },
};

calculated_number test6_results[] = {
        4000, 4000, 4000, 4000
};

struct test test6 = {
        "test6",            // name
        "test incremental values updated within the same second",
        1,                  // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_INCREMENTAL, // algorithm
        16,                 // feed entries
        4,                  // result entries
        test6_feed,         // feed
        test6_results,      // results
        NULL,               // feed2
        NULL                // results2
};

// --------------------------------------------------------------------------------------------------------------------
// test7

struct feed_values test7_feed[] = {
        { 500000, 1000 },
        { 2000000, 2000 },
        { 2000000, 3000 },
        { 2000000, 4000 },
        { 2000000, 5000 },
        { 2000000, 6000 },
        { 2000000, 7000 },
        { 2000000, 8000 },
        { 2000000, 9000 },
        { 2000000, 10000 },
};

calculated_number test7_results[] = {
        500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500
};

struct test test7 = {
        "test7",            // name
        "test incremental values updated in long durations",
        1,                  // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_INCREMENTAL, // algorithm
        10,                 // feed entries
        18,                 // result entries
        test7_feed,         // feed
        test7_results,      // results
        NULL,               // feed2
        NULL                // results2
};

// --------------------------------------------------------------------------------------------------------------------
// test8

struct feed_values test8_feed[] = {
        { 500000, 1000 },
        { 2000000, 2000 },
        { 2000000, 3000 },
        { 2000000, 4000 },
        { 2000000, 5000 },
        { 2000000, 6000 },
};

calculated_number test8_results[] = {
        1250, 2000, 2250, 3000, 3250, 4000, 4250, 5000, 5250, 6000
};

struct test test8 = {
        "test8",            // name
        "test absolute values updated in long durations",
        1,                  // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_ABSOLUTE,    // algorithm
        6,                  // feed entries
        10,                 // result entries
        test8_feed,         // feed
        test8_results,      // results
        NULL,               // feed2
        NULL                // results2
};

// --------------------------------------------------------------------------------------------------------------------
// test9

struct feed_values test9_feed[] = {
        { 250000, 1000 },
        { 250000, 2000 },
        { 250000, 3000 },
        { 250000, 4000 },
        { 250000, 5000 },
        { 250000, 6000 },
        { 250000, 7000 },
        { 250000, 8000 },
        { 250000, 9000 },
        { 250000, 10000 },
        { 250000, 11000 },
        { 250000, 12000 },
        { 250000, 13000 },
        { 250000, 14000 },
        { 250000, 15000 },
        { 250000, 16000 },
};

calculated_number test9_results[] = {
        4000, 8000, 12000, 16000
};

struct test test9 = {
        "test9",            // name
        "test absolute values updated within the same second",
        1,                  // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_ABSOLUTE,    // algorithm
        16,                 // feed entries
        4,                  // result entries
        test9_feed,         // feed
        test9_results,      // results
        NULL,               // feed2
        NULL                // results2
};

// --------------------------------------------------------------------------------------------------------------------
// test10

struct feed_values test10_feed[] = {
        { 500000,  1000 },
        { 600000,  1000 +  600 },
        { 200000,  1600 +  200 },
        { 1000000, 1800 + 1000 },
        { 200000,  2800 +  200 },
        { 2000000, 3000 + 2000 },
        { 600000,  5000 +  600 },
        { 400000,  5600 +  400 },
        { 900000,  6000 +  900 },
        { 1000000, 6900 + 1000 },
};

calculated_number test10_results[] = {
        1000, 1000, 1000, 1000, 1000, 1000, 1000
};

struct test test10 = {
        "test10",           // name
        "test incremental values updated in short and long durations",
        1,                  // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_INCREMENTAL, // algorithm
        10,                 // feed entries
        7,                  // result entries
        test10_feed,        // feed
        test10_results,     // results
        NULL,               // feed2
        NULL                // results2
};

// --------------------------------------------------------------------------------------------------------------------
// test11

struct feed_values test11_feed[] = {
        { 0, 10 },
        { 1000000, 20 },
        { 1000000, 30 },
        { 1000000, 40 },
        { 1000000, 50 },
        { 1000000, 60 },
        { 1000000, 70 },
        { 1000000, 80 },
        { 1000000, 90 },
        { 1000000, 100 },
};

collected_number test11_feed2[] = {
    10, 20, 30, 40, 50, 60, 70, 80, 90, 100
};

calculated_number test11_results[] = {
        50, 50, 50, 50, 50, 50, 50, 50, 50
};

calculated_number test11_results2[] = {
        50, 50, 50, 50, 50, 50, 50, 50, 50
};

struct test test11 = {
        "test11",           // name
        "test percentage-of-incremental-row with equal values",
        1,                  // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL,   // algorithm
        10,                 // feed entries
        9,                  // result entries
        test11_feed,        // feed
        test11_results,     // results
        test11_feed2,       // feed2
        test11_results2     // results2
};

// --------------------------------------------------------------------------------------------------------------------
// test12

struct feed_values test12_feed[] = {
        { 0, 10 },
        { 1000000, 20 },
        { 1000000, 30 },
        { 1000000, 40 },
        { 1000000, 50 },
        { 1000000, 60 },
        { 1000000, 70 },
        { 1000000, 80 },
        { 1000000, 90 },
        { 1000000, 100 },
};

collected_number test12_feed2[] = {
    10*3, 20*3, 30*3, 40*3, 50*3, 60*3, 70*3, 80*3, 90*3, 100*3
};

calculated_number test12_results[] = {
        25, 25, 25, 25, 25, 25, 25, 25, 25
};

calculated_number test12_results2[] = {
        75, 75, 75, 75, 75, 75, 75, 75, 75
};

struct test test12 = {
        "test12",           // name
        "test percentage-of-incremental-row with equal values",
        1,                  // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL,   // algorithm
        10,                 // feed entries
        9,                  // result entries
        test12_feed,        // feed
        test12_results,     // results
        test12_feed2,       // feed2
        test12_results2     // results2
};

// --------------------------------------------------------------------------------------------------------------------
// test13

struct feed_values test13_feed[] = {
        { 500000,  1000 },
        { 600000,  1000 +  600 },
        { 200000,  1600 +  200 },
        { 1000000, 1800 + 1000 },
        { 200000,  2800 +  200 },
        { 2000000, 3000 + 2000 },
        { 600000,  5000 +  600 },
        { 400000,  5600 +  400 },
        { 900000,  6000 +  900 },
        { 1000000, 6900 + 1000 },
};

calculated_number test13_results[] = {
        83.3333300, 100, 100, 100, 100, 100, 100
};

struct test test13 = {
        "test13",           // name
        "test incremental values updated in short and long durations",
        1,                  // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL,   // algorithm
        10,                 // feed entries
        7,                  // result entries
        test13_feed,        // feed
        test13_results,     // results
        NULL,               // feed2
        NULL                // results2
};

// --------------------------------------------------------------------------------------------------------------------
// test14

struct feed_values test14_feed[] = {
        {        0, 0x015397dc42151c41ULL },
        { 13573000, 0x015397e612e3ff5dULL },
        { 29969000, 0x015397f905ecdaa8ULL },
        { 29958000, 0x0153980c2a6cb5e4ULL },
        { 30054000, 0x0153981f4032fb83ULL },
        { 34952000, 0x015398355efadaccULL },
        { 25046000, 0x01539845ba4b09f8ULL },
        { 29947000, 0x0153985948bf381dULL },
        { 30054000, 0x0153986c5b9c27e2ULL },
        { 29942000, 0x0153987f888982d0ULL },
};

calculated_number test14_results[] = {
        23.1383300, 21.8515600, 21.8804600, 21.7788000, 22.0112200, 22.4386100, 22.0906100, 21.9150800
};

struct test test14 = {
        "test14",            // name
        "issue #981 with real data",
        30,                 // update_every
        8,                  // multiplier
        1000000000,         // divisor
        RRD_ALGORITHM_INCREMENTAL, // algorithm
        10,                 // feed entries
        8,                  // result entries
        test14_feed,        // feed
        test14_results,     // results
        NULL,               // feed2
        NULL                // results2
};

struct feed_values test14b_feed[] = {
        {        0, 0 },
        { 13573000, 13573000 },
        { 29969000, 13573000 + 29969000 },
        { 29958000, 13573000 + 29969000 + 29958000 },
        { 30054000, 13573000 + 29969000 + 29958000 + 30054000 },
        { 34952000, 13573000 + 29969000 + 29958000 + 30054000 + 34952000 },
        { 25046000, 13573000 + 29969000 + 29958000 + 30054000 + 34952000 + 25046000 },
        { 29947000, 13573000 + 29969000 + 29958000 + 30054000 + 34952000 + 25046000 + 29947000 },
        { 30054000, 13573000 + 29969000 + 29958000 + 30054000 + 34952000 + 25046000 + 29947000 + 30054000 },
        { 29942000, 13573000 + 29969000 + 29958000 + 30054000 + 34952000 + 25046000 + 29947000 + 30054000 + 29942000 },
};

calculated_number test14b_results[] = {
        1000000, 1000000, 1000000, 1000000, 1000000, 1000000, 1000000, 1000000
};

struct test test14b = {
        "test14b",            // name
        "issue #981 with dummy data",
        30,                 // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_INCREMENTAL, // algorithm
        10,                 // feed entries
        8,                  // result entries
        test14b_feed,        // feed
        test14b_results,     // results
        NULL,               // feed2
        NULL                // results2
};

struct feed_values test14c_feed[] = {
        { 29000000, 29000000 },
        {  1000000, 29000000 + 1000000 },
        { 30000000, 29000000 + 1000000 + 30000000 },
        { 30000000, 29000000 + 1000000 + 30000000 + 30000000 },
        { 30000000, 29000000 + 1000000 + 30000000 + 30000000 + 30000000 },
        { 30000000, 29000000 + 1000000 + 30000000 + 30000000 + 30000000 + 30000000 },
        { 30000000, 29000000 + 1000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 },
        { 30000000, 29000000 + 1000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 },
        { 30000000, 29000000 + 1000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 },
        { 30000000, 29000000 + 1000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 + 30000000 },
};

calculated_number test14c_results[] = {
        1000000, 1000000, 1000000, 1000000, 1000000, 1000000, 1000000, 1000000, 1000000
};

struct test test14c = {
        "test14c",            // name
        "issue #981 with dummy data, checking for late start",
        30,                 // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_INCREMENTAL, // algorithm
        10,                 // feed entries
        9,                  // result entries
        test14c_feed,        // feed
        test14c_results,     // results
        NULL,               // feed2
        NULL                // results2
};

// --------------------------------------------------------------------------------------------------------------------
// test15

struct feed_values test15_feed[] = {
        {       0, 1068066388 },
        { 1008752, 1068822698 },
        {  993809, 1069573072 },
        {  995911, 1070324135 },
        { 1014562, 1071078166 },
        {  994684, 1071831349 },
        {  993128, 1072235739 },
        { 1010332, 1072958871 },
        { 1003394, 1073707019 },
        {  995201, 1074460255 },
};

collected_number test15_feed2[] = {
    178825286, 178825286, 178825286, 178825286, 178825498, 178825498, 179165652, 179202964, 179203282, 179204130
};

calculated_number test15_results[] = {
        5857.4080000, 5898.4540000, 5891.6590000, 5806.3160000, 5914.2640000, 3202.2630000, 5589.6560000, 5822.5260000, 5911.7520000
};

calculated_number test15_results2[] = {
        0.0000000, 0.0000000, 0.0024944, 1.6324779, 0.0212777, 2655.1890000, 290.5387000, 5.6733610, 6.5960220
};

struct test test15 = {
        "test15",           // name
        "test incremental with 2 dimensions",
        1,                  // update_every
        8,                  // multiplier
        1024,               // divisor
        RRD_ALGORITHM_INCREMENTAL, // algorithm
        10,                 // feed entries
        9,                  // result entries
        test15_feed,        // feed
        test15_results,     // results
        test15_feed2,       // feed2
        test15_results2     // results2
};

// --------------------------------------------------------------------------------------------------------------------

int run_test(struct test *test)
{
    fprintf(stderr, "\nRunning test '%s':\n%s\n", test->name, test->description);

    default_rrd_memory_mode = RRD_MEMORY_MODE_ALLOC;
    default_rrd_update_every = test->update_every;

    char name[101];
    snprintfz(name, 100, "unittest-%s", test->name);

    // create the chart
    RRDSET *st = rrdset_create_localhost("netdata", name, name, "netdata", NULL, "Unit Testing", "a value", "unittest", NULL, 1
                                         , test->update_every, RRDSET_TYPE_LINE);
    RRDDIM *rd = rrddim_add(st, "dim1", NULL, test->multiplier, test->divisor, test->algorithm);
    
    RRDDIM *rd2 = NULL;
    if(test->feed2)
        rd2 = rrddim_add(st, "dim2", NULL, test->multiplier, test->divisor, test->algorithm);

    rrdset_flag_set(st, RRDSET_FLAG_DEBUG);

    // feed it with the test data
    time_t time_now = 0, time_start = now_realtime_sec();
    unsigned long c;
    collected_number last = 0;
    for(c = 0; c < test->feed_entries; c++) {
        if(debug_flags) fprintf(stderr, "\n\n");

        if(c) {
            time_now += test->feed[c].microseconds;
            fprintf(stderr, "    > %s: feeding position %lu, after %0.3f seconds (%0.3f seconds from start), delta " CALCULATED_NUMBER_FORMAT ", rate " CALCULATED_NUMBER_FORMAT "\n", 
                test->name, c+1,
                (float)test->feed[c].microseconds / 1000000.0,
                (float)time_now / 1000000.0,
                ((calculated_number)test->feed[c].value - (calculated_number)last) * (calculated_number)test->multiplier / (calculated_number)test->divisor,
                (((calculated_number)test->feed[c].value - (calculated_number)last) * (calculated_number)test->multiplier / (calculated_number)test->divisor) / (calculated_number)test->feed[c].microseconds * (calculated_number)1000000);

            // rrdset_next_usec_unfiltered(st, test->feed[c].microseconds);
            st->usec_since_last_update = test->feed[c].microseconds;
        }
        else {
            fprintf(stderr, "    > %s: feeding position %lu\n", test->name, c+1);
        }

        fprintf(stderr, "       >> %s with value " COLLECTED_NUMBER_FORMAT "\n", rd->name, test->feed[c].value);
        rrddim_set(st, "dim1", test->feed[c].value);
        last = test->feed[c].value;

        if(rd2) {
            fprintf(stderr, "       >> %s with value " COLLECTED_NUMBER_FORMAT "\n", rd2->name, test->feed2[c]);
            rrddim_set(st, "dim2", test->feed2[c]);
        }

        rrdset_done(st);

        // align the first entry to second boundary
        if(!c) {
            fprintf(stderr, "    > %s: fixing first collection time to be %llu microseconds to second boundary\n", test->name, test->feed[c].microseconds);
            rd->last_collected_time.tv_usec = st->last_collected_time.tv_usec = st->last_updated.tv_usec = test->feed[c].microseconds;
            // time_start = st->last_collected_time.tv_sec;
        }
    }

    // check the result
    int errors = 0;

    if(st->counter != test->result_entries) {
        fprintf(stderr, "    %s stored %zu entries, but we were expecting %lu, ### E R R O R ###\n", test->name, st->counter, test->result_entries);
        errors++;
    }

    unsigned long max = (st->counter < test->result_entries)?st->counter:test->result_entries;
    for(c = 0 ; c < max ; c++) {
        calculated_number v = unpack_storage_number(rd->values[c]);
        calculated_number n = test->results[c];
        int same = (calculated_number_round(v * 10000000.0) == calculated_number_round(n * 10000000.0))?1:0;
        fprintf(stderr, "    %s/%s: checking position %lu (at %lu secs), expecting value " CALCULATED_NUMBER_FORMAT ", found " CALCULATED_NUMBER_FORMAT ", %s\n",
            test->name, rd->name, c+1,
            (rrdset_first_entry_t(st) + c * st->update_every) - time_start,
            n, v, (same)?"OK":"### E R R O R ###");

        if(!same) errors++;

        if(rd2) {
            v = unpack_storage_number(rd2->values[c]);
            n = test->results2[c];
            same = (calculated_number_round(v * 10000000.0) == calculated_number_round(n * 10000000.0))?1:0;
            fprintf(stderr, "    %s/%s: checking position %lu (at %lu secs), expecting value " CALCULATED_NUMBER_FORMAT ", found " CALCULATED_NUMBER_FORMAT ", %s\n",
                test->name, rd2->name, c+1,
                (rrdset_first_entry_t(st) + c * st->update_every) - time_start,
                n, v, (same)?"OK":"### E R R O R ###");
            if(!same) errors++;
        }
    }

    return errors;
}

static int test_variable_renames(void) {
    fprintf(stderr, "Creating chart\n");
    RRDSET *st = rrdset_create_localhost("chart", "ID", NULL, "family", "context", "Unit Testing", "a value", "unittest", NULL, 1, 1, RRDSET_TYPE_LINE);
    fprintf(stderr, "Created chart with id '%s', name '%s'\n", st->id, st->name);

    fprintf(stderr, "Creating dimension DIM1\n");
    RRDDIM *rd1 = rrddim_add(st, "DIM1", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    fprintf(stderr, "Created dimension with id '%s', name '%s'\n", rd1->id, rd1->name);

    fprintf(stderr, "Creating dimension DIM2\n");
    RRDDIM *rd2 = rrddim_add(st, "DIM2", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    fprintf(stderr, "Created dimension with id '%s', name '%s'\n", rd2->id, rd2->name);

    fprintf(stderr, "Renaming chart to CHARTNAME1\n");
    rrdset_set_name(st, "CHARTNAME1");
    fprintf(stderr, "Renamed chart with id '%s' to name '%s'\n", st->id, st->name);

    fprintf(stderr, "Renaming chart to CHARTNAME2\n");
    rrdset_set_name(st, "CHARTNAME2");
    fprintf(stderr, "Renamed chart with id '%s' to name '%s'\n", st->id, st->name);

    fprintf(stderr, "Renaming dimension DIM1 to DIM1NAME1\n");
    rrddim_set_name(st, rd1, "DIM1NAME1");
    fprintf(stderr, "Renamed dimension with id '%s' to name '%s'\n", rd1->id, rd1->name);

    fprintf(stderr, "Renaming dimension DIM1 to DIM1NAME2\n");
    rrddim_set_name(st, rd1, "DIM1NAME2");
    fprintf(stderr, "Renamed dimension with id '%s' to name '%s'\n", rd1->id, rd1->name);

    fprintf(stderr, "Renaming dimension DIM2 to DIM2NAME1\n");
    rrddim_set_name(st, rd2, "DIM2NAME1");
    fprintf(stderr, "Renamed dimension with id '%s' to name '%s'\n", rd2->id, rd2->name);

    fprintf(stderr, "Renaming dimension DIM2 to DIM2NAME2\n");
    rrddim_set_name(st, rd2, "DIM2NAME2");
    fprintf(stderr, "Renamed dimension with id '%s' to name '%s'\n", rd2->id, rd2->name);

    BUFFER *buf = buffer_create(1);
    health_api_v1_chart_variables2json(st, buf);
    fprintf(stderr, "%s", buffer_tostring(buf));
    buffer_free(buf);
    return 1;
}

int run_all_mockup_tests(void)
{
    if(check_number_printing())
        return 1;

    if(check_rrdcalc_comparisons())
        return 1;

    if(!test_variable_renames())
        return 1;

    if(run_test(&test1))
        return 1;

    if(run_test(&test2))
        return 1;

    if(run_test(&test3))
        return 1;

    if(run_test(&test4))
        return 1;

    if(run_test(&test5))
        return 1;

    if(run_test(&test6))
        return 1;

    if(run_test(&test7))
        return 1;

    if(run_test(&test8))
        return 1;

    if(run_test(&test9))
        return 1;

    if(run_test(&test10))
        return 1;

    if(run_test(&test11))
        return 1;

    if(run_test(&test12))
        return 1;

    if(run_test(&test13))
        return 1;

    if(run_test(&test14))
        return 1;

    if(run_test(&test14b))
        return 1;

    if(run_test(&test14c))
        return 1;

    if(run_test(&test15))
        return 1;



    return 0;
}

int unit_test(long delay, long shift)
{
    static int repeat = 0;
    repeat++;

    char name[101];
    snprintfz(name, 100, "unittest-%d-%ld-%ld", repeat, delay, shift);

    //debug_flags = 0xffffffff;
    default_rrd_memory_mode = RRD_MEMORY_MODE_ALLOC;
    default_rrd_update_every = 1;

    int do_abs = 1;
    int do_inc = 1;
    int do_abst = 0;
    int do_absi = 0;

    RRDSET *st = rrdset_create_localhost("netdata", name, name, "netdata", NULL, "Unit Testing", "a value", "unittest", NULL, 1, 1
                                         , RRDSET_TYPE_LINE);
    rrdset_flag_set(st, RRDSET_FLAG_DEBUG);

    RRDDIM *rdabs = NULL;
    RRDDIM *rdinc = NULL;
    RRDDIM *rdabst = NULL;
    RRDDIM *rdabsi = NULL;

    if(do_abs) rdabs = rrddim_add(st, "absolute", "absolute", 1, 1, RRD_ALGORITHM_ABSOLUTE);
    if(do_inc) rdinc = rrddim_add(st, "incremental", "incremental", 1, 1, RRD_ALGORITHM_INCREMENTAL);
    if(do_abst) rdabst = rrddim_add(st, "percentage-of-absolute-row", "percentage-of-absolute-row", 1, 1, RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL);
    if(do_absi) rdabsi = rrddim_add(st, "percentage-of-incremental-row", "percentage-of-incremental-row", 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);

    long increment = 1000;
    collected_number i = 0;

    unsigned long c, dimensions = 0;
    RRDDIM *rd;
    for(rd = st->dimensions ; rd ; rd = rd->next) dimensions++;

    for(c = 0; c < 20 ;c++) {
        i += increment;

        fprintf(stderr, "\n\nLOOP = %lu, DELAY = %ld, VALUE = " COLLECTED_NUMBER_FORMAT "\n", c, delay, i);
        if(c) {
            // rrdset_next_usec_unfiltered(st, delay);
            st->usec_since_last_update = delay;
        }
        if(do_abs) rrddim_set(st, "absolute", i);
        if(do_inc) rrddim_set(st, "incremental", i);
        if(do_abst) rrddim_set(st, "percentage-of-absolute-row", i);
        if(do_absi) rrddim_set(st, "percentage-of-incremental-row", i);

        if(!c) {
            now_realtime_timeval(&st->last_collected_time);
            st->last_collected_time.tv_usec = shift;
        }

        // prevent it from deleting the dimensions
        for(rd = st->dimensions ; rd ; rd = rd->next)
            rd->last_collected_time.tv_sec = st->last_collected_time.tv_sec;

        rrdset_done(st);
    }

    unsigned long oincrement = increment;
    increment = increment * st->update_every * 1000000 / delay;
    fprintf(stderr, "\n\nORIGINAL INCREMENT: %lu, INCREMENT %ld, DELAY %ld, SHIFT %ld\n", oincrement * 10, increment * 10, delay, shift);

    int ret = 0;
    storage_number sn;
    calculated_number cn, v;
    for(c = 0 ; c < st->counter ; c++) {
        fprintf(stderr, "\nPOSITION: c = %lu, EXPECTED VALUE %lu\n", c, (oincrement + c * increment + increment * (1000000 - shift) / 1000000 )* 10);

        for(rd = st->dimensions ; rd ; rd = rd->next) {
            sn = rd->values[c];
            cn = unpack_storage_number(sn);
            fprintf(stderr, "\t %s " CALCULATED_NUMBER_FORMAT " (PACKED AS " STORAGE_NUMBER_FORMAT ")   ->   ", rd->id, cn, sn);

            if(rd == rdabs) v =
                (     oincrement
                    // + (increment * (1000000 - shift) / 1000000)
                    + (c + 1) * increment
                );

            else if(rd == rdinc) v = (c?(increment):(increment * (1000000 - shift) / 1000000));
            else if(rd == rdabst) v = oincrement / dimensions / 10;
            else if(rd == rdabsi) v = oincrement / dimensions / 10;
            else v = 0;

            if(v == cn) fprintf(stderr, "passed.\n");
            else {
                fprintf(stderr, "ERROR! (expected " CALCULATED_NUMBER_FORMAT ")\n", v);
                ret = 1;
            }
        }
    }

    if(ret)
        fprintf(stderr, "\n\nUNIT TEST(%ld, %ld) FAILED\n\n", delay, shift);

    return ret;
}
