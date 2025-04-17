// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata.h"

#define MALLOC_ALIGNMENT (sizeof(uintptr_t) * 2)
#define size_t_atomic_count(op, var, size) __atomic_## op ##_fetch(&(var), size, __ATOMIC_RELAXED)
#define size_t_atomic_bytes(op, var, size) __atomic_## op ##_fetch(&(var), ((size) % MALLOC_ALIGNMENT)?((size) + MALLOC_ALIGNMENT - ((size) % MALLOC_ALIGNMENT)):(size), __ATOMIC_RELAXED)

struct rlimit rlimit_nofile = { .rlim_cur = 1024, .rlim_max = 1024 };

// --------------------------------------------------------------------------------------------------------------------

void json_escape_string(char *dst, const char *src, size_t size) {
    const char *t;
    char *d = dst, *e = &dst[size - 1];

    for(t = src; *t && d < e ;t++) {
        if(unlikely(*t == '\\' || *t == '"')) {
            if(unlikely(d + 1 >= e)) break;
            *d++ = '\\';
        }
        *d++ = *t;
    }

    *d = '\0';
}

void json_fix_string(char *s) {
    unsigned char c;
    while((c = (unsigned char)*s)) {
        if(unlikely(c == '\\'))
            *s++ = '/';
        else if(unlikely(c == '"'))
            *s++ = '\'';
        else if(unlikely(isspace(c) || iscntrl(c)))
            *s++ = ' ';
        else if(unlikely(!isprint(c) || c > 127))
            *s++ = '_';
        else
            s++;
    }
}

char *fgets_trim_len(char *buf, size_t buf_size, FILE *fp, size_t *len) {
    char *s = fgets(buf, (int)buf_size, fp);
    if (!s) return NULL;

    char *t = s;
    if (*t != '\0') {
        // find the string end
        while (*++t != '\0');

        // trim trailing spaces/newlines/tabs
        while (--t > s && *t == '\n')
            *t = '\0';
    }

    if (len)
        *len = t - s + 1;

    return s;
}

// vsnprintfz() returns the number of bytes actually written - after possible truncation
int vsnprintfz(char *dst, size_t n, const char *fmt, va_list args) {
    if(unlikely(!n)) return 0;

    int size = vsnprintf(dst, n, fmt, args);
    dst[n - 1] = '\0';

    if (unlikely((size_t) size >= n)) size = (int)(n - 1);

    return size;
}

// snprintfz() returns the number of bytes actually written - after possible truncation
int snprintfz(char *dst, size_t n, const char *fmt, ...) {
    va_list args;

    va_start(args, fmt);
    int ret = vsnprintfz(dst, n, fmt, args);
    va_end(args);

    return ret;
}

// Returns the number of bytes read from the file if file_size is not NULL.
// The actual buffer has an extra byte set to zero (not included in the count).
char *read_by_filename(const char *filename, long *file_size)
{
    FILE *f = fopen(filename, "r");
    if (!f)
        return NULL;

    if (fseek(f, 0, SEEK_END) < 0) {
        fclose(f);
        return NULL;
    }

    long size = ftell(f);
    if (size <= 0 || fseek(f, 0, SEEK_END) < 0) {
        fclose(f);
        return NULL;
    }

    char *contents = callocz(size + 1, 1);
    if (fseek(f, 0, SEEK_SET) < 0) {
        fclose(f);
        freez(contents);
        return NULL;
    }

    size_t res = fread(contents, 1, size, f);
    if ( res != (size_t)size) {
        freez(contents);
        fclose(f);
        return NULL;
    }

    fclose(f);

    if (file_size)
        *file_size = size;

    return contents;
}

char *find_and_replace(const char *src, const char *find, const char *replace, const char *where)
{
    size_t size = strlen(src) + 1;
    size_t find_len = strlen(find);
    size_t repl_len = strlen(replace);
    char *value, *dst;

    if (likely(where))
        size += (repl_len - find_len);

    value = mallocz(size);
    dst = value;

    if (likely(where)) {
        size_t count = where - src;

        memmove(dst, src, count);
        src += count;
        dst += count;

        memmove(dst, replace, repl_len);
        src += find_len;
        dst += repl_len;
    }

    strcpy(dst, src);

    return value;
}

