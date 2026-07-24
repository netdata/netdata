// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "web/api/formatters/rrd2json.h"
#include "web/api/queries/query-internal.h"
#include "database/contexts/rrdcontext-internal.h"
#ifdef OS_WINDOWS
#include "win_system-info.h"
#endif

#if defined(OS_LINUX)
#include "collectors/proc.plugin/plugin_proc.h"
#endif

static bool cmd_arg_sanitization_test(const char *expected, const char *src, char *dst, size_t dst_size) {
    bool ok = sanitize_command_argument_string(dst, src, dst_size);

    if (!expected)
        return ok == false;

    return strcmp(expected, dst) == 0;
}

bool command_argument_sanitization_tests() {
    char dst[1024];

    for (size_t i = 0; i != 5; i++)  {
        const char *expected = i == 4 ? "'\\''" : NULL;
        if (cmd_arg_sanitization_test(expected, "'", dst, i) == false) {
            fprintf(stderr, "expected: >>>%s<<<, got: >>>%s<<<\n", expected, dst);
            return 1;
        }
    }

    for (size_t i = 0; i != 9; i++)  {
        const char *expected = i == 8 ? "'\\'''\\''" : NULL;
        if (cmd_arg_sanitization_test(expected, "''", dst, i) == false) {
            fprintf(stderr, "expected: >>>%s<<<, got: >>>%s<<<\n", expected, dst);
            return 1;
        }
    }

    for (size_t i = 0; i != 7; i++)  {
        const char *expected = i == 6 ? "'\\''a" : NULL;
        if (cmd_arg_sanitization_test(expected, "'a", dst, i) == false) {
            fprintf(stderr, "expected: >>>%s<<<, got: >>>%s<<<\n", expected, dst);
            return 1;
        }
    }

    for (size_t i = 0; i != 7; i++)  {
        const char *expected = i == 6 ? "a'\\''" : NULL;
        if (cmd_arg_sanitization_test(expected, "a'", dst, i) == false) {
            fprintf(stderr, "expected: >>>%s<<<, got: >>>%s<<<\n", expected, dst);
            return 1;
        }
    }

    for (size_t i = 0; i != 22; i++)  {
        const char *expected = i == 21 ? "foo'\\''a'\\'''\\'''\\''b" : NULL;
        if (cmd_arg_sanitization_test(expected, "--foo'a'''b", dst, i) == false) {
            fprintf(stderr, "expected: >>>%s<<<, got: >>>%s<<<\n length: %zu\n", expected, dst, strlen(dst));
            return 1;
        }
    }

    return 0;
}

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
            { .n = 123.4567899123456789, .correct = "123.4567899" },
            { .n = 123.4567890123456789, .correct = "123.456789" },
            { .n = 123.4567800123456789, .correct = "123.45678" },
            { .n = 123.4567000123456789, .correct = "123.4567" },
            { .n = 123.4560000123456789, .correct = "123.456" },
            { .n = 123.4500000123456789, .correct = "123.45" },
            { .n = 123.4000000123456789, .correct = "123.4" },
            { .n = 123.0000000123456789, .correct = "123" },
            { .n = 123.0000000923456789, .correct = "123.0000001" },
            { .n = 4294967295.123456789, .correct = "4294967295.123457" },
            { .n = 8294967295.123456789, .correct = "8294967295.123457" },
            { .n = 1.000000000000002e+19, .correct = "1.000000000000001998e+19" },
            { .n = 9.2233720368547676e+18, .correct = "9.223372036854767584e+18" },
            { .n = 18446744073709541376.0, .correct = "1.84467440737095424e+19" },
            { .n = 18446744073709551616.0, .correct = "1.844674407370955136e+19" },
            { .n = 12318446744073710600192.0, .correct = "1.231844674407371008e+22" },
            { .n = 1677721499999999885312.0, .correct = "1.677721499999999872e+21" },
            { .n = -1677721499999999885312.0, .correct = "-1.677721499999999872e+21" },
            { .n = -1.677721499999999885312e40, .correct = "-1.677721499999999872e+40" },
            { .n = -16777214999999997337621690403742592008192.0, .correct = "-1.677721499999999616e+40" },
            { .n = 9999.9999999, .correct = "9999.9999999" },
            { .n = -9999.9999999, .correct = "-9999.9999999" },
            { .n = 0, .correct = NULL },
    };

    char netdata[512 + 2], system[512 + 2];
    int i, failed = 0;
    for(i = 0; values[i].correct ; i++) {
        print_netdata_double(netdata, values[i].n);
        snprintfz(system, sizeof(system) - 1, "%0.12" NETDATA_DOUBLE_MODIFIER, (NETDATA_DOUBLE)values[i].n);

        int ok = 1;
        if(strcmp(netdata, values[i].correct) != 0) {
            ok = 0;
            failed++;
        }

        NETDATA_DOUBLE parsed_netdata = str2ndd(netdata, NULL);
        NETDATA_DOUBLE parsed_system = strtondd(netdata, NULL);

        if(parsed_system != parsed_netdata)
            failed++;

        fprintf(stderr, "[%d]. '%s' (system) printed as '%s' (netdata): PRINT %s, "
                        "PARSED %0.12" NETDATA_DOUBLE_MODIFIER " (system), %0.12" NETDATA_DOUBLE_MODIFIER " (netdata): %s\n",
                        i,
                        system, netdata, ok?"OK":"FAILED",
                        parsed_system, parsed_netdata,
                        parsed_netdata == parsed_system ? "OK" : "FAILED");
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
            snprintfz(buffer, sizeof(buffer) - 1, NETDATA_DOUBLE_FORMAT, n);
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
    is_system_ieee754_double();

    char *values[] = {
            "1.2345678",
            "-35.6",
            "0.00123",
            "23842384234234.2",
            ".1",
            "1.2e-10",
            "18446744073709551616.0",
            "18446744073709551616123456789123456789123456789123456789123456789123456789123456789.0",
            "1.8446744073709551616123456789123456789123456789123456789123456789123456789123456789e+300",
            "9.",
            "9.e2",
            "1.2e",
            "1.2e+",
            "1.2e-",
            "1.2e0",
            "1.2e-0",
            "1.2e+0",
            "-1.2e+1",
            "-1.2e-1",
            "1.2e1",
            "1.2e400",
            "hello",
            "1wrong",
            "nan",
            "inf",
            NULL
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
            fprintf(stderr, "Value '%s' is parsed correctly, but endptr is not right (netdata returned %d, but system returned %d)\n",
                    values[i], (int)(e_mine - values[i]), (int)(e_sys - values[i]));
            return -1;
        }

        fprintf(stderr, "str2ndd() parsed value '%s' exactly the same way with strtold(), returned %" NETDATA_DOUBLE_MODIFIER
            " vs %" NETDATA_DOUBLE_MODIFIER "\n", values[i], mine, sys);
    }

    return 0;
}

