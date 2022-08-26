// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

static int check_number_printing(void) {
    struct {
        NETDATA_DOUBLE n;
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
        print_netdata_double(netdata, values[i].n);
        snprintfz(system, 49, "%0.12" NETDATA_DOUBLE_MODIFIER, (NETDATA_DOUBLE)values[i].n);

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

int check_storage_number(NETDATA_DOUBLE n, int debug) {
    char buffer[100];
    uint32_t flags = SN_DEFAULT_FLAGS;

    storage_number s = pack_storage_number(n, flags);
    NETDATA_DOUBLE d = unpack_storage_number(s);

    if(!does_storage_number_exist(s)) {
        fprintf(stderr, "Exists flags missing for number " NETDATA_DOUBLE_FORMAT "!\n", n);
        return 5;
    }

    NETDATA_DOUBLE ddiff = d - n;
    NETDATA_DOUBLE dcdiff = ddiff * 100.0 / n;

    if(dcdiff < 0) dcdiff = -dcdiff;

    size_t len = (size_t)print_netdata_double(buffer, d);
    NETDATA_DOUBLE p = str2ndd(buffer, NULL);
    NETDATA_DOUBLE pdiff = n - p;
    NETDATA_DOUBLE pcdiff = pdiff * 100.0 / n;
    if(pcdiff < 0) pcdiff = -pcdiff;

    if(debug) {
        fprintf(stderr,
            NETDATA_DOUBLE_FORMAT
            " original\n" NETDATA_DOUBLE_FORMAT " packed and unpacked, (stored as 0x%08X, diff " NETDATA_DOUBLE_FORMAT
            ", " NETDATA_DOUBLE_FORMAT "%%)\n"
            "%s printed after unpacked (%zu bytes)\n" NETDATA_DOUBLE_FORMAT
            " re-parsed from printed (diff " NETDATA_DOUBLE_FORMAT ", " NETDATA_DOUBLE_FORMAT "%%)\n\n",
            n,
            d, s, ddiff, dcdiff,
            buffer, len,
            p, pdiff, pcdiff
        );
        if(len != strlen(buffer)) fprintf(stderr, "ERROR: printed number %s is reported to have length %zu but it has %zu\n", buffer, len, strlen(buffer));

        if(dcdiff > ACCURACY_LOSS_ACCEPTED_PERCENT)
            fprintf(stderr, "WARNING: packing number " NETDATA_DOUBLE_FORMAT " has accuracy loss " NETDATA_DOUBLE_FORMAT " %%\n", n, dcdiff);

        if(pcdiff > ACCURACY_LOSS_ACCEPTED_PERCENT)
            fprintf(stderr, "WARNING: re-parsing the packed, unpacked and printed number " NETDATA_DOUBLE_FORMAT
                " has accuracy loss " NETDATA_DOUBLE_FORMAT " %%\n", n, pcdiff);
    }

    if(len != strlen(buffer)) return 1;
    if(dcdiff > ACCURACY_LOSS_ACCEPTED_PERCENT) return 3;
    if(pcdiff > ACCURACY_LOSS_ACCEPTED_PERCENT) return 4;
    return 0;
}

NETDATA_DOUBLE storage_number_min(NETDATA_DOUBLE n) {
    NETDATA_DOUBLE r = 1, last;

    do {
        last = n;
        n /= 2.0;
        storage_number t = pack_storage_number(n, SN_DEFAULT_FLAGS);
        r = unpack_storage_number(t);
    } while(r != 0.0 && r != last);

    return last;
}

void benchmark_storage_number(int loop, int multiplier) {
    int i, j;
    NETDATA_DOUBLE n, d;
    storage_number s;
    unsigned long long user, system, total, mine, their;

    NETDATA_DOUBLE storage_number_positive_min = unpack_storage_number(STORAGE_NUMBER_POSITIVE_MIN_RAW);
    NETDATA_DOUBLE storage_number_positive_max = unpack_storage_number(STORAGE_NUMBER_POSITIVE_MAX_RAW);

    char buffer[100];

    struct rusage now, last;

    fprintf(stderr, "\n\nBenchmarking %d numbers, please wait...\n\n", loop);

    // ------------------------------------------------------------------------

    fprintf(stderr, "SYSTEM  LONG DOUBLE    SIZE: %zu bytes\n", sizeof(NETDATA_DOUBLE));
    fprintf(stderr, "NETDATA FLOATING POINT SIZE: %zu bytes\n", sizeof(storage_number));

    mine = (NETDATA_DOUBLE)sizeof(storage_number) * (NETDATA_DOUBLE)loop;
    their = (NETDATA_DOUBLE)sizeof(NETDATA_DOUBLE) * (NETDATA_DOUBLE)loop;

    if(mine > their) {
        fprintf(stderr, "\nNETDATA NEEDS %0.2" NETDATA_DOUBLE_MODIFIER " TIMES MORE MEMORY. Sorry!\n", (NETDATA_DOUBLE)(mine / their));
    }
    else {
        fprintf(stderr, "\nNETDATA INTERNAL FLOATING POINT ARITHMETICS NEEDS %0.2" NETDATA_DOUBLE_MODIFIER " TIMES LESS MEMORY.\n", (NETDATA_DOUBLE)(their / mine));
    }

    fprintf(stderr, "\nNETDATA FLOATING POINT\n");
    fprintf(stderr, "MIN POSITIVE VALUE " NETDATA_DOUBLE_FORMAT "\n", unpack_storage_number(STORAGE_NUMBER_POSITIVE_MIN_RAW));
    fprintf(stderr, "MAX POSITIVE VALUE " NETDATA_DOUBLE_FORMAT "\n", unpack_storage_number(STORAGE_NUMBER_POSITIVE_MAX_RAW));
    fprintf(stderr, "MIN NEGATIVE VALUE " NETDATA_DOUBLE_FORMAT "\n", unpack_storage_number(STORAGE_NUMBER_NEGATIVE_MIN_RAW));
    fprintf(stderr, "MAX NEGATIVE VALUE " NETDATA_DOUBLE_FORMAT "\n", unpack_storage_number(STORAGE_NUMBER_NEGATIVE_MAX_RAW));
    fprintf(stderr, "Maximum accuracy loss accepted: " NETDATA_DOUBLE_FORMAT "%%\n\n\n", (NETDATA_DOUBLE)ACCURACY_LOSS_ACCEPTED_PERCENT);

    // ------------------------------------------------------------------------

    fprintf(stderr, "INTERNAL LONG DOUBLE PRINTING: ");
    getrusage(RUSAGE_SELF, &last);

    // do the job
    for(j = 1; j < 11 ;j++) {
        n = storage_number_positive_min * j;

        for(i = 0; i < loop ;i++) {
            n *= multiplier;
            if(n > storage_number_positive_max) n = storage_number_positive_min;

            print_netdata_double(buffer, n);
        }
    }

    getrusage(RUSAGE_SELF, &now);
    user   = now.ru_utime.tv_sec * 1000000ULL + now.ru_utime.tv_usec - last.ru_utime.tv_sec * 1000000ULL + last.ru_utime.tv_usec;
    system = now.ru_stime.tv_sec * 1000000ULL + now.ru_stime.tv_usec - last.ru_stime.tv_sec * 1000000ULL + last.ru_stime.tv_usec;
    total  = user + system;
    mine = total;

    fprintf(stderr, "user %0.5" NETDATA_DOUBLE_MODIFIER ", system %0.5" NETDATA_DOUBLE_MODIFIER
        ", total %0.5" NETDATA_DOUBLE_MODIFIER "\n", (NETDATA_DOUBLE)(user / 1000000.0), (NETDATA_DOUBLE)(system / 1000000.0), (NETDATA_DOUBLE)(total / 1000000.0));

    // ------------------------------------------------------------------------

    fprintf(stderr, "SYSTEM   LONG DOUBLE PRINTING: ");
    getrusage(RUSAGE_SELF, &last);

    // do the job
    for(j = 1; j < 11 ;j++) {
        n = storage_number_positive_min * j;

        for(i = 0; i < loop ;i++) {
            n *= multiplier;
            if(n > storage_number_positive_max) n = storage_number_positive_min;
            snprintfz(buffer, 100, NETDATA_DOUBLE_FORMAT, n);
        }
    }

    getrusage(RUSAGE_SELF, &now);
    user   = now.ru_utime.tv_sec * 1000000ULL + now.ru_utime.tv_usec - last.ru_utime.tv_sec * 1000000ULL + last.ru_utime.tv_usec;
    system = now.ru_stime.tv_sec * 1000000ULL + now.ru_stime.tv_usec - last.ru_stime.tv_sec * 1000000ULL + last.ru_stime.tv_usec;
    total  = user + system;
    their = total;

    fprintf(stderr, "user %0.5" NETDATA_DOUBLE_MODIFIER ", system %0.5" NETDATA_DOUBLE_MODIFIER
        ", total %0.5" NETDATA_DOUBLE_MODIFIER "\n", (NETDATA_DOUBLE)(user / 1000000.0), (NETDATA_DOUBLE)(system / 1000000.0), (NETDATA_DOUBLE)(total / 1000000.0));

    if(mine > total) {
        fprintf(stderr, "NETDATA CODE IS SLOWER %0.2" NETDATA_DOUBLE_MODIFIER " %%\n", (NETDATA_DOUBLE)(mine * 100.0 / their - 100.0));
    }
    else {
        fprintf(stderr, "NETDATA CODE IS  F A S T E R  %0.2" NETDATA_DOUBLE_MODIFIER " %%\n", (NETDATA_DOUBLE)(their * 100.0 / mine - 100.0));
    }

    // ------------------------------------------------------------------------

    fprintf(stderr, "\nINTERNAL LONG DOUBLE PRINTING WITH PACK / UNPACK: ");
    getrusage(RUSAGE_SELF, &last);

    // do the job
    for(j = 1; j < 11 ;j++) {
        n = storage_number_positive_min * j;

        for(i = 0; i < loop ;i++) {
            n *= multiplier;
            if(n > storage_number_positive_max) n = storage_number_positive_min;

            s = pack_storage_number(n, SN_DEFAULT_FLAGS);
            d = unpack_storage_number(s);
            print_netdata_double(buffer, d);
        }
    }

    getrusage(RUSAGE_SELF, &now);
    user   = now.ru_utime.tv_sec * 1000000ULL + now.ru_utime.tv_usec - last.ru_utime.tv_sec * 1000000ULL + last.ru_utime.tv_usec;
    system = now.ru_stime.tv_sec * 1000000ULL + now.ru_stime.tv_usec - last.ru_stime.tv_sec * 1000000ULL + last.ru_stime.tv_usec;
    total  = user + system;
    mine = total;

    fprintf(stderr, "user %0.5" NETDATA_DOUBLE_MODIFIER ", system %0.5" NETDATA_DOUBLE_MODIFIER
        ", total %0.5" NETDATA_DOUBLE_MODIFIER "\n", (NETDATA_DOUBLE)(user / 1000000.0), (NETDATA_DOUBLE)(system / 1000000.0), (NETDATA_DOUBLE)(total / 1000000.0));

    if(mine > their) {
        fprintf(stderr, "WITH PACKING UNPACKING NETDATA CODE IS SLOWER %0.2" NETDATA_DOUBLE_MODIFIER " %%\n", (NETDATA_DOUBLE)(mine * 100.0 / their - 100.0));
    }
    else {
        fprintf(stderr, "EVEN WITH PACKING AND UNPACKING, NETDATA CODE IS  F A S T E R  %0.2" NETDATA_DOUBLE_MODIFIER " %%\n", (NETDATA_DOUBLE)(their * 100.0 / mine - 100.0));
    }

    // ------------------------------------------------------------------------

}

static int check_storage_number_exists() {
    uint32_t flags = SN_DEFAULT_FLAGS;
    NETDATA_DOUBLE n = 0.0;

    storage_number s = pack_storage_number(n, flags);
    NETDATA_DOUBLE d = unpack_storage_number(s);

    if(n != d) {
        fprintf(stderr, "Wrong number returned. Expected " NETDATA_DOUBLE_FORMAT ", returned " NETDATA_DOUBLE_FORMAT "!\n", n, d);
        return 1;
    }

    return 0;
}

int unit_test_storage() {
    if(check_storage_number_exists()) return 0;

    NETDATA_DOUBLE storage_number_positive_min = unpack_storage_number(STORAGE_NUMBER_POSITIVE_MIN_RAW);
    NETDATA_DOUBLE storage_number_negative_max = unpack_storage_number(STORAGE_NUMBER_NEGATIVE_MAX_RAW);

    NETDATA_DOUBLE c, a = 0;
    int i, j, g, r = 0;

    for(g = -1; g <= 1 ; g++) {
        a = 0;

        if(!g) continue;

        for(j = 0; j < 9 ;j++) {
            a += 0.0000001;
            c = a * g;
            for(i = 0; i < 21 ;i++, c *= 10) {
                if(c > 0 && c < storage_number_positive_min) continue;
                if(c < 0 && c > storage_number_negative_max) continue;

                if(check_storage_number(c, 1)) return 1;
            }
        }
    }

    // if(check_storage_number(858993459.1234567, 1)) return 1;
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
        NETDATA_DOUBLE mine = str2ndd(values[i], &e_mine);
        NETDATA_DOUBLE sys = strtondd(values[i], &e_sys);

        if(isnan(mine)) {
            if(!isnan(sys)) {
                fprintf(stderr, "Value '%s' is parsed as %" NETDATA_DOUBLE_MODIFIER
                    ", but system believes it is %" NETDATA_DOUBLE_MODIFIER ".\n", values[i], mine, sys);
                return -1;
            }
        }
        else if(isinf(mine)) {
            if(!isinf(sys)) {
                fprintf(stderr, "Value '%s' is parsed as %" NETDATA_DOUBLE_MODIFIER
                    ", but system believes it is %" NETDATA_DOUBLE_MODIFIER ".\n", values[i], mine, sys);
                return -1;
            }
        }
        else if(mine != sys && ABS(mine-sys) > 0.000001) {
            fprintf(stderr, "Value '%s' is parsed as %" NETDATA_DOUBLE_MODIFIER
                ", but system believes it is %" NETDATA_DOUBLE_MODIFIER ", delta %" NETDATA_DOUBLE_MODIFIER ".\n", values[i], mine, sys, sys-mine);
            return -1;
        }

        if(e_mine != e_sys) {
            fprintf(stderr, "Value '%s' is parsed correctly, but endptr is not right\n", values[i]);
            return -1;
        }

        fprintf(stderr, "str2ndd() parsed value '%s' exactly the same way with strtold(), returned %" NETDATA_DOUBLE_MODIFIER
            " vs %" NETDATA_DOUBLE_MODIFIER "\n", values[i], mine, sys);
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

int unit_test_static_threads() {
    struct netdata_static_thread *static_threads = static_threads_get();

    /*
     * make sure enough static threads have been registered
     */
    if (!static_threads) {
        fprintf(stderr, "empty static_threads array\n");
        return 1;
    }

    int n;
    for (n = 0; static_threads[n].start_routine != NULL; n++) {}

    if (n < 2) {
        fprintf(stderr, "only %d static threads registered", n);
        freez(static_threads);
        return 1;
    }

    /*
     * verify that each thread's start routine is unique.
     */
    for (int i = 0; i != n - 1; i++) {
        for (int j = i + 1; j != n; j++) {
            if (static_threads[i].start_routine != static_threads[j].start_routine)
                continue;

            fprintf(stderr, "Found duplicate threads with name: %s\n", static_threads[i].name);
            freez(static_threads);
            return 1;
        }
    }

    freez(static_threads);
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
    NETDATA_DOUBLE *results;

    collected_number *feed2;
    NETDATA_DOUBLE *results2;
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

NETDATA_DOUBLE test1_results[] = {
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

NETDATA_DOUBLE test2_results[] = {
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

NETDATA_DOUBLE test3_results[] = {
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

NETDATA_DOUBLE test4_results[] = {
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
// test5 - 32 bit overflows

struct feed_values test5_feed[] = {
        { 0,       0x00000000FFFFFFFFULL / 15 * 0 },
        { 1000000, 0x00000000FFFFFFFFULL / 15 * 7 },
        { 1000000, 0x00000000FFFFFFFFULL / 15 * 14 },
        { 1000000, 0x00000000FFFFFFFFULL / 15 * 0 },
        { 1000000, 0x00000000FFFFFFFFULL / 15 * 7 },
        { 1000000, 0x00000000FFFFFFFFULL / 15 * 14 },
        { 1000000, 0x00000000FFFFFFFFULL / 15 * 0 },
        { 1000000, 0x00000000FFFFFFFFULL / 15 * 7 },
        { 1000000, 0x00000000FFFFFFFFULL / 15 * 14 },
        { 1000000, 0x00000000FFFFFFFFULL / 15 * 0 },
};

NETDATA_DOUBLE test5_results[] = {
        0x00000000FFFFFFFFULL / 15 * 7,
        0x00000000FFFFFFFFULL / 15 * 7,
        0x00000000FFFFFFFFULL / 15,
        0x00000000FFFFFFFFULL / 15 * 7,
        0x00000000FFFFFFFFULL / 15 * 7,
        0x00000000FFFFFFFFULL / 15,
        0x00000000FFFFFFFFULL / 15 * 7,
        0x00000000FFFFFFFFULL / 15 * 7,
        0x00000000FFFFFFFFULL / 15,
};

struct test test5 = {
        "test5",            // name
        "test 32-bit incremental values overflow",
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
// test5b - 64 bit overflows

struct feed_values test5b_feed[] = {
        { 0,       0xFFFFFFFFFFFFFFFFULL / 15 * 0 },
        { 1000000, 0xFFFFFFFFFFFFFFFFULL / 15 * 7 },
        { 1000000, 0xFFFFFFFFFFFFFFFFULL / 15 * 14 },
        { 1000000, 0xFFFFFFFFFFFFFFFFULL / 15 * 0 },
        { 1000000, 0xFFFFFFFFFFFFFFFFULL / 15 * 7 },
        { 1000000, 0xFFFFFFFFFFFFFFFFULL / 15 * 14 },
        { 1000000, 0xFFFFFFFFFFFFFFFFULL / 15 * 0 },
        { 1000000, 0xFFFFFFFFFFFFFFFFULL / 15 * 7 },
        { 1000000, 0xFFFFFFFFFFFFFFFFULL / 15 * 14 },
        { 1000000, 0xFFFFFFFFFFFFFFFFULL / 15 * 0 },
};

NETDATA_DOUBLE test5b_results[] = {
        0xFFFFFFFFFFFFFFFFULL / 15 * 7,
        0xFFFFFFFFFFFFFFFFULL / 15 * 7,
        0xFFFFFFFFFFFFFFFFULL / 15,
        0xFFFFFFFFFFFFFFFFULL / 15 * 7,
        0xFFFFFFFFFFFFFFFFULL / 15 * 7,
        0xFFFFFFFFFFFFFFFFULL / 15,
        0xFFFFFFFFFFFFFFFFULL / 15 * 7,
        0xFFFFFFFFFFFFFFFFULL / 15 * 7,
        0xFFFFFFFFFFFFFFFFULL / 15,
};

struct test test5b = {
        "test5b",            // name
        "test 64-bit incremental values overflow",
        1,                  // update_every
        1,                  // multiplier
        1,                  // divisor
        RRD_ALGORITHM_INCREMENTAL, // algorithm
        10,                 // feed entries
        9,                  // result entries
        test5b_feed,        // feed
        test5b_results,     // results
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

NETDATA_DOUBLE test6_results[] = {
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

NETDATA_DOUBLE test7_results[] = {
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

NETDATA_DOUBLE test8_results[] = {
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

NETDATA_DOUBLE test9_results[] = {
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

NETDATA_DOUBLE test10_results[] = {
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

NETDATA_DOUBLE test11_results[] = {
        50, 50, 50, 50, 50, 50, 50, 50, 50
};

NETDATA_DOUBLE test11_results2[] = {
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

NETDATA_DOUBLE test12_results[] = {
        25, 25, 25, 25, 25, 25, 25, 25, 25
};

NETDATA_DOUBLE test12_results2[] = {
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

NETDATA_DOUBLE test13_results[] = {
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

NETDATA_DOUBLE test14_results[] = {
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

NETDATA_DOUBLE test14b_results[] = {
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

NETDATA_DOUBLE test14c_results[] = {
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

NETDATA_DOUBLE test15_results[] = {
        5857.4080000, 5898.4540000, 5891.6590000, 5806.3160000, 5914.2640000, 3202.2630000, 5589.6560000, 5822.5260000, 5911.7520000
};

NETDATA_DOUBLE test15_results2[] = {
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
            fprintf(stderr, "    > %s: feeding position %lu, after %0.3f seconds (%0.3f seconds from start), delta " NETDATA_DOUBLE_FORMAT
                ", rate " NETDATA_DOUBLE_FORMAT "\n",
                test->name, c+1,
                (float)test->feed[c].microseconds / 1000000.0,
                (float)time_now / 1000000.0,
                ((NETDATA_DOUBLE)test->feed[c].value - (NETDATA_DOUBLE)last) * (NETDATA_DOUBLE)test->multiplier / (NETDATA_DOUBLE)test->divisor,
                (((NETDATA_DOUBLE)test->feed[c].value - (NETDATA_DOUBLE)last) * (NETDATA_DOUBLE)test->multiplier / (NETDATA_DOUBLE)test->divisor) / (NETDATA_DOUBLE)test->feed[c].microseconds * (NETDATA_DOUBLE)1000000);

            // rrdset_next_usec_unfiltered(st, test->feed[c].microseconds);
            st->usec_since_last_update = test->feed[c].microseconds;
        }
        else {
            fprintf(stderr, "    > %s: feeding position %lu\n", test->name, c+1);
        }

        fprintf(stderr, "       >> %s with value " COLLECTED_NUMBER_FORMAT "\n", rrddim_name(rd), test->feed[c].value);
        rrddim_set(st, "dim1", test->feed[c].value);
        last = test->feed[c].value;

        if(rd2) {
            fprintf(stderr, "       >> %s with value " COLLECTED_NUMBER_FORMAT "\n", rrddim_name(rd2), test->feed2[c]);
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
        NETDATA_DOUBLE v = unpack_storage_number(rd->db[c]);
        NETDATA_DOUBLE n = unpack_storage_number(pack_storage_number(test->results[c], SN_DEFAULT_FLAGS));
        int same = (roundndd(v * 10000000.0) == roundndd(n * 10000000.0))?1:0;
        fprintf(stderr, "    %s/%s: checking position %lu (at %"PRId64" secs), expecting value " NETDATA_DOUBLE_FORMAT
            ", found " NETDATA_DOUBLE_FORMAT ", %s\n",
            test->name, rrddim_name(rd), c+1,
            (int64_t)((rrdset_first_entry_t(st) + c * st->update_every) - time_start),
            n, v, (same)?"OK":"### E R R O R ###");

        if(!same) errors++;

        if(rd2) {
            v = unpack_storage_number(rd2->db[c]);
            n = test->results2[c];
            same = (roundndd(v * 10000000.0) == roundndd(n * 10000000.0))?1:0;
            fprintf(stderr, "    %s/%s: checking position %lu (at %"PRId64" secs), expecting value " NETDATA_DOUBLE_FORMAT
                ", found " NETDATA_DOUBLE_FORMAT ", %s\n",
                test->name, rrddim_name(rd2), c+1,
                (int64_t)((rrdset_first_entry_t(st) + c * st->update_every) - time_start),
                n, v, (same)?"OK":"### E R R O R ###");
            if(!same) errors++;
        }
    }

    return errors;
}

static int test_variable_renames(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );

    fprintf(stderr, "Creating chart\n");
    RRDSET *st = rrdset_create_localhost("chart", "ID", NULL, "family", "context", "Unit Testing", "a value", "unittest", NULL, 1, 1, RRDSET_TYPE_LINE);
    fprintf(stderr, "Created chart with id '%s', name '%s'\n", rrdset_id(st), rrdset_name(st));

    fprintf(stderr, "Creating dimension DIM1\n");
    RRDDIM *rd1 = rrddim_add(st, "DIM1", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    fprintf(stderr, "Created dimension with id '%s', name '%s'\n", rrddim_id(rd1), rrddim_name(rd1));

    fprintf(stderr, "Creating dimension DIM2\n");
    RRDDIM *rd2 = rrddim_add(st, "DIM2", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    fprintf(stderr, "Created dimension with id '%s', name '%s'\n", rrddim_id(rd2), rrddim_name(rd2));

    fprintf(stderr, "Renaming chart to CHARTNAME1\n");
    rrdset_set_name(st, "CHARTNAME1");
    fprintf(stderr, "Renamed chart with id '%s' to name '%s'\n", rrdset_id(st), rrdset_name(st));

    fprintf(stderr, "Renaming chart to CHARTNAME2\n");
    rrdset_set_name(st, "CHARTNAME2");
    fprintf(stderr, "Renamed chart with id '%s' to name '%s'\n", rrdset_id(st), rrdset_name(st));

    fprintf(stderr, "Renaming dimension DIM1 to DIM1NAME1\n");
    rrddim_set_name(st, rd1, "DIM1NAME1");
    fprintf(stderr, "Renamed dimension with id '%s' to name '%s'\n", rrddim_id(rd1), rrddim_name(rd1));

    fprintf(stderr, "Renaming dimension DIM1 to DIM1NAME2\n");
    rrddim_set_name(st, rd1, "DIM1NAME2");
    fprintf(stderr, "Renamed dimension with id '%s' to name '%s'\n", rrddim_id(rd1), rrddim_name(rd1));

    fprintf(stderr, "Renaming dimension DIM2 to DIM2NAME1\n");
    rrddim_set_name(st, rd2, "DIM2NAME1");
    fprintf(stderr, "Renamed dimension with id '%s' to name '%s'\n", rrddim_id(rd2), rrddim_name(rd2));

    fprintf(stderr, "Renaming dimension DIM2 to DIM2NAME2\n");
    rrddim_set_name(st, rd2, "DIM2NAME2");
    fprintf(stderr, "Renamed dimension with id '%s' to name '%s'\n", rrddim_id(rd2), rrddim_name(rd2));

    BUFFER *buf = buffer_create(1);
    health_api_v1_chart_variables2json(st, buf);
    fprintf(stderr, "%s", buffer_tostring(buf));
    buffer_free(buf);
    return 1;
}

int check_strdupz_path_subpath() {

    struct strdupz_path_subpath_checks {
        const char *path;
        const char *subpath;
        const char *result;
    } checks[] = {
            { "",                "",            "."                     },
            { "/",               "",            "/"                     },
            { "/etc/netdata",    "",            "/etc/netdata"          },
            { "/etc/netdata///", "",            "/etc/netdata"          },
            { "/etc/netdata///", "health.d",    "/etc/netdata/health.d" },
            { "/etc/netdata///", "///health.d", "/etc/netdata/health.d" },
            { "/etc/netdata",    "///health.d", "/etc/netdata/health.d" },
            { "",                "///health.d", "./health.d"            },
            { "/",               "///health.d", "/health.d"             },

            // terminator
            { NULL, NULL, NULL }
    };

    size_t i;
    for(i = 0; checks[i].result ; i++) {
        char *s = strdupz_path_subpath(checks[i].path, checks[i].subpath);
        fprintf(stderr, "strdupz_path_subpath(\"%s\", \"%s\") = \"%s\": ", checks[i].path, checks[i].subpath, s);
        if(!s || strcmp(s, checks[i].result) != 0) {
            freez(s);
            fprintf(stderr, "FAILED\n");
            return 1;
        }
        else {
            freez(s);
            fprintf(stderr, "OK\n");
        }
    }

    return 0;
}

int run_all_mockup_tests(void)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    if(check_strdupz_path_subpath())
        return 1;

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

    if(run_test(&test5b))
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
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
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
    NETDATA_DOUBLE cn, v;
    for(c = 0 ; c < st->counter ; c++) {
        fprintf(stderr, "\nPOSITION: c = %lu, EXPECTED VALUE %lu\n", c, (oincrement + c * increment + increment * (1000000 - shift) / 1000000 )* 10);

        for(rd = st->dimensions ; rd ; rd = rd->next) {
            sn = rd->db[c];
            cn = unpack_storage_number(sn);
            fprintf(stderr, "\t %s " NETDATA_DOUBLE_FORMAT " (PACKED AS " STORAGE_NUMBER_FORMAT ")   ->   ", rrddim_id(rd), cn, sn);

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
                fprintf(stderr, "ERROR! (expected " NETDATA_DOUBLE_FORMAT ")\n", v);
                ret = 1;
            }
        }
    }

    if(ret)
        fprintf(stderr, "\n\nUNIT TEST(%ld, %ld) FAILED\n\n", delay, shift);

    return ret;
}

int test_sqlite(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    sqlite3  *db_meta;
    fprintf(stderr, "Testing SQLIte\n");

    int rc = sqlite3_open(":memory:", &db_meta);
    if (rc != SQLITE_OK) {
        fprintf(stderr,"Failed to test SQLite: DB init failed\n");
        return 1;
    }

    rc = sqlite3_exec_monitored(db_meta, "CREATE TABLE IF NOT EXISTS mine (id1, id2);", 0, 0, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr,"Failed to test SQLite: Create table failed\n");
        return 1;
    }

    rc = sqlite3_exec_monitored(db_meta, "DELETE FROM MINE LIMIT 1;", 0, 0, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr,"Failed to test SQLite: Delete with LIMIT failed\n");
        return 1;
    }

    rc = sqlite3_exec_monitored(db_meta, "UPDATE MINE SET id1=1 LIMIT 1;", 0, 0, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr,"Failed to test SQLite: Update with LIMIT failed\n");
        return 1;
    }

    BUFFER *sql = buffer_create(ACLK_SYNC_QUERY_SIZE);
    char *uuid_str = "0000_000";

    buffer_sprintf(sql, TABLE_ACLK_CHART, uuid_str);
    rc = sqlite3_exec_monitored(db_meta, buffer_tostring(sql), 0, 0, NULL);
    buffer_flush(sql);
    if (rc != SQLITE_OK)
        goto error;

    buffer_sprintf(sql, TABLE_ACLK_CHART_PAYLOAD, uuid_str);
    rc = sqlite3_exec_monitored(db_meta, buffer_tostring(sql), 0, 0, NULL);
    buffer_flush(sql);
    if (rc != SQLITE_OK)
        goto error;

    buffer_sprintf(sql, TABLE_ACLK_CHART_LATEST, uuid_str);
    rc = sqlite3_exec_monitored(db_meta, buffer_tostring(sql), 0, 0, NULL);
    if (rc != SQLITE_OK)
        goto error;
    buffer_flush(sql);

    buffer_sprintf(sql, INDEX_ACLK_CHART, uuid_str, uuid_str);
    rc = sqlite3_exec_monitored(db_meta, buffer_tostring(sql), 0, 0, NULL);
    if (rc != SQLITE_OK)
        goto error;
    buffer_flush(sql);

    buffer_sprintf(sql, INDEX_ACLK_CHART_LATEST, uuid_str, uuid_str);
    rc = sqlite3_exec_monitored(db_meta, buffer_tostring(sql), 0, 0, NULL);
    if (rc != SQLITE_OK)
        goto error;
    buffer_flush(sql);

    buffer_sprintf(sql, TRIGGER_ACLK_CHART_PAYLOAD, uuid_str, uuid_str, uuid_str);
    rc = sqlite3_exec_monitored(db_meta, buffer_tostring(sql), 0, 0, NULL);
    if (rc != SQLITE_OK)
        goto error;
    buffer_flush(sql);

    buffer_sprintf(sql, TABLE_ACLK_ALERT, uuid_str);
    rc = sqlite3_exec_monitored(db_meta, buffer_tostring(sql), 0, 0, NULL);
    if (rc != SQLITE_OK)
        goto error;
    buffer_flush(sql);

    buffer_sprintf(sql, INDEX_ACLK_ALERT, uuid_str, uuid_str);
    rc = sqlite3_exec_monitored(db_meta, buffer_tostring(sql), 0, 0, NULL);
    if (rc != SQLITE_OK)
        goto error;
    buffer_flush(sql);

    buffer_free(sql);
    fprintf(stderr,"SQLite is OK\n");
    return 0;
error:
    fprintf(stderr,"SQLite statement failed: %s\n", buffer_tostring(sql));
    buffer_free(sql);
    fprintf(stderr,"SQLite tests failed\n");
    return 1;
}

int unit_test_bitmap256(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );

    BITMAP256 test_bitmap = {0};

    bitmap256_set_bit(&test_bitmap, 0, 1);
    bitmap256_set_bit(&test_bitmap, 64, 1);
    bitmap256_set_bit(&test_bitmap, 128, 1);
    bitmap256_set_bit(&test_bitmap, 192, 1);
    if (test_bitmap.data[0] == 1)
        fprintf(stderr, "%s() INDEX 1 is OK\n", __FUNCTION__ );
    if (test_bitmap.data[1] == 1)
        fprintf(stderr, "%s() INDEX 65 is OK\n", __FUNCTION__ );
    if (test_bitmap.data[2] == 1)
        fprintf(stderr, "%s() INDEX 129 is OK\n", __FUNCTION__ );
    if (test_bitmap.data[3] == 1)
        fprintf(stderr, "%s() INDEX 192 is OK\n", __FUNCTION__ );

    uint8_t i=0;
    int j = 0;
    do {
        bitmap256_set_bit(&test_bitmap, i++, 1);
        j++;
    } while (j < 256);

    if (test_bitmap.data[0] == 0xffffffffffffffff)
        fprintf(stderr, "%s() INDEX 0 is fully set OK\n", __FUNCTION__);
    else {
        fprintf(stderr, "%s() INDEX 0 is %lx expected 0xffffffffffffffff\n", __FUNCTION__, test_bitmap.data[0]);
        return 1;
    }

    if (test_bitmap.data[1] == 0xffffffffffffffff)
        fprintf(stderr, "%s() INDEX 1 is fully set OK\n", __FUNCTION__);
    else {
        fprintf(stderr, "%s() INDEX 1 is %lx expected 0xffffffffffffffff\n", __FUNCTION__, test_bitmap.data[0]);
        return 1;
    }

    if (test_bitmap.data[2] == 0xffffffffffffffff)
        fprintf(stderr, "%s() INDEX 2 is fully set OK\n", __FUNCTION__);
    else {
        fprintf(stderr, "%s() INDEX 2 is %lx expected 0xffffffffffffffff\n", __FUNCTION__, test_bitmap.data[0]);
        return 1;
    }

    if (test_bitmap.data[3] == 0xffffffffffffffff)
        fprintf(stderr, "%s() INDEX 3 is fully set OK\n", __FUNCTION__);
    else {
        fprintf(stderr, "%s() INDEX 3 is %lx expected 0xffffffffffffffff\n", __FUNCTION__, test_bitmap.data[0]);
        return 1;
    }

    i = 0;
    j = 0;
    do {
        bitmap256_set_bit(&test_bitmap, i++, 0);
        j++;
    } while (j < 256);

    if (test_bitmap.data[0] == 0)
        fprintf(stderr, "%s() INDEX 0 is reset OK\n", __FUNCTION__);
    else {
        fprintf(stderr, "%s() INDEX 0 is not reset FAILED\n", __FUNCTION__);
        return 1;
    }
    if (test_bitmap.data[1] == 0)
        fprintf(stderr, "%s() INDEX 1 is reset OK\n", __FUNCTION__);
    else {
        fprintf(stderr, "%s() INDEX 1 is not reset FAILED\n", __FUNCTION__);
        return 1;
    }

    if (test_bitmap.data[2] == 0)
        fprintf(stderr, "%s() INDEX 2 is reset OK\n", __FUNCTION__);
    else {
        fprintf(stderr, "%s() INDEX 2 is not reset FAILED\n", __FUNCTION__);
        return 1;
    }

    if (test_bitmap.data[3] == 0)
        fprintf(stderr, "%s() INDEX 3 is reset OK\n", __FUNCTION__);
    else {
        fprintf(stderr, "%s() INDEX 3 is not reset FAILED\n", __FUNCTION__);
        return 1;
    }

    i=0;
    j = 0;
    do {
        bitmap256_set_bit(&test_bitmap, i, 1);
        i += 4;
        j += 4;
    } while (j < 256);

    if (test_bitmap.data[0] == 0x1111111111111111)
        fprintf(stderr, "%s() INDEX 0 is 0x1111111111111111 set OK\n", __FUNCTION__);
    else {
        fprintf(stderr, "%s() INDEX 0 is %lx expected 0x1111111111111111\n", __FUNCTION__, test_bitmap.data[0]);
        return 1;
    }

    if (test_bitmap.data[1] == 0x1111111111111111)
        fprintf(stderr, "%s() INDEX 1 is 0x1111111111111111 set OK\n", __FUNCTION__);
    else {
        fprintf(stderr, "%s() INDEX 1 is %lx expected 0x1111111111111111\n", __FUNCTION__, test_bitmap.data[1]);
        return 1;
    }

    if (test_bitmap.data[2] == 0x1111111111111111)
        fprintf(stderr, "%s() INDEX 2 is 0x1111111111111111 set OK\n", __FUNCTION__);
    else {
        fprintf(stderr, "%s() INDEX 2 is %lx expected 0x1111111111111111\n", __FUNCTION__, test_bitmap.data[2]);
        return 1;
    }

    if (test_bitmap.data[3] == 0x1111111111111111)
        fprintf(stderr, "%s() INDEX 3 is 0x1111111111111111 set OK\n", __FUNCTION__);
    else {
        fprintf(stderr, "%s() INDEX 3 is %lx expected 0x1111111111111111\n", __FUNCTION__, test_bitmap.data[3]);
        return 1;
    }

    fprintf(stderr, "%s() tests passed\n", __FUNCTION__);
    return 0;
}

#ifdef ENABLE_DBENGINE
static inline void rrddim_set_by_pointer_fake_time(RRDDIM *rd, collected_number value, time_t now)
{
    rd->last_collected_time.tv_sec = now;
    rd->last_collected_time.tv_usec = 0;
    rd->collected_value = value;
    rd->updated = 1;

    rd->collections_counter++;

    collected_number v = (value >= 0) ? value : -value;
    if(unlikely(v > rd->collected_value_max)) rd->collected_value_max = v;
}

static RRDHOST *dbengine_rrdhost_find_or_create(char *name)
{
    /* We don't want to drop metrics when generating load, we prefer to block data generation itself */
    rrdeng_drop_metrics_under_page_cache_pressure = 0;

    return rrdhost_find_or_create(
            name
            , name
            , name
            , os_type
            , netdata_configured_timezone
            , netdata_configured_abbrev_timezone
            , netdata_configured_utc_offset
            , ""
            , program_name
            , program_version
            , default_rrd_update_every
            , default_rrd_history_entries
            , RRD_MEMORY_MODE_DBENGINE
            , default_health_enabled
            , default_rrdpush_enabled
            , default_rrdpush_destination
            , default_rrdpush_api_key
            , default_rrdpush_send_charts_matching
            , NULL
            , 0
    );
}

// constants for test_dbengine
static const int CHARTS = 64;
static const int DIMS = 16; // That gives us 64 * 16 = 1024 metrics
#define REGIONS  (3) // 3 regions of update_every
// first region update_every is 2, second is 3, third is 1
static const int REGION_UPDATE_EVERY[REGIONS] = {2, 3, 1};
static const int REGION_POINTS[REGIONS] = {
        16384, // This produces 64MiB of metric data for the first region: update_every = 2
        16384, // This produces 64MiB of metric data for the second region: update_every = 3
        16384, // This produces 64MiB of metric data for the third region: update_every = 1
};
static const int QUERY_BATCH = 4096;

static void test_dbengine_create_charts(RRDHOST *host, RRDSET *st[CHARTS], RRDDIM *rd[CHARTS][DIMS],
                                        int update_every)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    int i, j;
    char name[101];

    for (i = 0 ; i < CHARTS ; ++i) {
        snprintfz(name, 100, "dbengine-chart-%d", i);

        // create the chart
        st[i] = rrdset_create(host, "netdata", name, name, "netdata", NULL, "Unit Testing", "a value", "unittest",
                              NULL, 1, update_every, RRDSET_TYPE_LINE);
        rrdset_flag_set(st[i], RRDSET_FLAG_DEBUG);
        rrdset_flag_set(st[i], RRDSET_FLAG_STORE_FIRST);
        for (j = 0 ; j < DIMS ; ++j) {
            snprintfz(name, 100, "dim-%d", j);

            rd[i][j] = rrddim_add(st[i], name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
    }

    // Initialize DB with the very first entries
    for (i = 0 ; i < CHARTS ; ++i) {
        for (j = 0 ; j < DIMS ; ++j) {
            rd[i][j]->last_collected_time.tv_sec =
            st[i]->last_collected_time.tv_sec = st[i]->last_updated.tv_sec = 2 * API_RELATIVE_TIME_MAX - 1;
            rd[i][j]->last_collected_time.tv_usec =
            st[i]->last_collected_time.tv_usec = st[i]->last_updated.tv_usec = 0;
        }
    }
    for (i = 0 ; i < CHARTS ; ++i) {
        st[i]->usec_since_last_update = USEC_PER_SEC;

        for (j = 0; j < DIMS; ++j) {
            rrddim_set_by_pointer_fake_time(rd[i][j], 69, 2 * API_RELATIVE_TIME_MAX); // set first value to 69
        }
        rrdset_done(st[i]);
    }
    // Fluh pages for subsequent real values
    for (i = 0 ; i < CHARTS ; ++i) {
        for (j = 0; j < DIMS; ++j) {
            rrdeng_store_metric_flush_current_page((rd[i][j])->tiers[0]->db_collection_handle);
        }
    }
}

// Feeds the database region with test data, returns last timestamp of region
static time_t test_dbengine_create_metrics(RRDSET *st[CHARTS], RRDDIM *rd[CHARTS][DIMS],
                                           int current_region, time_t time_start)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    time_t time_now;
    int i, j, c, update_every;
    collected_number next;

    update_every = REGION_UPDATE_EVERY[current_region];
    time_now = time_start;
    // feed it with the test data
    for (i = 0 ; i < CHARTS ; ++i) {
        for (j = 0 ; j < DIMS ; ++j) {
            rd[i][j]->last_collected_time.tv_sec =
            st[i]->last_collected_time.tv_sec = st[i]->last_updated.tv_sec = time_now;
            rd[i][j]->last_collected_time.tv_usec =
            st[i]->last_collected_time.tv_usec = st[i]->last_updated.tv_usec = 0;
        }
    }
    for (c = 0; c < REGION_POINTS[current_region] ; ++c) {
        time_now += update_every; // time_now = start + (c + 1) * update_every
        for (i = 0 ; i < CHARTS ; ++i) {
            st[i]->usec_since_last_update = USEC_PER_SEC * update_every;

            for (j = 0; j < DIMS; ++j) {
                next = ((collected_number)i * DIMS) * REGION_POINTS[current_region] +
                       j * REGION_POINTS[current_region] + c;
                rrddim_set_by_pointer_fake_time(rd[i][j], next, time_now);
            }
            rrdset_done(st[i]);
        }
    }
    return time_now; //time_end
}

// Checks the metric data for the given region, returns number of errors
static int test_dbengine_check_metrics(RRDSET *st[CHARTS], RRDDIM *rd[CHARTS][DIMS],
                                       int current_region, time_t time_start)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    uint8_t same;
    time_t time_now, time_retrieved, end_time;
    int i, j, k, c, errors, update_every;
    collected_number last;
    NETDATA_DOUBLE value, expected;
    struct rrddim_query_handle handle;
    size_t value_errors = 0, time_errors = 0;

    update_every = REGION_UPDATE_EVERY[current_region];
    errors = 0;

    // check the result
    for (c = 0; c < REGION_POINTS[current_region] ; c += QUERY_BATCH) {
        time_now = time_start + (c + 1) * update_every;
        for (i = 0 ; i < CHARTS ; ++i) {
            for (j = 0; j < DIMS; ++j) {
                rd[i][j]->tiers[0]->query_ops.init(rd[i][j]->tiers[0]->db_metric_handle, &handle, time_now, time_now + QUERY_BATCH * update_every, TIER_QUERY_FETCH_SUM);
                for (k = 0; k < QUERY_BATCH; ++k) {
                    last = ((collected_number)i * DIMS) * REGION_POINTS[current_region] +
                           j * REGION_POINTS[current_region] + c + k;
                    expected = unpack_storage_number(pack_storage_number((NETDATA_DOUBLE)last, SN_DEFAULT_FLAGS));

                    STORAGE_POINT sp = rd[i][j]->tiers[0]->query_ops.next_metric(&handle);
                    value = sp.sum;
                    time_retrieved = sp.start_time;
                    end_time = sp.end_time;

                    same = (roundndd(value) == roundndd(expected)) ? 1 : 0;
                    if(!same) {
                        if(!value_errors)
                            fprintf(stderr, "    DB-engine unittest %s/%s: at %lu secs, expecting value " NETDATA_DOUBLE_FORMAT
                                ", found " NETDATA_DOUBLE_FORMAT ", ### E R R O R ###\n",
                                    rrdset_name(st[i]), rrddim_name(rd[i][j]), (unsigned long)time_now + k * update_every, expected, value);
                        value_errors++;
                        errors++;
                    }
                    if(end_time != time_now + k * update_every) {
                        if(!time_errors)
                            fprintf(stderr, "    DB-engine unittest %s/%s: at %lu secs, found timestamp %lu ### E R R O R ###\n",
                                    rrdset_name(st[i]), rrddim_name(rd[i][j]), (unsigned long)time_now + k * update_every, (unsigned long)time_retrieved);
                        time_errors++;
                        errors++;
                    }
                }
                rd[i][j]->tiers[0]->query_ops.finalize(&handle);
            }
        }
    }

    if(value_errors)
        fprintf(stderr, "%zu value errors encountered\n", value_errors);

    if(time_errors)
        fprintf(stderr, "%zu time errors encountered\n", time_errors);

    return errors;
}

// Check rrdr transformations
static int test_dbengine_check_rrdr(RRDSET *st[CHARTS], RRDDIM *rd[CHARTS][DIMS],
                                    int current_region, time_t time_start, time_t time_end)
{
    int update_every = REGION_UPDATE_EVERY[current_region];
    fprintf(stderr, "%s() running on region %d, start time %ld, end time %ld, update every %d...\n", __FUNCTION__, current_region, time_start, time_end, update_every);
    uint8_t same;
    time_t time_now, time_retrieved;
    int i, j, errors, value_errors = 0, time_errors = 0;
    long c;
    collected_number last;
    NETDATA_DOUBLE value, expected;

    errors = 0;
    long points = (time_end - time_start) / update_every;
    for (i = 0 ; i < CHARTS ; ++i) {
        ONEWAYALLOC *owa = onewayalloc_create(0);
        RRDR *r = rrd2rrdr(owa, st[i], points, time_start, time_end,
                           RRDR_GROUPING_AVERAGE, 0, RRDR_OPTION_NATURAL_POINTS,
                           NULL, NULL, NULL, 0, 0);

        if (!r) {
            fprintf(stderr, "    DB-engine unittest %s: empty RRDR on region %d ### E R R O R ###\n", rrdset_name(st[i]), current_region);
            return ++errors;
        } else {
            assert(r->st == st[i]);
            for (c = 0; c != rrdr_rows(r) ; ++c) {
                RRDDIM *d;
                time_now = time_start + (c + 1) * update_every;
                time_retrieved = r->t[c];

                // for each dimension
                for (j = 0, d = r->st->dimensions ; d && j < r->d ; ++j, d = d->next) {
                    NETDATA_DOUBLE *cn = &r->v[ c * r->d ];
                    value = cn[j];
                    assert(rd[i][j] == d);

                    last = i * DIMS * REGION_POINTS[current_region] + j * REGION_POINTS[current_region] + c;
                    expected = unpack_storage_number(pack_storage_number((NETDATA_DOUBLE)last, SN_DEFAULT_FLAGS));

                    same = (roundndd(value) == roundndd(expected)) ? 1 : 0;
                    if(!same) {
                        if(value_errors < 20)
                            fprintf(stderr, "    DB-engine unittest %s/%s: at %lu secs, expecting value " NETDATA_DOUBLE_FORMAT
                                ", RRDR found " NETDATA_DOUBLE_FORMAT ", ### E R R O R ###\n",
                                    rrdset_name(st[i]), rrddim_name(rd[i][j]), (unsigned long)time_now, expected, value);
                        value_errors++;
                    }
                    if(time_retrieved != time_now) {
                        if(time_errors < 20)
                            fprintf(stderr, "    DB-engine unittest %s/%s: at %lu secs, found RRDR timestamp %lu ### E R R O R ###\n",
                                    rrdset_name(st[i]), rrddim_name(rd[i][j]), (unsigned long)time_now, (unsigned long)time_retrieved);
                        time_errors++;
                    }
                }
            }
            rrdr_free(owa, r);
        }
        onewayalloc_destroy(owa);
    }

    if(value_errors)
        fprintf(stderr, "%d value errors encountered\n", value_errors);

    if(time_errors)
        fprintf(stderr, "%d time errors encountered\n", time_errors);

    return errors + value_errors + time_errors;
}

int test_dbengine(void)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    int i, j, errors, value_errors = 0, time_errors = 0, update_every, current_region;
    RRDHOST *host = NULL;
    RRDSET *st[CHARTS];
    RRDDIM *rd[CHARTS][DIMS];
    time_t time_start[REGIONS], time_end[REGIONS];

    error_log_limit_unlimited();
    fprintf(stderr, "\nRunning DB-engine test\n");

    default_rrd_memory_mode = RRD_MEMORY_MODE_DBENGINE;

    fprintf(stderr, "Initializing localhost with hostname 'unittest-dbengine'");
    host = dbengine_rrdhost_find_or_create("unittest-dbengine");
    if (NULL == host)
        return 1;

    current_region = 0; // this is the first region of data
    update_every = REGION_UPDATE_EVERY[current_region]; // set data collection frequency to 2 seconds
    test_dbengine_create_charts(host, st, rd, update_every);

    time_start[current_region] = 2 * API_RELATIVE_TIME_MAX;
    time_end[current_region] = test_dbengine_create_metrics(st,rd, current_region, time_start[current_region]);

    errors = test_dbengine_check_metrics(st, rd, current_region, time_start[current_region]);
    if (errors)
        goto error_out;

    current_region = 1; //this is the second region of data
    update_every = REGION_UPDATE_EVERY[current_region]; // set data collection frequency to 3 seconds
    // Align pages for frequency change
    for (i = 0 ; i < CHARTS ; ++i) {
        st[i]->update_every = update_every;
        for (j = 0; j < DIMS; ++j) {
            rrdeng_store_metric_flush_current_page((rd[i][j])->tiers[0]->db_collection_handle);
        }
    }

    time_start[current_region] = time_end[current_region - 1] + update_every;
    if (0 != time_start[current_region] % update_every) // align to update_every
        time_start[current_region] += update_every - time_start[current_region] % update_every;
    time_end[current_region] = test_dbengine_create_metrics(st,rd, current_region, time_start[current_region]);

    errors = test_dbengine_check_metrics(st, rd, current_region, time_start[current_region]);
    if (errors)
        goto error_out;

    current_region = 2; //this is the third region of data
    update_every = REGION_UPDATE_EVERY[current_region]; // set data collection frequency to 1 seconds
    // Align pages for frequency change
    for (i = 0 ; i < CHARTS ; ++i) {
        st[i]->update_every = update_every;
        for (j = 0; j < DIMS; ++j) {
            rrdeng_store_metric_flush_current_page((rd[i][j])->tiers[0]->db_collection_handle);
        }
    }

    time_start[current_region] = time_end[current_region - 1] + update_every;
    if (0 != time_start[current_region] % update_every) // align to update_every
        time_start[current_region] += update_every - time_start[current_region] % update_every;
    time_end[current_region] = test_dbengine_create_metrics(st,rd, current_region, time_start[current_region]);

    errors = test_dbengine_check_metrics(st, rd, current_region, time_start[current_region]);
    if (errors)
        goto error_out;

    for (current_region = 0 ; current_region < REGIONS ; ++current_region) {
        errors = test_dbengine_check_rrdr(st, rd, current_region, time_start[current_region], time_end[current_region]);
        if (errors)
            goto error_out;
    }

    current_region = 1;
    update_every = REGION_UPDATE_EVERY[current_region]; // use the maximum update_every = 3
    errors = 0;
    long points = (time_end[REGIONS - 1] - time_start[0]) / update_every; // cover all time regions with RRDR
    long point_offset = (time_start[current_region] - time_start[0]) / update_every;
    for (i = 0 ; i < CHARTS ; ++i) {
        ONEWAYALLOC *owa = onewayalloc_create(0);
        RRDR *r = rrd2rrdr(owa, st[i], points, time_start[0] + update_every,
                           time_end[REGIONS - 1], RRDR_GROUPING_AVERAGE, 0,
                           RRDR_OPTION_NATURAL_POINTS, NULL, NULL, NULL, 0, 0);
        if (!r) {
            fprintf(stderr, "    DB-engine unittest %s: empty RRDR ### E R R O R ###\n", rrdset_name(st[i]));
            ++errors;
        } else {
            long c;

            assert(r->st == st[i]);
            // test current region values only, since they must be left unchanged
            for (c = point_offset ; c < point_offset + rrdr_rows(r) / REGIONS / 2 ; ++c) {
                RRDDIM *d;
                time_t time_now = time_start[current_region] + (c - point_offset + 2) * update_every;
                time_t time_retrieved = r->t[c];

                // for each dimension
                for(j = 0, d = r->st->dimensions ; d && j < r->d ; ++j, d = d->next) {
                    NETDATA_DOUBLE *cn = &r->v[ c * r->d ];
                    NETDATA_DOUBLE value = cn[j];
                    assert(rd[i][j] == d);

                    collected_number last = i * DIMS * REGION_POINTS[current_region] + j * REGION_POINTS[current_region] + c - point_offset + 1;
                    NETDATA_DOUBLE expected = unpack_storage_number(pack_storage_number((NETDATA_DOUBLE)last, SN_DEFAULT_FLAGS));

                    uint8_t same = (roundndd(value) == roundndd(expected)) ? 1 : 0;
                    if(!same) {
                        if(!value_errors)
                            fprintf(stderr, "    DB-engine unittest %s/%s: at %lu secs, expecting value " NETDATA_DOUBLE_FORMAT
                                ", RRDR found " NETDATA_DOUBLE_FORMAT ", ### E R R O R ###\n",
                                    rrdset_name(st[i]), rrddim_name(rd[i][j]), (unsigned long)time_now, expected, value);
                        value_errors++;
                    }
                    if(time_retrieved != time_now) {
                        if(!time_errors)
                            fprintf(stderr, "    DB-engine unittest %s/%s: at %lu secs, found RRDR timestamp %lu ### E R R O R ###\n",
                                    rrdset_name(st[i]), rrddim_name(rd[i][j]), (unsigned long)time_now, (unsigned long)time_retrieved);
                        time_errors++;
                    }
                }
            }
            rrdr_free(owa, r);
        }
        onewayalloc_destroy(owa);
    }
error_out:
    rrd_wrlock();
    rrdeng_prepare_exit((struct rrdengine_instance *)host->storage_instance[0]);
    rrdhost_delete_charts(host);
    rrdeng_exit((struct rrdengine_instance *)host->storage_instance[0]);
    rrd_unlock();

    return errors + value_errors + time_errors;
}

struct dbengine_chart_thread {
    uv_thread_t thread;
    RRDHOST *host;
    char *chartname; /* Will be prefixed by type, e.g. "example_local1.", "example_local2." etc */
    unsigned dset_charts; /* number of charts */
    unsigned dset_dims; /* dimensions per chart */
    unsigned chart_i; /* current chart offset */
    time_t time_present; /* current virtual time of the benchmark */
    volatile time_t time_max; /* latest timestamp of stored values */
    unsigned history_seconds; /* how far back in the past to go */

    volatile long done; /* initialize to 0, set to 1 to stop thread */
    struct completion charts_initialized;
    unsigned long errors, stored_metrics_nr; /* statistics */

    RRDSET *st;
    RRDDIM *rd[]; /* dset_dims elements */
};

collected_number generate_dbengine_chart_value(int chart_i, int dim_i, time_t time_current)
{
    collected_number value;

    value = ((collected_number)time_current) * (chart_i + 1);
    value += ((collected_number)time_current) * (dim_i + 1);
    value %= 1024LLU;

    return value;
}

static void generate_dbengine_chart(void *arg)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    struct dbengine_chart_thread *thread_info = (struct dbengine_chart_thread *)arg;
    RRDHOST *host = thread_info->host;
    char *chartname = thread_info->chartname;
    const unsigned DSET_DIMS = thread_info->dset_dims;
    unsigned history_seconds = thread_info->history_seconds;
    time_t time_present = thread_info->time_present;

    unsigned j, update_every = 1;
    RRDSET *st;
    RRDDIM *rd[DSET_DIMS];
    char name[RRD_ID_LENGTH_MAX + 1];
    time_t time_current;

    // create the chart
    snprintfz(name, RRD_ID_LENGTH_MAX, "example_local%u", thread_info->chart_i + 1);
    thread_info->st = st = rrdset_create(host, name, chartname, chartname, "example", NULL, chartname, chartname,
                                         chartname, NULL, 1, update_every, RRDSET_TYPE_LINE);
    for (j = 0 ; j < DSET_DIMS ; ++j) {
        snprintfz(name, RRD_ID_LENGTH_MAX, "%s%u", chartname, j + 1);

        thread_info->rd[j] = rd[j] = rrddim_add(st, name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    completion_mark_complete(&thread_info->charts_initialized);

    // feed it with the test data
    time_current = time_present - history_seconds;
    for (j = 0 ; j < DSET_DIMS ; ++j) {
        rd[j]->last_collected_time.tv_sec =
        st->last_collected_time.tv_sec = st->last_updated.tv_sec = time_current - update_every;
        rd[j]->last_collected_time.tv_usec =
        st->last_collected_time.tv_usec = st->last_updated.tv_usec = 0;
    }
    for( ; !thread_info->done && time_current < time_present ; time_current += update_every) {
        st->usec_since_last_update = USEC_PER_SEC * update_every;

        for (j = 0; j < DSET_DIMS; ++j) {
            collected_number value;

            value = generate_dbengine_chart_value(thread_info->chart_i, j, time_current);
            rrddim_set_by_pointer_fake_time(rd[j], value, time_current);
            ++thread_info->stored_metrics_nr;
        }
        rrdset_done(st);
        thread_info->time_max = time_current;
    }
    for (j = 0; j < DSET_DIMS; ++j) {
        rrdeng_store_metric_finalize((rd[j])->tiers[0]->db_collection_handle);
    }
}

void generate_dbengine_dataset(unsigned history_seconds)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    const int DSET_CHARTS = 16;
    const int DSET_DIMS = 128;
    const uint64_t EXPECTED_COMPRESSION_RATIO = 20;
    RRDHOST *host = NULL;
    struct dbengine_chart_thread **thread_info;
    int i;
    time_t time_present;

    default_rrd_memory_mode = RRD_MEMORY_MODE_DBENGINE;
    default_rrdeng_page_cache_mb = 128;
    // Worst case for uncompressible data
    default_rrdeng_disk_quota_mb = (((uint64_t)DSET_DIMS * DSET_CHARTS) * sizeof(storage_number) * history_seconds) /
                                   (1024 * 1024);
    default_rrdeng_disk_quota_mb -= default_rrdeng_disk_quota_mb * EXPECTED_COMPRESSION_RATIO / 100;

    error_log_limit_unlimited();
    fprintf(stderr, "Initializing localhost with hostname 'dbengine-dataset'");

    host = dbengine_rrdhost_find_or_create("dbengine-dataset");
    if (NULL == host)
        return;

    thread_info = mallocz(sizeof(*thread_info) * DSET_CHARTS);
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        thread_info[i] = mallocz(sizeof(*thread_info[i]) + sizeof(RRDDIM *) * DSET_DIMS);
    }
    fprintf(stderr, "\nRunning DB-engine workload generator\n");

    time_present = now_realtime_sec();
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        thread_info[i]->host = host;
        thread_info[i]->chartname = "random";
        thread_info[i]->dset_charts = DSET_CHARTS;
        thread_info[i]->chart_i = i;
        thread_info[i]->dset_dims = DSET_DIMS;
        thread_info[i]->history_seconds = history_seconds;
        thread_info[i]->time_present = time_present;
        thread_info[i]->time_max = 0;
        thread_info[i]->done = 0;
        completion_init(&thread_info[i]->charts_initialized);
        assert(0 == uv_thread_create(&thread_info[i]->thread, generate_dbengine_chart, thread_info[i]));
        completion_wait_for(&thread_info[i]->charts_initialized);
        completion_destroy(&thread_info[i]->charts_initialized);
    }
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        assert(0 == uv_thread_join(&thread_info[i]->thread));
    }

    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        freez(thread_info[i]);
    }
    freez(thread_info);
    rrd_wrlock();
    rrdhost_free(host, 1);
    rrd_unlock();
}

struct dbengine_query_thread {
    uv_thread_t thread;
    RRDHOST *host;
    char *chartname; /* Will be prefixed by type, e.g. "example_local1.", "example_local2." etc */
    unsigned dset_charts; /* number of charts */
    unsigned dset_dims; /* dimensions per chart */
    time_t time_present; /* current virtual time of the benchmark */
    unsigned history_seconds; /* how far back in the past to go */
    volatile long done; /* initialize to 0, set to 1 to stop thread */
    unsigned long errors, queries_nr, queried_metrics_nr; /* statistics */
    uint8_t delete_old_data; /* if non zero then data are deleted when disk space is exhausted */

    struct dbengine_chart_thread *chart_threads[]; /* dset_charts elements */
};

static void query_dbengine_chart(void *arg)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    struct dbengine_query_thread *thread_info = (struct dbengine_query_thread *)arg;
    const int DSET_CHARTS = thread_info->dset_charts;
    const int DSET_DIMS = thread_info->dset_dims;
    time_t time_after, time_before, time_min, time_approx_min, time_max, duration;
    int i, j, update_every = 1;
    RRDSET *st;
    RRDDIM *rd;
    uint8_t same;
    time_t time_now, time_retrieved, end_time;
    collected_number generatedv;
    NETDATA_DOUBLE value, expected;
    struct rrddim_query_handle handle;
    size_t value_errors = 0, time_errors = 0;

    do {
        // pick a chart and dimension
        i = random() % DSET_CHARTS;
        st = thread_info->chart_threads[i]->st;
        j = random() % DSET_DIMS;
        rd = thread_info->chart_threads[i]->rd[j];

        time_min = thread_info->time_present - thread_info->history_seconds + 1;
        time_max = thread_info->chart_threads[i]->time_max;

        if (thread_info->delete_old_data) {
            /* A time window of twice the disk space is sufficient for compression space savings of up to 50% */
            time_approx_min = time_max - (default_rrdeng_disk_quota_mb * 2 * 1024 * 1024) /
                                         (((uint64_t) DSET_DIMS * DSET_CHARTS) * sizeof(storage_number));
            time_min = MAX(time_min, time_approx_min);
        }
        if (!time_max) {
            time_before = time_after = time_min;
        } else {
            time_after = time_min + random() % (MAX(time_max - time_min, 1));
            duration = random() % 3600;
            time_before = MIN(time_after + duration, time_max); /* up to 1 hour queries */
        }

        rd->tiers[0]->query_ops.init(rd->tiers[0]->db_metric_handle, &handle, time_after, time_before, TIER_QUERY_FETCH_SUM);
        ++thread_info->queries_nr;
        for (time_now = time_after ; time_now <= time_before ; time_now += update_every) {
            generatedv = generate_dbengine_chart_value(i, j, time_now);
            expected = unpack_storage_number(pack_storage_number((NETDATA_DOUBLE) generatedv, SN_DEFAULT_FLAGS));

            if (unlikely(rd->tiers[0]->query_ops.is_finished(&handle))) {
                if (!thread_info->delete_old_data) { /* data validation only when we don't delete */
                    fprintf(stderr, "    DB-engine stresstest %s/%s: at %lu secs, expecting value " NETDATA_DOUBLE_FORMAT
                        ", found data gap, ### E R R O R ###\n",
                            rrdset_name(st), rrddim_name(rd), (unsigned long) time_now, expected);
                    ++thread_info->errors;
                }
                break;
            }

            STORAGE_POINT sp = rd->tiers[0]->query_ops.next_metric(&handle);
            value = sp.sum;
            time_retrieved = sp.start_time;
            end_time = sp.end_time;

            if (!netdata_double_isnumber(value)) {
                if (!thread_info->delete_old_data) { /* data validation only when we don't delete */
                    fprintf(stderr, "    DB-engine stresstest %s/%s: at %lu secs, expecting value " NETDATA_DOUBLE_FORMAT
                        ", found data gap, ### E R R O R ###\n",
                            rrdset_name(st), rrddim_name(rd), (unsigned long) time_now, expected);
                    ++thread_info->errors;
                }
                break;
            }
            ++thread_info->queried_metrics_nr;

            same = (roundndd(value) == roundndd(expected)) ? 1 : 0;
            if (!same) {
                if (!thread_info->delete_old_data) { /* data validation only when we don't delete */
                    if(!value_errors)
                       fprintf(stderr, "    DB-engine stresstest %s/%s: at %lu secs, expecting value " NETDATA_DOUBLE_FORMAT
                            ", found " NETDATA_DOUBLE_FORMAT ", ### E R R O R ###\n",
                                rrdset_name(st), rrddim_name(rd), (unsigned long) time_now, expected, value);
                    value_errors++;
                    thread_info->errors++;
                }
            }
            if (end_time != time_now) {
                if (!thread_info->delete_old_data) { /* data validation only when we don't delete */
                    if(!time_errors)
                        fprintf(stderr,
                            "    DB-engine stresstest %s/%s: at %lu secs, found timestamp %lu ### E R R O R ###\n",
                                rrdset_name(st), rrddim_name(rd), (unsigned long) time_now, (unsigned long) time_retrieved);
                    time_errors++;
                    thread_info->errors++;
                }
            }
        }
        rd->tiers[0]->query_ops.finalize(&handle);
    } while(!thread_info->done);

    if(value_errors)
        fprintf(stderr, "%zu value errors encountered\n", value_errors);

    if(time_errors)
        fprintf(stderr, "%zu time errors encountered\n", time_errors);
}

void dbengine_stress_test(unsigned TEST_DURATION_SEC, unsigned DSET_CHARTS, unsigned QUERY_THREADS,
                          unsigned RAMP_UP_SECONDS, unsigned PAGE_CACHE_MB, unsigned DISK_SPACE_MB)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    const unsigned DSET_DIMS = 128;
    const uint64_t EXPECTED_COMPRESSION_RATIO = 20;
    const unsigned HISTORY_SECONDS = 3600 * 24 * 365 * 50; /* 50 year of history */
    RRDHOST *host = NULL;
    struct dbengine_chart_thread **chart_threads;
    struct dbengine_query_thread **query_threads;
    unsigned i, j;
    time_t time_start, test_duration;

    error_log_limit_unlimited();

    if (!TEST_DURATION_SEC)
        TEST_DURATION_SEC = 10;
    if (!DSET_CHARTS)
        DSET_CHARTS = 1;
    if (!QUERY_THREADS)
        QUERY_THREADS = 1;
    if (PAGE_CACHE_MB < RRDENG_MIN_PAGE_CACHE_SIZE_MB)
        PAGE_CACHE_MB = RRDENG_MIN_PAGE_CACHE_SIZE_MB;

    default_rrd_memory_mode = RRD_MEMORY_MODE_DBENGINE;
    default_rrdeng_page_cache_mb = PAGE_CACHE_MB;
    if (DISK_SPACE_MB) {
        fprintf(stderr, "By setting disk space limit data are allowed to be deleted. "
                        "Data validation is turned off for this run.\n");
        default_rrdeng_disk_quota_mb = DISK_SPACE_MB;
    } else {
        // Worst case for uncompressible data
        default_rrdeng_disk_quota_mb =
                (((uint64_t) DSET_DIMS * DSET_CHARTS) * sizeof(storage_number) * HISTORY_SECONDS) / (1024 * 1024);
        default_rrdeng_disk_quota_mb -= default_rrdeng_disk_quota_mb * EXPECTED_COMPRESSION_RATIO / 100;
    }

    fprintf(stderr, "Initializing localhost with hostname 'dbengine-stress-test'\n");

    (void) sql_init_database(DB_CHECK_NONE, 1);
    host = dbengine_rrdhost_find_or_create("dbengine-stress-test");
    if (NULL == host)
        return;

    chart_threads = mallocz(sizeof(*chart_threads) * DSET_CHARTS);
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        chart_threads[i] = mallocz(sizeof(*chart_threads[i]) + sizeof(RRDDIM *) * DSET_DIMS);
    }
    query_threads = mallocz(sizeof(*query_threads) * QUERY_THREADS);
    for (i = 0 ; i < QUERY_THREADS ; ++i) {
        query_threads[i] = mallocz(sizeof(*query_threads[i]) + sizeof(struct dbengine_chart_thread *) * DSET_CHARTS);
    }
    fprintf(stderr, "\nRunning DB-engine stress test, %u seconds writers ramp-up time,\n"
                    "%u seconds of concurrent readers and writers, %u writer threads, %u reader threads,\n"
                    "%u MiB of page cache.\n",
                    RAMP_UP_SECONDS, TEST_DURATION_SEC, DSET_CHARTS, QUERY_THREADS, PAGE_CACHE_MB);

    time_start = now_realtime_sec() + HISTORY_SECONDS; /* move history to the future */
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        chart_threads[i]->host = host;
        chart_threads[i]->chartname = "random";
        chart_threads[i]->dset_charts = DSET_CHARTS;
        chart_threads[i]->chart_i = i;
        chart_threads[i]->dset_dims = DSET_DIMS;
        chart_threads[i]->history_seconds = HISTORY_SECONDS;
        chart_threads[i]->time_present = time_start;
        chart_threads[i]->time_max = 0;
        chart_threads[i]->done = 0;
        chart_threads[i]->errors = chart_threads[i]->stored_metrics_nr = 0;
        completion_init(&chart_threads[i]->charts_initialized);
        assert(0 == uv_thread_create(&chart_threads[i]->thread, generate_dbengine_chart, chart_threads[i]));
    }
    /* barrier so that subsequent queries can access valid chart data */
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        completion_wait_for(&chart_threads[i]->charts_initialized);
        completion_destroy(&chart_threads[i]->charts_initialized);
    }
    sleep(RAMP_UP_SECONDS);
    /* at this point data have already began being written to the database */
    for (i = 0 ; i < QUERY_THREADS ; ++i) {
        query_threads[i]->host = host;
        query_threads[i]->chartname = "random";
        query_threads[i]->dset_charts = DSET_CHARTS;
        query_threads[i]->dset_dims = DSET_DIMS;
        query_threads[i]->history_seconds = HISTORY_SECONDS;
        query_threads[i]->time_present = time_start;
        query_threads[i]->done = 0;
        query_threads[i]->errors = query_threads[i]->queries_nr = query_threads[i]->queried_metrics_nr = 0;
        for (j = 0 ; j < DSET_CHARTS ; ++j) {
            query_threads[i]->chart_threads[j] = chart_threads[j];
        }
        query_threads[i]->delete_old_data = DISK_SPACE_MB ? 1 : 0;
        assert(0 == uv_thread_create(&query_threads[i]->thread, query_dbengine_chart, query_threads[i]));
    }
    sleep(TEST_DURATION_SEC);
    /* stop workload */
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        chart_threads[i]->done = 1;
    }
    for (i = 0 ; i < QUERY_THREADS ; ++i) {
        query_threads[i]->done = 1;
    }
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        assert(0 == uv_thread_join(&chart_threads[i]->thread));
    }
    for (i = 0 ; i < QUERY_THREADS ; ++i) {
        assert(0 == uv_thread_join(&query_threads[i]->thread));
    }
    test_duration = now_realtime_sec() - (time_start - HISTORY_SECONDS);
    if (!test_duration)
        test_duration = 1;
    fprintf(stderr, "\nDB-engine stress test finished in %ld seconds.\n", test_duration);
    unsigned long stored_metrics_nr = 0;
    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        stored_metrics_nr += chart_threads[i]->stored_metrics_nr;
    }
    unsigned long queried_metrics_nr = 0;
    for (i = 0 ; i < QUERY_THREADS ; ++i) {
        queried_metrics_nr += query_threads[i]->queried_metrics_nr;
    }
    fprintf(stderr, "%u metrics were stored (dataset size of %lu MiB) in %u charts by 1 writer thread per chart.\n",
            DSET_CHARTS * DSET_DIMS, stored_metrics_nr * sizeof(storage_number) / (1024 * 1024), DSET_CHARTS);
    fprintf(stderr, "Metrics were being generated per 1 emulated second and time was accelerated.\n");
    fprintf(stderr, "%lu metric data points were queried by %u reader threads.\n", queried_metrics_nr, QUERY_THREADS);
    fprintf(stderr, "Query starting time is randomly chosen from the beginning of the time-series up to the time of\n"
                    "the latest data point, and ending time from 1 second up to 1 hour after the starting time.\n");
    fprintf(stderr, "Performance is %lu written data points/sec and %lu read data points/sec.\n",
            stored_metrics_nr / test_duration, queried_metrics_nr / test_duration);

    for (i = 0 ; i < DSET_CHARTS ; ++i) {
        freez(chart_threads[i]);
    }
    freez(chart_threads);
    for (i = 0 ; i < QUERY_THREADS ; ++i) {
        freez(query_threads[i]);
    }
    freez(query_threads);
    rrd_wrlock();
    rrdeng_prepare_exit((struct rrdengine_instance *)host->storage_instance[0]);
    rrdhost_delete_charts(host);
    rrdeng_exit((struct rrdengine_instance *)host->storage_instance[0]);
    rrd_unlock();
}

#endif