BUFFER *run_command_and_get_output_to_buffer(const char *command, int max_line_length) {
    BUFFER *wb = buffer_create(0, NULL);

    POPEN_INSTANCE *pi = spawn_popen_run(command);
    if(pi) {
        char buffer[max_line_length + 1];
        while (fgets(buffer, max_line_length, spawn_popen_stdout(pi))) {
            buffer[max_line_length] = '\0';
            buffer_strcat(wb, buffer);
        }
        spawn_popen_kill(pi, 0);
    }
    else {
        buffer_free(wb);
        netdata_log_error("Failed to execute command '%s'.", command);
        return NULL;
    }

    return wb;
}

bool run_command_and_copy_output_to_stdout(const char *command, int max_line_length) {
    POPEN_INSTANCE *pi = spawn_popen_run(command);
    if(pi) {
        char buffer[max_line_length + 1];

        while (fgets(buffer, max_line_length, spawn_popen_stdout(pi)))
            fprintf(stdout, "%s", buffer);

        spawn_popen_kill(pi, 0);
    }
    else {
        netdata_log_error("Failed to execute command '%s'.", command);
        return false;
    }

    return true;
}

struct timing_steps {
    const char *name;
    usec_t time;
    size_t count;
} timing_steps[TIMING_STEP_MAX + 1] = {
        [TIMING_STEP_INTERNAL] = { .name = "internal", .time = 0, },

        [TIMING_STEP_BEGIN2_PREPARE] = { .name = "BEGIN2 prepare", .time = 0, },
        [TIMING_STEP_BEGIN2_FIND_CHART] = { .name = "BEGIN2 find chart", .time = 0, },
        [TIMING_STEP_BEGIN2_PARSE] = { .name = "BEGIN2 parse", .time = 0, },
        [TIMING_STEP_BEGIN2_ML] = { .name = "BEGIN2 ml", .time = 0, },
        [TIMING_STEP_BEGIN2_PROPAGATE] = { .name = "BEGIN2 propagate", .time = 0, },
        [TIMING_STEP_BEGIN2_STORE] = { .name = "BEGIN2 store", .time = 0, },

        [TIMING_STEP_SET2_PREPARE] = { .name = "SET2 prepare", .time = 0, },
        [TIMING_STEP_SET2_LOOKUP_DIMENSION] = { .name = "SET2 find dimension", .time = 0, },
        [TIMING_STEP_SET2_PARSE] = { .name = "SET2 parse", .time = 0, },
        [TIMING_STEP_SET2_ML] = { .name = "SET2 ml", .time = 0, },
        [TIMING_STEP_SET2_PROPAGATE] = { .name = "SET2 propagate", .time = 0, },
        [TIMING_STEP_RRDSET_STORE_METRIC] = { .name = "SET2 rrdset store", .time = 0, },
        [TIMING_STEP_DBENGINE_FIRST_CHECK] = { .name = "db 1st check", .time = 0, },
        [TIMING_STEP_DBENGINE_CHECK_DATA] = { .name = "db check data", .time = 0, },
        [TIMING_STEP_DBENGINE_PACK] = { .name = "db pack", .time = 0, },
        [TIMING_STEP_DBENGINE_PAGE_FIN] = { .name = "db page fin", .time = 0, },
        [TIMING_STEP_DBENGINE_MRG_UPDATE] = { .name = "db mrg update", .time = 0, },
        [TIMING_STEP_DBENGINE_PAGE_ALLOC] = { .name = "db page alloc", .time = 0, },
        [TIMING_STEP_DBENGINE_CREATE_NEW_PAGE] = { .name = "db new page", .time = 0, },
        [TIMING_STEP_DBENGINE_FLUSH_PAGE] = { .name = "db page flush", .time = 0, },
        [TIMING_STEP_SET2_STORE] = { .name = "SET2 store", .time = 0, },

        [TIMING_STEP_END2_PREPARE] = { .name = "END2 prepare", .time = 0, },
        [TIMING_STEP_END2_PUSH_V1] = { .name = "END2 push v1", .time = 0, },
        [TIMING_STEP_END2_ML] = { .name = "END2 ml", .time = 0, },
        [TIMING_STEP_END2_RRDSET] = { .name = "END2 rrdset", .time = 0, },
        [TIMING_STEP_END2_PROPAGATE] = { .name = "END2 propagate", .time = 0, },
        [TIMING_STEP_END2_STORE] = { .name = "END2 store", .time = 0, },

        [TIMING_STEP_DBENGINE_EVICT_LOCK]                       = { .name = "EVC_LOCK", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_SELECT]                     = { .name = "EVC_SELECT", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_SELECT_PAGE ]               = { .name = "EVT_SELECT_PAGE", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_RELOCATE_PAGE ]             = { .name = "EVT_RELOCATE_PAGE", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_SORT]                       = { .name = "EVC_SORT", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_DEINDEX]                    = { .name = "EVC_DEINDEX", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_DEINDEX_PAGE]               = { .name = "EVC_DEINDEX_PAGE", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FINISHED]                   = { .name = "EVC_FINISHED", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_LOOP]                  = { .name = "EVC_FREE_LOOP", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_PAGE]                  = { .name = "EVC_FREE_PAGE", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_ATOMICS]               = { .name = "EVC_FREE_ATOMICS", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_CB]                    = { .name = "EVC_FREE_CB", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_ATOMICS2]              = { .name = "EVC_FREE_ATOMICS2", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_ARAL]                  = { .name = "EVC_FREE_ARAL", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_DATA]         = { .name = "EVC_FREE_PGD_DATA", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_ARAL]         = { .name = "EVC_FREE_PGD_ARAL", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_TIER1_ARAL]   = { .name = "EVC_FREE_MAIN_T1ARL", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_GLIVE]        = { .name = "EVC_FREE_MAIN_GLIVE", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_GWORKER]      = { .name = "EVC_FREE_MAIN_GWORK", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_OPEN]                  = { .name = "EVC_FREE_OPEN", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_EXTENT]                = { .name = "EVC_FREE_EXTENT", .time = 0, },

        // terminator
        [TIMING_STEP_MAX] = { .name = NULL, .time = 0, },
};