int unit_test_buffer() {
    BUFFER *wb = buffer_create(1, NULL);
    char string[2048 + 1];
    char final[9000 + 1];
    int i;

    for(i = 0; i < 2048; i++)
        string[i] = (char)((i % 24) + 'a');
    string[2048] = '\0';

    const char *fmt = "string1: %s\nstring2: %s\nstring3: %s\nstring4: %s";
    buffer_sprintf(wb, fmt, string, string, string, string);
    snprintfz(final, sizeof(final) - 1, fmt, string, string, string, string);

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

static int check_jsonwrap_v2_partial_data_trimming_case(RRDR_OPTIONS options, const char *expected) {
    RRDR r = {
        .partial_data_trimming = {
            .max_update_every = 10,
            .expected_after = 100,
            .trimmed_after = 101,
        },
    };

    BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    rrdr_json_wrapper_partial_data_trimming_v2(wb, &r, options);
    buffer_json_finalize(wb);

    const char *actual = buffer_tostring(wb);
    if(strcmp(actual, expected) != 0) {
        fprintf(stderr, "rrdr_json_wrapper_partial_data_trimming_v2() generated '%s', expected '%s'\n",
                actual, expected);
        buffer_free(wb);
        return 1;
    }

    buffer_free(wb);
    return 0;
}

static int test_jsonwrap_v2_partial_data_trimming_raw_metadata(void) {
    const char *expected =
        "{\"partial_data_trimming\":{\"max_update_every\":10,\"expected_after\":100,\"trimmed_after\":101}}";

    if(check_jsonwrap_v2_partial_data_trimming_case(0, "{}"))
        return 1;

    if(check_jsonwrap_v2_partial_data_trimming_case(RRDR_OPTION_DEBUG, expected))
        return 1;

    if(check_jsonwrap_v2_partial_data_trimming_case(RRDR_OPTION_RETURN_RAW, expected))
        return 1;

    if(check_jsonwrap_v2_partial_data_trimming_case(RRDR_OPTION_DEBUG | RRDR_OPTION_RETURN_RAW, expected))
        return 1;

    QUERY_TARGET qt = { 0 };
    qt.window.options = RRDR_OPTION_RETURN_RAW;
    if(!query_target_aggregatable(&qt)) {
        fprintf(stderr, "RRDR_OPTION_RETURN_RAW must keep query_target_aggregatable() true\n");
        return 1;
    }

    qt.window.options = 0;
    if(query_target_aggregatable(&qt)) {
        fprintf(stderr, "query_target_aggregatable() unexpectedly true without RRDR_OPTION_RETURN_RAW\n");
        return 1;
    }

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
        (NETDATA_DOUBLE)0xFFFFFFFFFFFFFFFFULL / (NETDATA_DOUBLE)15 * (NETDATA_DOUBLE)7,
        (NETDATA_DOUBLE)0xFFFFFFFFFFFFFFFFULL / (NETDATA_DOUBLE)15 * (NETDATA_DOUBLE)7,
        (NETDATA_DOUBLE)0xFFFFFFFFFFFFFFFFULL / (NETDATA_DOUBLE)15,
        (NETDATA_DOUBLE)0xFFFFFFFFFFFFFFFFULL / (NETDATA_DOUBLE)15 * (NETDATA_DOUBLE)7,
        (NETDATA_DOUBLE)0xFFFFFFFFFFFFFFFFULL / (NETDATA_DOUBLE)15 * (NETDATA_DOUBLE)7,
        (NETDATA_DOUBLE)0xFFFFFFFFFFFFFFFFULL / (NETDATA_DOUBLE)15,
        (NETDATA_DOUBLE)0xFFFFFFFFFFFFFFFFULL / (NETDATA_DOUBLE)15 * (NETDATA_DOUBLE)7,
        (NETDATA_DOUBLE)0xFFFFFFFFFFFFFFFFULL / (NETDATA_DOUBLE)15 * (NETDATA_DOUBLE)7,
        (NETDATA_DOUBLE)0xFFFFFFFFFFFFFFFFULL / (NETDATA_DOUBLE)15,
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

    default_rrd_memory_mode = RRD_DB_MODE_ALLOC;
    nd_profile.update_every = test->update_every;

    char name[101];
    snprintfz(name, sizeof(name) - 1, "unittest-%s", test->name);

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

        struct timeval now;
        now_realtime_timeval(&now);
        rrdset_timed_done(st, now, false);

        // align the first entry to second boundary
        if(!c) {
            fprintf(stderr, "    > %s: fixing first collection time to be %llu microseconds to second boundary\n", test->name, test->feed[c].microseconds);
            rd->collector.last_collected_time.tv_usec = st->last_collected_time.tv_usec = st->last_updated.tv_usec = test->feed[c].microseconds;
            // time_start = st->last_collected_time.tv_sec;
        }
    }

    // check the result
    int errors = 0;

    if(st->counter != test->result_entries) {
        fprintf(stderr, "    %s stored %u entries, but we were expecting %lu, ### E R R O R ###\n",
                test->name, st->counter, test->result_entries);
        errors++;
    }

    unsigned long max = (st->counter < test->result_entries)?st->counter:test->result_entries;
    for(c = 0 ; c < max ; c++) {
        NETDATA_DOUBLE v = unpack_storage_number(rd->db.data[c]);
        NETDATA_DOUBLE n = unpack_storage_number(pack_storage_number(test->results[c], SN_DEFAULT_FLAGS));
        int same = (roundndd(v * 10000000.0) == roundndd(n * 10000000.0))?1:0;
        fprintf(stderr, "    %s/%s: checking position %lu (at %"PRId64" secs), expecting value " NETDATA_DOUBLE_FORMAT
            ", found " NETDATA_DOUBLE_FORMAT ", %s\n",
            test->name, rrddim_name(rd), c+1,
            (int64_t)((rrdset_first_entry_s(st) + c * st->update_every) - time_start),
            n, v, (same)?"OK":"### E R R O R ###");

        if(!same) errors++;

        if(rd2) {
            v = unpack_storage_number(rd2->db.data[c]);
            n = test->results2[c];
            same = (roundndd(v * 10000000.0) == roundndd(n * 10000000.0))?1:0;
            fprintf(stderr, "    %s/%s: checking position %lu (at %"PRId64" secs), expecting value " NETDATA_DOUBLE_FORMAT
                ", found " NETDATA_DOUBLE_FORMAT ", %s\n",
                test->name, rrddim_name(rd2), c+1,
                (int64_t)((rrdset_first_entry_s(st) + c * st->update_every) - time_start),
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
    rrdset_reset_name(st, "CHARTNAME1");
    fprintf(stderr, "Renamed chart with id '%s' to name '%s'\n", rrdset_id(st), rrdset_name(st));

    fprintf(stderr, "Renaming chart to CHARTNAME2\n");
    rrdset_reset_name(st, "CHARTNAME2");
    fprintf(stderr, "Renamed chart with id '%s' to name '%s'\n", rrdset_id(st), rrdset_name(st));

    fprintf(stderr, "Renaming dimension DIM1 to DIM1NAME1\n");
    rrddim_reset_name(st, rd1, "DIM1NAME1");
    fprintf(stderr, "Renamed dimension with id '%s' to name '%s'\n", rrddim_id(rd1), rrddim_name(rd1));

    fprintf(stderr, "Renaming dimension DIM1 to DIM1NAME2\n");
    rrddim_reset_name(st, rd1, "DIM1NAME2");
    fprintf(stderr, "Renamed dimension with id '%s' to name '%s'\n", rrddim_id(rd1), rrddim_name(rd1));

    fprintf(stderr, "Renaming dimension DIM2 to DIM2NAME1\n");
    rrddim_reset_name(st, rd2, "DIM2NAME1");
    fprintf(stderr, "Renamed dimension with id '%s' to name '%s'\n", rrddim_id(rd2), rrddim_name(rd2));

    fprintf(stderr, "Renaming dimension DIM2 to DIM2NAME2\n");
    rrddim_reset_name(st, rd2, "DIM2NAME2");
    fprintf(stderr, "Renamed dimension with id '%s' to name '%s'\n", rrddim_id(rd2), rrddim_name(rd2));

    BUFFER *buf = buffer_create(1, NULL);
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
        char *s = filename_from_path_entry_strdupz(checks[i].path, checks[i].subpath);
        fprintf(stderr, "filename_from_path_entry_strdupz(\"%s\", \"%s\") = \"%s\": ", checks[i].path, checks[i].subpath, s);
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

static int test_incremental_sum_lookup_respects_update_every(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__);

    const time_t update_every = 10;
    const collected_number increment = 100;
    const size_t samples = 6;
    RRD_DB_MODE old_default_rrd_memory_mode = default_rrd_memory_mode;
    time_t old_update_every = nd_profile.update_every;

    default_rrd_memory_mode = RRD_DB_MODE_ALLOC;
    nd_profile.update_every = update_every;

    char name[101];
    snprintfz(name, sizeof(name) - 1, "unittest-incremental-sum-lookup");

    RRDSET *st = rrdset_create_localhost(
        "netdata", name, name, "netdata", NULL, "Unit Testing", "requests", "unittest", NULL, 1,
        update_every, RRDSET_TYPE_LINE);
    RRDDIM *rd = rrddim_add(st, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

    time_t first_update_s = MAX(2 * API_RELATIVE_TIME_MAX, 200000000);
    st->last_collected_time.tv_sec = st->last_updated.tv_sec = first_update_s - update_every;
    st->last_collected_time.tv_usec = st->last_updated.tv_usec = 0;
    rd->collector.last_collected_time = st->last_collected_time;

    for(size_t i = 0; i <= samples; i++) {
        struct timeval now = {
            .tv_sec = first_update_s + (time_t)(i * update_every),
            .tv_usec = 0,
        };

        st->usec_since_last_update = update_every * USEC_PER_SEC;
        rrddim_timed_set_by_pointer(st, rd, now, (collected_number)(i * increment));
        rrdset_timed_done(st, now, false);
    }

    time_t before = rrdset_last_entry_s(st);
    time_t after = before - (time_t)(samples * update_every);

    ONEWAYALLOC *owa = onewayalloc_create(0);
    NETDATA_DOUBLE value = NAN;
    int value_is_null = 0;
    int ret = rrdset2value_api_v1_with_owa(
        owa, st, NULL, &value, "requests", 1, after, before, RRDR_GROUPING_SUM, NULL, 0,
        RRDR_OPTION_NOT_ALIGNED | RRDR_OPTION_SELECTED_TIER | RRDR_OPTION_MATCH_IDS, NULL, NULL, NULL, NULL,
        NULL, &value_is_null, NULL, 0, 0, QUERY_SOURCE_UNITTEST, STORAGE_PRIORITY_SYNCHRONOUS);
    onewayalloc_destroy(owa);

    NETDATA_DOUBLE expected = (NETDATA_DOUBLE)(samples * increment);
    int rc = 0;
    if(ret != HTTP_RESP_OK || value_is_null || fabsndd(value - expected) > 0.000001) {
        fprintf(
            stderr,
            "incremental sum lookup failed: ret=%d, null=%d, expected " NETDATA_DOUBLE_FORMAT
            ", got " NETDATA_DOUBLE_FORMAT "\n",
            ret, value_is_null, expected, value);
        rc = 1;
    }

    default_rrd_memory_mode = old_default_rrd_memory_mode;
    nd_profile.update_every = old_update_every;
    return rc;
}

static int test_rrdmetric_algorithm_follows_rrddim(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__);

    RRD_DB_MODE old_default_rrd_memory_mode = default_rrd_memory_mode;
    default_rrd_memory_mode = RRD_DB_MODE_ALLOC;

    RRDSET *st = rrdset_create_localhost(
        "netdata", "unittest-algo-track", "unittest-algo-track", "netdata", NULL,
        "Unit Testing", "x", "unittest", NULL, 1,
        nd_profile.update_every, RRDSET_TYPE_LINE);
    RRDDIM *rd = rrddim_add(st, "d", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

    int rc = 0;
    RRDMETRIC *rm = rrdmetric_acquired_value(rd->rrdcontexts.rrdmetric);
    if(!rm || rrdmetric_algorithm_atomic_load(rm) != RRD_ALGORITHM_INCREMENTAL) {
        fprintf(stderr, "%s: after rrddim_add INCREMENTAL, rm->algorithm = %d (want %d)\n",
                __FUNCTION__, rm ? (int)rrdmetric_algorithm_atomic_load(rm) : -1,
                (int)RRD_ALGORITHM_INCREMENTAL);
        rc = 1;
    }

    rrddim_set_algorithm(st, rd, RRD_ALGORITHM_ABSOLUTE);
    rm = rrdmetric_acquired_value(rd->rrdcontexts.rrdmetric);
    if(!rm || rrdmetric_algorithm_atomic_load(rm) != RRD_ALGORITHM_ABSOLUTE) {
        fprintf(stderr, "%s: after rrddim_set_algorithm ABSOLUTE, rm->algorithm = %d (want %d)\n",
                __FUNCTION__, rm ? (int)rrdmetric_algorithm_atomic_load(rm) : -1,
                (int)RRD_ALGORITHM_ABSOLUTE);
        rc = 1;
    }

    default_rrd_memory_mode = old_default_rrd_memory_mode;
    return rc;
}

static int test_rrddim_scale_minimum_magnitude(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__);

    RRD_DB_MODE old_default_rrd_memory_mode = default_rrd_memory_mode;
    default_rrd_memory_mode = RRD_DB_MODE_ALLOC;

    int rc = 0;
    if(rrddim_scale_magnitude(INT32_MIN) != (int64_t)INT32_MAX + 1) {
        fprintf(stderr, "%s: INT32_MIN magnitude is not representable\n", __FUNCTION__);
        rc = 1;
    }

    RRDSET *st_insert = rrdset_create_localhost(
        "netdata", "unittest-scale-min-insert", "unittest-scale-min-insert", "netdata", NULL,
        "Unit Testing", "x", "unittest", NULL, 1,
        nd_profile.update_every, RRDSET_TYPE_LINE);
    RRDDIM *insert_a = rrddim_add(st_insert, "a", NULL, INT32_MIN, INT32_MIN, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *insert_b = rrddim_add(st_insert, "b", NULL, INT32_MIN, INT32_MIN, RRD_ALGORITHM_ABSOLUTE);

    if(insert_a->multiplier != INT32_MIN || insert_a->divisor != INT32_MIN ||
       insert_b->multiplier != INT32_MIN || insert_b->divisor != INT32_MIN ||
       rrdset_flag_check(st_insert, RRDSET_FLAG_HETEROGENEOUS)) {
        fprintf(stderr, "%s: identical signed-minimum dimensions were not preserved as homogeneous\n", __FUNCTION__);
        rc = 1;
    }

    RRDSET *st_multiplier = rrdset_create_localhost(
        "netdata", "unittest-scale-min-multiplier", "unittest-scale-min-multiplier", "netdata", NULL,
        "Unit Testing", "x", "unittest", NULL, 1,
        nd_profile.update_every, RRDSET_TYPE_LINE);
    rrddim_add(st_multiplier, "a", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(st_multiplier, "b", NULL, INT32_MIN, 1, RRD_ALGORITHM_ABSOLUTE);
    rrdset_flag_set(st_multiplier, RRDSET_FLAG_HOMOGENEOUS_CHECK);
    rrdset_update_heterogeneous_flag(st_multiplier);
    if(!rrdset_flag_check(st_multiplier, RRDSET_FLAG_HETEROGENEOUS)) {
        fprintf(stderr, "%s: differing signed-minimum multiplier was not heterogeneous\n", __FUNCTION__);
        rc = 1;
    }

    RRDSET *st_divisor = rrdset_create_localhost(
        "netdata", "unittest-scale-min-divisor", "unittest-scale-min-divisor", "netdata", NULL,
        "Unit Testing", "x", "unittest", NULL, 1,
        nd_profile.update_every, RRDSET_TYPE_LINE);
    rrddim_add(st_divisor, "a", NULL, 1, INT32_MIN, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(st_divisor, "b", NULL, 1, INT32_MIN, RRD_ALGORITHM_ABSOLUTE);
    rrdset_flag_set(st_divisor, RRDSET_FLAG_HOMOGENEOUS_CHECK);
    rrdset_update_heterogeneous_flag(st_divisor);
    if(rrdset_flag_check(st_divisor, RRDSET_FLAG_HETEROGENEOUS)) {
        fprintf(stderr, "%s: identical signed-minimum divisors were not homogeneous\n", __FUNCTION__);
        rc = 1;
    }

    default_rrd_memory_mode = old_default_rrd_memory_mode;
    return rc;
}

static int test_rrddim_collected_minimum_magnitude(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__);

    RRD_DB_MODE old_default_rrd_memory_mode = default_rrd_memory_mode;
    default_rrd_memory_mode = RRD_DB_MODE_ALLOC;

    RRDSET *st = rrdset_create_localhost(
        "netdata", "unittest-collected-min-magnitude", "unittest-collected-min-magnitude", "netdata", NULL,
        "Unit Testing", "x", "unittest", NULL, 1,
        nd_profile.update_every, RRDSET_TYPE_LINE);
    RRDDIM *rd_int = rrddim_add(st, "int", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_float = rrddim_add(st, "float", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    memset(&rd_float->collector.collected, 0, sizeof(rd_float->collector.collected));
    rrddim_option_set(rd_float, RRDDIM_OPTION_VALUE_FLOAT);

    const struct {
        collected_number value;
        uint64_t expected_max;
    } cases[] = {
        { 0, 0 },
        { 1, 1 },
        { -1, 1 },
        { LLONG_MAX, (uint64_t)LLONG_MAX },
        { -LLONG_MAX, (uint64_t)LLONG_MAX },
        { LLONG_MIN, UINT64_C(1) << 63 },
        { 7, UINT64_C(1) << 63 },
    };

    int rc = 0;
    for(size_t i = 0; i < _countof(cases); i++) {
        rrddim_set_by_pointer(st, rd_int, cases[i].value);
        uint64_t actual = rrddim_collected_max_as_uint64(rd_int);
        if(actual != cases[i].expected_max) {
            fprintf(stderr, "%s: case %zu maximum is %" PRIu64 ", expected %" PRIu64 "\n",
                    __FUNCTION__, i, actual, cases[i].expected_max);
            rc = 1;
        }
    }

    if(rd_int->collector.collected.i.collected_value_max != INT64_MIN ||
       rrddim_collected_max_as_double(rd_int) != (NETDATA_DOUBLE)(UINT64_C(1) << 63)) {
        fprintf(stderr, "%s: integer 2^63 maximum representation was not preserved\n", __FUNCTION__);
        rc = 1;
    }

    rrddim_set_by_pointer(st, rd_float, LLONG_MIN);
    rrddim_set_by_pointer(st, rd_float, 7);
    if(rd_float->collector.collected.f.collected_value_max != (NETDATA_DOUBLE)(UINT64_C(1) << 63)) {
        fprintf(stderr, "%s: float lane did not preserve the LLONG_MIN magnitude\n", __FUNCTION__);
        rc = 1;
    }

    default_rrd_memory_mode = old_default_rrd_memory_mode;
    return rc;
}

static int test_rrdset_homogeneity_multiplier_sign(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__);

    RRD_DB_MODE old_default_rrd_memory_mode = default_rrd_memory_mode;
    default_rrd_memory_mode = RRD_DB_MODE_ALLOC;

    struct homogeneity_case {
        const char *name;
        int32_t multiplier_a;
        int32_t multiplier_b;
        int32_t divisor_a;
        int32_t divisor_b;
        RRD_ALGORITHM algorithm_a;
        RRD_ALGORITHM algorithm_b;
        bool heterogeneous;
    } cases[] = {
        { "negative-positive", -1, 1, 1, 1, RRD_ALGORITHM_ABSOLUTE, RRD_ALGORITHM_ABSOLUTE, false },
        { "negative-negative", -1, -1, 1, 1, RRD_ALGORITHM_ABSOLUTE, RRD_ALGORITHM_ABSOLUTE, false },
        { "positive-negative", 1, -1, 1, 1, RRD_ALGORITHM_ABSOLUTE, RRD_ALGORITHM_ABSOLUTE, false },
        { "unequal-magnitude", -1, 2, 1, 1, RRD_ALGORITHM_ABSOLUTE, RRD_ALGORITHM_ABSOLUTE, true },
        { "unequal-divisor", -1, 1, 1, 2, RRD_ALGORITHM_ABSOLUTE, RRD_ALGORITHM_ABSOLUTE, true },
        { "unequal-algorithm", -1, 1, 1, 1, RRD_ALGORITHM_ABSOLUTE, RRD_ALGORITHM_INCREMENTAL, true },
        { "minimum-magnitude", INT32_MIN, INT32_MIN, 1, 1, RRD_ALGORITHM_ABSOLUTE, RRD_ALGORITHM_ABSOLUTE, false },
        { "minimum-unequal", INT32_MIN, 1, 1, 1, RRD_ALGORITHM_ABSOLUTE, RRD_ALGORITHM_ABSOLUTE, true },
    };

    int rc = 0;
    for(size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "unittest-homogeneity-%s", cases[i].name);

        RRDSET *st = rrdset_create_localhost(
            "netdata", id, id, "netdata", NULL, "Unit Testing", "x", "unittest", NULL, 1,
            nd_profile.update_every, RRDSET_TYPE_LINE);
        rrddim_add(st, "a", NULL, cases[i].multiplier_a, cases[i].divisor_a, cases[i].algorithm_a);
        rrddim_add(st, "b", NULL, cases[i].multiplier_b, cases[i].divisor_b, cases[i].algorithm_b);

        bool immediate = rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS);
        if(immediate != cases[i].heterogeneous) {
            fprintf(stderr, "%s: %s insertion classified heterogeneous=%d, expected %d\n",
                    __FUNCTION__, cases[i].name, immediate, cases[i].heterogeneous);
            rc = 1;
        }

        rrdset_flag_set(st, RRDSET_FLAG_HOMOGENEOUS_CHECK);
        rrdset_update_heterogeneous_flag(st);
        bool deferred = rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS);
        if(deferred != cases[i].heterogeneous ||
           rrdset_flag_check(st, RRDSET_FLAG_HOMOGENEOUS_CHECK)) {
            fprintf(stderr, "%s: %s deferred classified heterogeneous=%d, expected %d (check pending=%d)\n",
                    __FUNCTION__, cases[i].name, deferred, cases[i].heterogeneous,
                    rrdset_flag_check(st, RRDSET_FLAG_HOMOGENEOUS_CHECK));
            rc = 1;
        }
    }

    RRDSET *st_single = rrdset_create_localhost(
        "netdata", "unittest-homogeneity-single", "unittest-homogeneity-single", "netdata", NULL,
        "Unit Testing", "x", "unittest", NULL, 1, nd_profile.update_every, RRDSET_TYPE_LINE);
    rrddim_add(st_single, "a", NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrdset_flag_set(st_single, RRDSET_FLAG_HOMOGENEOUS_CHECK);
    rrdset_update_heterogeneous_flag(st_single);
    if(rrdset_flag_check(st_single, RRDSET_FLAG_HETEROGENEOUS) ||
       rrdset_flag_check(st_single, RRDSET_FLAG_HOMOGENEOUS_CHECK)) {
        fprintf(stderr, "%s: one negative dimension was not homogeneous after deferred recomputation\n", __FUNCTION__);
        rc = 1;
    }

    RRDSET *st_update = rrdset_create_localhost(
        "netdata", "unittest-homogeneity-update", "unittest-homogeneity-update", "netdata", NULL,
        "Unit Testing", "x", "unittest", NULL, 1, nd_profile.update_every, RRDSET_TYPE_LINE);
    rrddim_add(st_update, "a", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    rrddim_add(st_update, "b", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    RRDDIM *first = NULL, *second = NULL, *rd;
    rrddim_foreach_read(rd, st_update) {
        if(!first)
            first = rd;
        else if(!second)
            second = rd;
    }
    rrddim_foreach_done(rd);

    if(!first || !second) {
        fprintf(stderr, "%s: failed to locate both metadata-update dimensions\n", __FUNCTION__);
        rc = 1;
    }
    else {
        rrddim_set_multiplier(st_update, first, -1);
        if(!rrdset_flag_check(st_update, RRDSET_FLAG_HOMOGENEOUS_CHECK)) {
            fprintf(stderr, "%s: multiplier metadata update did not request homogeneity recomputation\n", __FUNCTION__);
            rc = 1;
        }

        rrdset_update_heterogeneous_flag(st_update);
        if(rrdset_flag_check(st_update, RRDSET_FLAG_HETEROGENEOUS)) {
            fprintf(stderr, "%s: equal magnitudes became heterogeneous after first-dimension sign update\n", __FUNCTION__);
            rc = 1;
        }

        rrddim_set_multiplier(st_update, second, 2);
        rrdset_update_heterogeneous_flag(st_update);
        if(!rrdset_flag_check(st_update, RRDSET_FLAG_HETEROGENEOUS)) {
            fprintf(stderr, "%s: unequal updated magnitudes were not heterogeneous\n", __FUNCTION__);
            rc = 1;
        }

        rrddim_set_multiplier(st_update, second, -1);
        rrdset_update_heterogeneous_flag(st_update);
        if(rrdset_flag_check(st_update, RRDSET_FLAG_HETEROGENEOUS)) {
            fprintf(stderr, "%s: restoring equal negative magnitudes did not clear heterogeneity\n", __FUNCTION__);
            rc = 1;
        }
    }

    default_rrd_memory_mode = old_default_rrd_memory_mode;
    return rc;
}

static int test_rrddim_divisor_normalization(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__);

    RRD_DB_MODE old_default_rrd_memory_mode = default_rrd_memory_mode;
    default_rrd_memory_mode = RRD_DB_MODE_ALLOC;

    RRDSET *st = rrdset_create_localhost(
        "netdata", "unittest-divisor-normalization", "unittest-divisor-normalization", "netdata", NULL,
        "Unit Testing", "x", "unittest", NULL, 1,
        nd_profile.update_every, RRDSET_TYPE_LINE);
    RRDDIM *rd = rrddim_add(st, "d", NULL, 1, 0, RRD_ALGORITHM_ABSOLUTE);

    int rc = 0;
    if(rd->divisor != 1) {
        fprintf(stderr, "%s: construction stored zero divisor as %d instead of 1\n", __FUNCTION__, rd->divisor);
        rc = 1;
    }

    if(rrddim_set_divisor(st, rd, 7) != 1 || rd->divisor != 7) {
        fprintf(stderr, "%s: valid positive divisor was not preserved\n", __FUNCTION__);
        rc = 1;
    }

    if(rrddim_set_divisor(st, rd, 0) != 1 || rd->divisor != 1) {
        fprintf(stderr, "%s: zero divisor update was not normalized to 1\n", __FUNCTION__);
        rc = 1;
    }

    if(rrddim_set_divisor(st, rd, 0) != 0 || rd->divisor != 1) {
        fprintf(stderr, "%s: repeated normalized zero update was not a no-op\n", __FUNCTION__);
        rc = 1;
    }

    if(rrddim_set_divisor(st, rd, -7) != 1 || rd->divisor != -7) {
        fprintf(stderr, "%s: valid negative divisor was not preserved\n", __FUNCTION__);
        rc = 1;
    }

    RRDDIM *same_rd = rrddim_add(st, "d", NULL, 1, 0, RRD_ALGORITHM_ABSOLUTE);
    if(same_rd != rd || rd->divisor != 1) {
        fprintf(stderr, "%s: conflict update did not preserve identity and normalize zero divisor\n", __FUNCTION__);
        rc = 1;
    }

    default_rrd_memory_mode = old_default_rrd_memory_mode;
    return rc;
}

static int test_rrdset_rejects_invalid_update_every(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__);

    RRD_DB_MODE old_default_rrd_memory_mode = default_rrd_memory_mode;
    time_t old_update_every = nd_profile.update_every;
    const time_t original_update_every = 5;

    default_rrd_memory_mode = RRD_DB_MODE_ALLOC;
    nd_profile.update_every = original_update_every;

    RRDSET *st = rrdset_create_localhost(
        "netdata", "unittest-update-every-guard", "unittest-update-every-guard", "netdata", NULL,
        "Unit Testing", "x", "unittest", NULL, 1,
        original_update_every, RRDSET_TYPE_LINE);

    int rc = 0;
    time_t previous = rrdset_set_update_every_s(st, 0);
    if(previous != original_update_every || st->update_every != original_update_every) {
        fprintf(stderr, "%s: zero update every changed chart from %ld to %d\n",
                __FUNCTION__, (long)previous, st->update_every);
        rc = 1;
    }

    previous = rrdset_set_update_every_s(st, -1);
    if(previous != original_update_every || st->update_every != original_update_every) {
        fprintf(stderr, "%s: negative update every changed chart from %ld to %d\n",
                __FUNCTION__, (long)previous, st->update_every);
        rc = 1;
    }

    if(sizeof(time_t) > sizeof(int32_t)) {
        time_t too_large_update_every = (time_t)INT32_MAX;
        too_large_update_every++;
        previous = rrdset_set_update_every_s(st, too_large_update_every);
        if(previous != original_update_every || st->update_every != original_update_every) {
            fprintf(stderr, "%s: out-of-range update every changed chart from %ld to %d\n",
                    __FUNCTION__, (long)previous, st->update_every);
            rc = 1;
        }
    }

    const time_t valid_update_every = 7;
    previous = rrdset_set_update_every_s(st, valid_update_every);
    if(previous != original_update_every || st->update_every != valid_update_every) {
        fprintf(stderr, "%s: valid update every did not change chart from %ld to %ld; current %d\n",
                __FUNCTION__, (long)previous, (long)valid_update_every, st->update_every);
        rc = 1;
    }

    if(st->update_every != original_update_every)
        rrdset_set_update_every_s(st, original_update_every);

    default_rrd_memory_mode = old_default_rrd_memory_mode;
    nd_profile.update_every = old_update_every;
    return rc;
}

static int test_rrdr_relative_window_extreme_values(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__);

    const time_t maximum = (time_t)(((uintmax_t)1 << (sizeof(time_t) * CHAR_BIT - 1)) - 1);
    const time_t minimum = -maximum - 1;
    int errors = 0;

#define RRDR_WINDOW_CHECK(condition, message) do {                                  \
        if(!(condition)) {                                                          \
            fprintf(stderr, "%s: %s\n", __FUNCTION__, (message));                \
            errors++;                                                               \
        }                                                                            \
    } while(0)

    RRDR_WINDOW_CHECK(rrdr_relative_window_value_is_relative(-API_RELATIVE_TIME_MAX),
                      "negative relative boundary was classified as absolute");
    RRDR_WINDOW_CHECK(rrdr_relative_window_value_is_relative(API_RELATIVE_TIME_MAX),
                      "positive relative boundary was classified as absolute");
    RRDR_WINDOW_CHECK(!rrdr_relative_window_value_is_relative(-API_RELATIVE_TIME_MAX - 1),
                      "value below the relative boundary was classified as relative");
    RRDR_WINDOW_CHECK(!rrdr_relative_window_value_is_relative(API_RELATIVE_TIME_MAX + 1),
                      "value above the relative boundary was classified as relative");
    RRDR_WINDOW_CHECK(!rrdr_relative_window_value_is_relative(minimum),
                      "minimum time_t was classified as relative");
    RRDR_WINDOW_CHECK(!rrdr_relative_window_value_is_relative(maximum),
                      "maximum time_t was classified as relative");

    struct {
        time_t after;
        time_t before;
        bool absolute;
        const char *name;
    } query_wrapper_cases[] = {
        { .after = -300, .before = -60, .absolute = false, .name = "relative" },
        { .after = 0, .before = 0, .absolute = false, .name = "default" },
        { .after = -300, .before = maximum, .absolute = false, .name = "mixed" },
        { .after = maximum - 100, .before = maximum, .absolute = true, .name = "future absolute" },
        { .after = minimum, .before = maximum, .absolute = true, .name = "extreme absolute" },
    };

    for(size_t i = 0; i < _countof(query_wrapper_cases); i++) {
        time_t after = query_wrapper_cases[i].after;
        time_t before = query_wrapper_cases[i].before;
        bool absolute = rrdr_relative_window_to_absolute_query(&after, &before, NULL, true);

        if(absolute != query_wrapper_cases[i].absolute) {
            fprintf(stderr, "%s: query wrapper misclassified %s window as %s\n", __FUNCTION__,
                    query_wrapper_cases[i].name, absolute ? "absolute" : "relative");
            errors++;
        }
    }

    {
        const time_t now = 2000000000;
        time_t after = -300;
        time_t before = -60;
        bool relative = rrdr_relative_window_to_absolute(&after, &before, now);

        RRDR_WINDOW_CHECK(relative, "ordinary relative window was classified as absolute");
        RRDR_WINDOW_CHECK(before == now - 60, "ordinary relative before changed");
        RRDR_WINDOW_CHECK(after == now - 60 - 300 + 1, "ordinary relative after changed");
    }

    {
        time_t after = minimum + 100;
        time_t before = maximum;
        bool relative = rrdr_relative_window_to_absolute(&after, &before, maximum - 1);

        RRDR_WINDOW_CHECK(!relative, "absolute future window was classified as relative");
        RRDR_WINDOW_CHECK(before == maximum - 1, "future before was not shifted to now");
        RRDR_WINDOW_CHECK(after == minimum + 99, "wide intermediate changed a representable shifted after");
    }

    {
        time_t after = -3;
        time_t before = minimum + 1;
        bool relative = rrdr_relative_window_to_absolute(&after, &before, 123);

        RRDR_WINDOW_CHECK(relative, "mixed relative window was classified as absolute");
        RRDR_WINDOW_CHECK(before == minimum + 1, "absolute before changed without a future shift");
        RRDR_WINDOW_CHECK(after == minimum, "unrepresentable relative after did not saturate to time_t minimum");
    }

    {
        time_t after = maximum;
        time_t before = minimum;
        bool relative = rrdr_relative_window_to_absolute(&after, &before, 123);

        RRDR_WINDOW_CHECK(!relative, "extreme absolute window was classified as relative");
        RRDR_WINDOW_CHECK(before == 123, "extreme future before was not shifted to now");
        RRDR_WINDOW_CHECK(after == minimum, "unrepresentable future shift did not saturate to time_t minimum");
    }

    {
        time_t after = -1;
        time_t before = -1;
        bool relative = rrdr_relative_window_to_absolute(&after, &before, minimum);

        RRDR_WINDOW_CHECK(relative, "minimum-now relative window was classified as absolute");
        RRDR_WINDOW_CHECK(before == minimum, "minimum-now before did not saturate");
        RRDR_WINDOW_CHECK(after == minimum, "minimum-now after did not saturate");
    }

    {
        time_t after = minimum;
        time_t before = maximum;
        time_t now;
        bool absolute = rrdr_relative_window_to_absolute_query(&after, &before, &now, false);
        time_t minimum_query_time = now - (10 * 365 * 86400);

        RRDR_WINDOW_CHECK(absolute, "query wrapper changed absolute classification");
        RRDR_WINDOW_CHECK(before == now, "query wrapper changed shifted before");
        RRDR_WINDOW_CHECK(after == minimum_query_time, "query wrapper did not apply its existing lower clamp");
    }

#undef RRDR_WINDOW_CHECK

    return errors;
}

static void query_window_test_target_init(
    QUERY_TARGET *qt, time_t after, time_t before, size_t points, time_t resampling_time,
    RRDR_TIME_GROUPING grouping, RRDR_OPTIONS options, time_t update_every) {
    *qt = (QUERY_TARGET){
        .request = {
            .after = after,
            .before = before,
            .points = points,
            .resampling_time = resampling_time,
            .time_group_method = grouping,
        },
        .window = {
            .options = options,
            .after = after,
            .before = before,
        },
        .db = {
            .first_time_s = 1000000000,
            .last_time_s = 1000000600,
            .minimum_latest_update_every_s = update_every,
        },
    };
    snprintfz(qt->id, sizeof(qt->id) - 1, "query-window-unittest");
}

static int test_query_window_resampling_boundaries(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__);

    const time_t maximum = (time_t)(((uintmax_t)1 << (sizeof(time_t) * CHAR_BIT - 1)) - 1);
    int errors = 0;

#define QUERY_WINDOW_CHECK(condition, message) do {                                 \
        if(!(condition)) {                                                          \
            fprintf(stderr, "%s: %s\n", __FUNCTION__, (message));                \
            errors++;                                                               \
        }                                                                            \
    } while(0)

    for(RRDR_TIME_GROUPING grouping = RRDR_GROUPING_AVERAGE;
        grouping <= RRDR_GROUPING_EXTREMES; grouping++) {
        QUERY_TARGET qt;
        query_window_test_target_init(
            &qt, 1000000001, 1000000600, 10, 60, grouping, RRDR_OPTION_NOT_ALIGNED, 1);

        QUERY_WINDOW_CHECK(query_target_calculate_window(&qt), "valid grouping mode was rejected");
        QUERY_WINDOW_CHECK(qt.window.time_group_method == grouping, "grouping mode changed");
        QUERY_WINDOW_CHECK(qt.window.after == 1000000001 && qt.window.before == 1000000600,
                           "unaligned resampling window changed");
        QUERY_WINDOW_CHECK(qt.window.points == 10 && qt.window.group == 60,
                           "unaligned resampling point layout changed");
        QUERY_WINDOW_CHECK(qt.window.resampling_group == 60 && qt.window.resampling_divisor == 1.0,
                           "unaligned resampling ratio changed");
    }

    {
        QUERY_TARGET qt;
        query_window_test_target_init(&qt, 1000000001, 1000000600, 10, 60, RRDR_GROUPING_AVERAGE, 0, 1);
        QUERY_WINDOW_CHECK(query_target_calculate_window(&qt), "valid aligned window was rejected");
        QUERY_WINDOW_CHECK(qt.window.after == 1000000021 && qt.window.before == 1000000620,
                           "aligned resampling endpoints changed");
        QUERY_WINDOW_CHECK(qt.window.points == 10 && qt.window.group == 60,
                           "aligned resampling point layout changed");
    }

    {
        QUERY_TARGET qt;
        query_window_test_target_init(
            &qt, 1000000001, 1000000600, 10, 60, RRDR_GROUPING_AVERAGE,
            RRDR_OPTION_NATURAL_POINTS | RRDR_OPTION_NOT_ALIGNED, 5);
        QUERY_WINDOW_CHECK(query_target_calculate_window(&qt), "valid natural-points window was rejected");
        QUERY_WINDOW_CHECK(qt.window.after == 1000000005 && qt.window.before == 1000000600,
                           "natural-points endpoints changed");
        QUERY_WINDOW_CHECK(qt.window.points == 10 && qt.window.group == 12 &&
                           qt.window.query_granularity == 5 && qt.window.resampling_group == 12,
                           "natural-points layout changed");
    }

    {
        QUERY_TARGET qt;
        query_window_test_target_init(
            &qt, -600, 0, 10, 0, RRDR_GROUPING_AVERAGE, RRDR_OPTION_NOT_ALIGNED, 1);
        QUERY_WINDOW_CHECK(query_target_calculate_window(&qt), "valid relative window was rejected");
        QUERY_WINDOW_CHECK(qt.window.relative, "relative window classification changed");
    }

    if(maximum > INT_MAX) {
        const time_t large_resampling = (time_t)((uint64_t)INT_MAX + 1);
        QUERY_TARGET qt;
        query_window_test_target_init(
            &qt, 1000000000, 1000000600, 2, large_resampling,
            RRDR_GROUPING_AVERAGE, RRDR_OPTION_NOT_ALIGNED, 1);
        QUERY_WINDOW_CHECK(query_target_calculate_window(&qt), "representable cadence above INT_MAX was rejected");
        QUERY_WINDOW_CHECK(qt.window.group == (size_t)large_resampling,
                           "cadence above INT_MAX was narrowed");
        QUERY_WINDOW_CHECK(qt.window.after == (time_t)(1000000600LL - 4294967295LL),
                           "large representable final window changed");

        time_t timestamps[2] = { 0 };
        RRDR r = {
            .n = 2,
            .t = timestamps,
            .internal = { .qt = &qt },
        };
        rrd2rrdr_set_timestamps(&r);
        QUERY_WINDOW_CHECK(r.view.update_every == large_resampling,
                           "RRDR cadence above INT_MAX was narrowed");
        QUERY_WINDOW_CHECK(timestamps[1] == qt.window.before,
                           "large representable timestamps did not end at before");

        if(sizeof(time_t) > sizeof(size_t)) {
            const time_t duration_above_size_max = (time_t)(((uint64_t)INT_MAX + 1) * 5);
            query_window_test_target_init(
                &qt, 1000000000, 1000000600, 2, duration_above_size_max,
                RRDR_GROUPING_AVERAGE,
                RRDR_OPTION_NATURAL_POINTS | RRDR_OPTION_NOT_ALIGNED, 5);
            QUERY_WINDOW_CHECK(query_target_calculate_window(&qt),
                               "representable time64 duration above SIZE_MAX was rejected");
            QUERY_WINDOW_CHECK(qt.window.group == (size_t)((uint64_t)INT_MAX + 1),
                               "time64 duration above SIZE_MAX changed its group");
        }
    }

    for(size_t i = 0; i < 2; i++) {
        QUERY_TARGET qt;
        query_window_test_target_init(
            &qt, 1000000000, 1000000600, 2, maximum, RRDR_GROUPING_AVERAGE,
            i ? RRDR_OPTION_NOT_ALIGNED : 0, 1);
        QUERY_WINDOW_CHECK(!query_target_calculate_window(&qt),
                           "unrepresentable maximum resampling window was accepted");
    }

    {
        RRDSET *st = rrdset_create_localhost(
            "netdata", "unittest-query-window-resampling", "unittest-query-window-resampling",
            "netdata", NULL, "Unit Testing", "x", "unittest", NULL, 1, 1, RRDSET_TYPE_LINE);
        rrddim_add(st, "d", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        QUERY_TARGET_REQUEST request = {
            .version = 1,
            .owa = onewayalloc_create(0),
            .st = st,
            .after = 1000000000,
            .before = 1000000600,
            .points = 2,
            .resampling_time = maximum,
            .alerts = "unittest-alert:*",
            .time_group_method = RRDR_GROUPING_AVERAGE,
            .options = RRDR_OPTION_NOT_ALIGNED,
            .query_source = QUERY_SOURCE_UNITTEST,
            .priority = STORAGE_PRIORITY_SYNCHRONOUS,
        };

        QUERY_WINDOW_CHECK(!query_target_create(&request),
                           "query target accepted an unrepresentable resampling window");
        onewayalloc_destroy(request.owa);

        request.owa = onewayalloc_create(0);
        request.resampling_time = 60;
        QUERY_TARGET *qt = query_target_create(&request);
        QUERY_WINDOW_CHECK(qt, "query target pool did not recover after rejected window");
        query_target_release(qt);
        onewayalloc_destroy(request.owa);
    }

#undef QUERY_WINDOW_CHECK

    return errors;
}

int run_all_mockup_tests(void)
{
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    if(check_strdupz_path_subpath())
        return 1;

    if(check_number_printing())
        return 1;

    if(test_jsonwrap_v2_partial_data_trimming_raw_metadata())
        return 1;

    if(check_rrdcalc_comparisons())
        return 1;

#if defined(OS_LINUX)
    if(proc_interrupts_unittest())
        return 1;
#endif
    if(test_incremental_sum_lookup_respects_update_every())
        return 1;

    if(test_rrdmetric_algorithm_follows_rrddim())
        return 1;

    if(test_rrddim_scale_minimum_magnitude())
        return 1;

    if(test_rrddim_collected_minimum_magnitude())
        return 1;

    if(test_rrdset_homogeneity_multiplier_sign())
        return 1;

    if(test_rrddim_divisor_normalization())
        return 1;

    if(test_rrdset_rejects_invalid_update_every())
        return 1;

    if(test_rrdr_relative_window_extreme_values())
        return 1;

    if(test_query_window_resampling_boundaries())
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
    snprintfz(name, sizeof(name) - 1, "unittest-%d-%ld-%ld", repeat, delay, shift);

    //debug_flags = 0xffffffff;
    default_rrd_memory_mode = RRD_DB_MODE_ALLOC;
    nd_profile.update_every = 1;

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

    unsigned long c, dimensions = rrdset_number_of_dimensions(st);
    RRDDIM *rd;

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
        rrddim_foreach_read(rd, st) {
            rd->collector.last_collected_time.tv_sec = st->last_collected_time.tv_sec;
        }
        rrddim_foreach_done(rd);

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

        rrddim_foreach_read(rd, st) {
            sn = rd->db.data[c];
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
        rrddim_foreach_done(rd);
    }

    if(ret)
        fprintf(stderr, "\n\nUNIT TEST(%ld, %ld) FAILED\n\n", delay, shift);

    return ret;
}

int test_sqlite(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__ );
    sqlite3  *db_mt;
    fprintf(stderr, "Testing SQLIte\n");

    int rc = sqlite3_open(":memory:", &db_mt);
    if (rc != SQLITE_OK) {
        fprintf(stderr,"Failed to test SQLite: DB init failed\n");
        return 1;
    }

    rc = sqlite3_exec_monitored(db_mt, "CREATE TABLE IF NOT EXISTS mine (id1, id2);", 0, 0, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr,"Failed to test SQLite: Create table failed\n");
        return 1;
    }

    rc = sqlite3_exec_monitored(db_mt, "DELETE FROM MINE LIMIT 1;", 0, 0, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr,"Failed to test SQLite: Delete with LIMIT failed\n");
        return 1;
    }

    rc = sqlite3_exec_monitored(db_mt, "UPDATE MINE SET id1=1 LIMIT 1;", 0, 0, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr,"Failed to test SQLite: Update with LIMIT failed\n");
        return 1;
    }

    rc = sqlite3_create_function(db_mt, "now_usec", 1, SQLITE_ANY, 0, sqlite_now_usec, 0, 0);
    if (unlikely(rc != SQLITE_OK)) {
        fprintf(stderr, "Failed to register internal now_usec function");
        return 1;
    }

    rc = sqlite3_exec_monitored(db_mt, "UPDATE MINE SET id1=now_usec(0);", 0, 0, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr,"Failed to test SQLite: Update with now_usec() failed\n");
        return 1;
    }

    fprintf(stderr,"SQLite is OK\n");
    (void) sqlite3_close_v2(db_mt);
    return 0;
}