void timing_action(TIMING_ACTION action, TIMING_STEP step) {
    static __thread usec_t last_action_time = 0;
    static struct timing_steps timings2[TIMING_STEP_MAX + 1] = {};

    switch(action) {
        case TIMING_ACTION_INIT:
            last_action_time = now_monotonic_usec();
            break;

        case TIMING_ACTION_STEP: {
            if(!last_action_time)
                return;

            usec_t now = now_monotonic_usec();
            __atomic_add_fetch(&timing_steps[step].time, now - last_action_time, __ATOMIC_RELAXED);
            __atomic_add_fetch(&timing_steps[step].count, 1, __ATOMIC_RELAXED);
            last_action_time = now;
            break;
        }

        case TIMING_ACTION_FINISH: {
            if(!last_action_time)
                return;

            usec_t expected = __atomic_load_n(&timing_steps[TIMING_STEP_INTERNAL].time, __ATOMIC_RELAXED);
            if(last_action_time - expected < 10 * USEC_PER_SEC) {
                last_action_time = 0;
                return;
            }

            if(!__atomic_compare_exchange_n(&timing_steps[TIMING_STEP_INTERNAL].time, &expected, last_action_time, false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST)) {
                last_action_time = 0;
                return;
            }

            struct timing_steps timings3[TIMING_STEP_MAX + 1];
            memcpy(timings3, timing_steps, sizeof(timings3));

            size_t total_reqs = 0;
            usec_t total_usec = 0;
            for(size_t t = 1; t < TIMING_STEP_MAX ; t++) {
                total_usec += timings3[t].time - timings2[t].time;
                total_reqs += timings3[t].count - timings2[t].count;
            }

            BUFFER *wb = buffer_create(1024, NULL);

            for(size_t t = 1; t < TIMING_STEP_MAX ; t++) {
                size_t requests = timings3[t].count - timings2[t].count;
                if(!requests) continue;

                buffer_sprintf(wb, "TIMINGS REPORT: [%3zu. %-20s]: # %10zu, t %11.2f ms (%6.2f %%), avg %6.2f usec/run\n",
                               t,
                               timing_steps[t].name ? timing_steps[t].name : "x",
                               requests,
                               (double) (timings3[t].time - timings2[t].time) / (double)USEC_PER_MS,
                               (double) (timings3[t].time - timings2[t].time) * 100.0 / (double) total_usec,
                               (double) (timings3[t].time - timings2[t].time) / (double)requests
                );
            }

            netdata_log_info("TIMINGS REPORT:\n%sTIMINGS REPORT:                        total # %10zu, t %11.2f ms",
                 buffer_tostring(wb), total_reqs, (double)total_usec / USEC_PER_MS);

            memcpy(timings2, timings3, sizeof(timings2));

            last_action_time = 0;
            buffer_free(wb);
        }
    }
}

int hash256_string(const unsigned char *string, size_t size, char *hash) {
    EVP_MD_CTX *ctx;
    ctx = EVP_MD_CTX_create();

    if (!ctx)
        return 0;

    if (!EVP_DigestInit(ctx, EVP_sha256())) {
        EVP_MD_CTX_destroy(ctx);
        return 0;
    }

    if (!EVP_DigestUpdate(ctx, string, size)) {
        EVP_MD_CTX_destroy(ctx);
        return 0;
    }

    if (!EVP_DigestFinal(ctx, (unsigned char *)hash, NULL)) {
        EVP_MD_CTX_destroy(ctx);
        return 0;
    }
    EVP_MD_CTX_destroy(ctx);
    return 1;
}


bool rrdr_relative_window_to_absolute(time_t *after, time_t *before, time_t now) {
    if(!now) now = now_realtime_sec();

    int absolute_period_requested = -1;
    time_t before_requested = *before;
    time_t after_requested = *after;

    // allow relative for before (smaller than API_RELATIVE_TIME_MAX)
    if(ABS(before_requested) <= API_RELATIVE_TIME_MAX) {
        // if the user asked for a positive relative time,
        // flip it to a negative
        if(before_requested > 0)
            before_requested = -before_requested;

        before_requested = now + before_requested;
        absolute_period_requested = 0;
    }

    // allow relative for after (smaller than API_RELATIVE_TIME_MAX)
    if(ABS(after_requested) <= API_RELATIVE_TIME_MAX) {
        if(after_requested > 0)
            after_requested = -after_requested;

        // if the user didn't give an after, use the number of points
        // to give a sane default
        if(after_requested == 0)
            after_requested = -600;

        // since the query engine now returns inclusive timestamps
        // it is awkward to return 6 points when after=-5 is given
        // so for relative queries we add 1 second, to give
        // more predictable results to users.
        after_requested = before_requested + after_requested + 1;
        absolute_period_requested = 0;
    }

    if(absolute_period_requested == -1)
        absolute_period_requested = 1;

    // check if the parameters are flipped
    if(after_requested > before_requested) {
        long long t = before_requested;
        before_requested = after_requested;
        after_requested = t;
    }

    // if the query requests future data
    // shift the query back to be in the present time
    // (this may also happen because of the rules above)
    if(before_requested > now) {
        time_t delta = before_requested - now;
        before_requested -= delta;
        after_requested  -= delta;
    }

    *before = before_requested;
    *after = after_requested;

    return (absolute_period_requested != 1);
}

// Returns 1 if an absolute period was requested or 0 if it was a relative period
bool rrdr_relative_window_to_absolute_query(time_t *after, time_t *before, time_t *now_ptr, bool unittest) {
    time_t now = now_realtime_sec() - 1;

    if(now_ptr)
        *now_ptr = now;

    time_t before_requested = *before;
    time_t after_requested = *after;

    int absolute_period_requested = rrdr_relative_window_to_absolute(&after_requested, &before_requested, now);

    time_t absolute_minimum_time = now - (10 * 365 * 86400);
    time_t absolute_maximum_time = now + (1 * 365 * 86400);

    if (after_requested < absolute_minimum_time && !unittest)
        after_requested = absolute_minimum_time;

    if (after_requested > absolute_maximum_time && !unittest)
        after_requested = absolute_maximum_time;

    if (before_requested < absolute_minimum_time && !unittest)
        before_requested = absolute_minimum_time;

    if (before_requested > absolute_maximum_time && !unittest)
        before_requested = absolute_maximum_time;

    *before = before_requested;
    *after = after_requested;

    return (absolute_period_requested != 1);
}


#if defined(OPENSSL_VERSION_NUMBER) && OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110
static inline EVP_ENCODE_CTX *EVP_ENCODE_CTX_new(void)
{
    EVP_ENCODE_CTX *ctx = OPENSSL_malloc(sizeof(*ctx));

    if (ctx != NULL) {
        memset(ctx, 0, sizeof(*ctx));
    }
    return ctx;
}