#ifdef OS_WINDOWS
int unit_test_windows_virt_normalize(void) {
    static const struct {
        const char *raw;
        const char *expected;
    } cases[] = {
        {"VMware Virtual Platform",            "vmware"},
        {"VMware7,1",                          "vmware"},
        {"VirtualBox",                         "oracle"},
        {"innotek GmbH VirtualBox",            "oracle"},
        {"Oracle Corporation VirtualBox",      "oracle"},
        {"Parallels Software International",   "parallels"},
        {"QEMU",                               "qemu"},
        {"QEMU Standard PC (i440FX + PIIX, 1995)", "qemu"},
        {"KVM",                                "kvm"},
        {"Standard PC (i440FX + PIIX, 1995)",  "unknown"},
        {"HVM domU",                           "xen"},
        {"Amazon EC2",                         "amazon"},
        {"amazon ec2",                         "amazon"},
        {"DigitalOcean Droplet",               "digitalocean"},
        {"Microsoft Hv",                       "microsoft"},
        {"Virtual Machine",                    "microsoft"},
        {"Hyper-V",                            "microsoft"},
        {"Microsoft Corporation",              "unknown"},
        {"Surface Laptop 5",                   "unknown"},
        {"HP ProLiant DL380 Gen10",            "unknown"},
        {"Linode",                             "unknown"},
        {"OpenStack",                          "unknown"},
        {"",                                   "none"},
        {NULL,                                 "none"},
    };

    int failures = 0;
    for(size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        const char *got = netdata_windows_normalize_virt_string(cases[i].raw);
        if(strcmp(got, cases[i].expected) != 0) {
            fprintf(stderr,
                    "unit_test_windows_virt_normalize: case '%s' expected '%s' got '%s'\n",
                    cases[i].raw ? cases[i].raw : "(NULL)",
                    cases[i].expected,
                    got);
            failures++;
        }
    }

    if(failures) {
        fprintf(stderr, "unit_test_windows_virt_normalize: %d failure(s)\n", failures);
        return 1;
    }

    fprintf(stderr, "unit_test_windows_virt_normalize: OK (%zu cases)\n",
            sizeof(cases) / sizeof(cases[0]));
    return 0;
}

int unit_test_windows_virt_resolution(void) {
    static const struct {
        const char *wmi;
        const char *smbios;
        const char *registry;
        const char *expected;
    } cases[] = {
        {"vmware", "unknown", "oracle", "vmware"},
        {NULL, "unknown", "vmware", "vmware"},
        {NULL, "unknown", "oracle", "oracle"},
        {NULL, "unknown", NULL, "unknown"},
        {NULL, "kvm", "vmware", "kvm"},
        {NULL, NULL, "parallels", "parallels"},
        {NULL, NULL, NULL, "none"},
    };

    int failures = 0;
    for(size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        const char *got = netdata_windows_resolve_virt_detection(
            cases[i].wmi, cases[i].smbios, cases[i].registry);
        if(strcmp(got, cases[i].expected) != 0) {
            fprintf(stderr,
                    "unit_test_windows_virt_resolution: case %zu expected '%s' got '%s'\n",
                    i,
                    cases[i].expected,
                    got);
            failures++;
        }
    }

    if(failures) {
        fprintf(stderr, "unit_test_windows_virt_resolution: %d failure(s)\n", failures);
        return 1;
    }

    fprintf(stderr, "unit_test_windows_virt_resolution: OK (%zu cases)\n",
            sizeof(cases) / sizeof(cases[0]));
    return 0;
}