static void EVP_ENCODE_CTX_free(EVP_ENCODE_CTX *ctx)
{
	OPENSSL_free(ctx);
}
#endif

int netdata_base64_decode(unsigned char *out, const unsigned char *in, const int in_len)
{
    int outl;
    unsigned char remaining_data[256];

    EVP_ENCODE_CTX *ctx = EVP_ENCODE_CTX_new();
    EVP_DecodeInit(ctx);
    EVP_DecodeUpdate(ctx, out, &outl, in, in_len);
    int remainder = 0;
    EVP_DecodeFinal(ctx, remaining_data, &remainder);
    EVP_ENCODE_CTX_free(ctx);
    if (remainder)
        return -1;

    return outl;
}

int netdata_base64_encode(unsigned char *encoded, const unsigned char *input, size_t input_size)
{
    return EVP_EncodeBlock(encoded, input, input_size);
}

// Keep internal implementation
// int netdata_base64_decode_internal(const char *encoded, char *decoded, size_t decoded_size) {
//     static const unsigned char base64_table[256] = {
//             ['A'] = 0, ['B'] = 1, ['C'] = 2, ['D'] = 3, ['E'] = 4, ['F'] = 5, ['G'] = 6, ['H'] = 7,
//             ['I'] = 8, ['J'] = 9, ['K'] = 10, ['L'] = 11, ['M'] = 12, ['N'] = 13, ['O'] = 14, ['P'] = 15,
//             ['Q'] = 16, ['R'] = 17, ['S'] = 18, ['T'] = 19, ['U'] = 20, ['V'] = 21, ['W'] = 22, ['X'] = 23,
//             ['Y'] = 24, ['Z'] = 25, ['a'] = 26, ['b'] = 27, ['c'] = 28, ['d'] = 29, ['e'] = 30, ['f'] = 31,
//             ['g'] = 32, ['h'] = 33, ['i'] = 34, ['j'] = 35, ['k'] = 36, ['l'] = 37, ['m'] = 38, ['n'] = 39,
//             ['o'] = 40, ['p'] = 41, ['q'] = 42, ['r'] = 43, ['s'] = 44, ['t'] = 45, ['u'] = 46, ['v'] = 47,
//             ['w'] = 48, ['x'] = 49, ['y'] = 50, ['z'] = 51, ['0'] = 52, ['1'] = 53, ['2'] = 54, ['3'] = 55,
//             ['4'] = 56, ['5'] = 57, ['6'] = 58, ['7'] = 59, ['8'] = 60, ['9'] = 61, ['+'] = 62, ['/'] = 63,
//             [0 ... '+' - 1] = 255,
//             ['+' + 1 ... '/' - 1] = 255,
//             ['9' + 1 ... 'A' - 1] = 255,
//             ['Z' + 1 ... 'a' - 1] = 255,
//             ['z' + 1 ... 255] = 255
//     };
//
//     size_t count = 0;
//     unsigned int tmp = 0;
//     int i, bit;
//
//     if (decoded_size < 1)
//         return 0; // Buffer size must be at least 1 for null termination
//
//     for (i = 0, bit = 0; encoded[i]; i++) {
//         unsigned char value = base64_table[(unsigned char)encoded[i]];
//         if (value > 63)
//             return -1; // Invalid character in input
//
//         tmp = tmp << 6 | value;
//         if (++bit == 4) {
//             if (count + 3 >= decoded_size) break; // Stop decoding if buffer is full
//             decoded[count++] = (tmp >> 16) & 0xFF;
//             decoded[count++] = (tmp >> 8) & 0xFF;
//             decoded[count++] = tmp & 0xFF;
//             tmp = 0;
//             bit = 0;
//         }
//     }
//
//     if (bit > 0 && count + 1 < decoded_size) {
//         tmp <<= 6 * (4 - bit);
//         if (bit > 2 && count + 1 < decoded_size) decoded[count++] = (tmp >> 16) & 0xFF;
//         if (bit > 3 && count + 1 < decoded_size) decoded[count++] = (tmp >> 8) & 0xFF;
//     }
//
//     decoded[count] = '\0'; // Null terminate the output string
//     return count;
// }

void libnetdata_init(void) {
    string_init();
    eval_functions_init();
}