int unit_test_windows_container(void) {
    int failures = 0;

    // Kubernetes env-var probe. NULL expected means "not detected via env, fall back to WMI".
    static const struct {
        const char *host;
        const char *port;
        const char *expected;
    } env_cases[] = {
        {"10.0.0.1", "443",  NETDATA_WIN_CONTAINER_KUBERNETES},
        {"10.0.0.1", "6443", NETDATA_WIN_CONTAINER_KUBERNETES},
        {"10.0.0.1", "",     NULL},
        {"",         "443",  NULL},
        {"",         "",     NULL},
        {NULL,       "443",  NULL},
        {"10.0.0.1", NULL,   NULL},
        {NULL,       NULL,   NULL},
    };

    for(size_t i = 0; i < sizeof(env_cases) / sizeof(env_cases[0]); i++) {
        const char *got = netdata_windows_container_from_env(env_cases[i].host, env_cases[i].port);
        bool ok = (got == NULL && env_cases[i].expected == NULL) ||
                  (got != NULL && env_cases[i].expected != NULL && strcmp(got, env_cases[i].expected) == 0);
        if(!ok) {
            fprintf(stderr,
                    "unit_test_windows_container: env case %zu (host='%s' port='%s') expected '%s' got '%s'\n",
                    i,
                    env_cases[i].host ? env_cases[i].host : "(NULL)",
                    env_cases[i].port ? env_cases[i].port : "(NULL)",
                    env_cases[i].expected ? env_cases[i].expected : "(NULL)",
                    got ? got : "(NULL)");
            failures++;
        }
    }

    // Container classification -> detection-method mapping.
    static const struct {
        const char *container;
        const char *expected;
    } map_cases[] = {
        {NETDATA_WIN_CONTAINER_KUBERNETES, NETDATA_WIN_CONTAINER_KUBERNETES_DETECT},
        {NETDATA_WIN_CONTAINER_WINDOWS,    NETDATA_WIN_CONTAINER_WINDOWS_DETECT},
        {NETDATA_WIN_CONTAINER_NONE,       NETDATA_WIN_CONTAINER_NONE},
        {"unexpected-value",               NETDATA_WIN_CONTAINER_NONE},
        {NULL,                             NETDATA_WIN_CONTAINER_NONE},
    };

    for(size_t i = 0; i < sizeof(map_cases) / sizeof(map_cases[0]); i++) {
        const char *got = netdata_windows_container_detection_method(map_cases[i].container);
        if(strcmp(got, map_cases[i].expected) != 0) {
            fprintf(stderr,
                    "unit_test_windows_container: map case '%s' expected '%s' got '%s'\n",
                    map_cases[i].container ? map_cases[i].container : "(NULL)",
                    map_cases[i].expected,
                    got);
            failures++;
        }
    }

    if(failures) {
        fprintf(stderr, "unit_test_windows_container: %d failure(s)\n", failures);
        return 1;
    }

    fprintf(stderr, "unit_test_windows_container: OK (%zu env + %zu map cases)\n",
            sizeof(env_cases) / sizeof(env_cases[0]),
            sizeof(map_cases) / sizeof(map_cases[0]));
    return 0;
}
#endif
