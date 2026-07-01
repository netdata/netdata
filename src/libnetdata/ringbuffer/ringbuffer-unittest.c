// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#define RB_TEST(condition, msg) do {                                           \
        if (!(condition)) {                                                    \
            fprintf(stderr, "ringbuffer unittest FAILED: %s (%s:%d)\n",       \
                    (msg), __FUNCTION__, __LINE__);                            \
            errors++;                                                          \
        }                                                                      \
    } while(0)

static int ringbuffer_fixed_size_unittest(void)
{
    int errors = 0;
    rbuf_t buffer = rbuf_create(8, 8);

    RB_TEST(rbuf_get_capacity(buffer) == 8, "fixed buffer initial capacity");
    RB_TEST(rbuf_get_max_capacity(buffer) == 8, "fixed buffer max capacity");
    RB_TEST(rbuf_push(buffer, "abcdefgh", 8) == 8, "fixed buffer fill");
    RB_TEST(rbuf_bytes_free(buffer) == 0, "fixed buffer has no free bytes");

    size_t bytes = 1;
    RB_TEST(rbuf_get_linear_insert_range(buffer, &bytes) == NULL, "fixed full buffer refuses insert");
    RB_TEST(bytes == 0, "fixed full buffer returns zero insert bytes");
    RB_TEST(rbuf_get_capacity(buffer) == 8, "fixed full buffer does not grow");

    char out[8];
    RB_TEST(rbuf_pop(buffer, out, sizeof(out)) == sizeof(out), "fixed buffer pop");
    RB_TEST(memcmp(out, "abcdefgh", sizeof(out)) == 0, "fixed buffer FIFO order");

    rbuf_free(buffer);
    return errors;
}

static int ringbuffer_linear_growth_unittest(void)
{
    int errors = 0;
    rbuf_t buffer = rbuf_create(4, 16);

    RB_TEST(rbuf_push(buffer, "abcd", 4) == 4, "growable buffer fill");
    RB_TEST(rbuf_get_capacity(buffer) == 4, "growable buffer starts at initial capacity");

    size_t bytes = 0;
    char *insert = rbuf_get_linear_insert_range(buffer, &bytes);
    RB_TEST(insert != NULL, "full growable buffer grows");
    RB_TEST(rbuf_get_capacity(buffer) == 8, "growable buffer doubles on first growth");
    RB_TEST(bytes == 4, "growth exposes the new free suffix");

    memcpy(insert, "efgh", 4);
    RB_TEST(rbuf_bump_head(buffer, 4), "bump head after linear growth");

    char out[8];
    RB_TEST(rbuf_pop(buffer, out, sizeof(out)) == sizeof(out), "pop after linear growth");
    RB_TEST(memcmp(out, "abcdefgh", sizeof(out)) == 0, "linear growth preserves FIFO order");

    rbuf_free(buffer);
    return errors;
}

static int ringbuffer_wrapped_growth_unittest(void)
{
    int errors = 0;
    rbuf_t buffer = rbuf_create(8, 16);
    char dropped[5];

    RB_TEST(rbuf_push(buffer, "abcdef", 6) == 6, "wrapped setup push");
    RB_TEST(rbuf_pop(buffer, dropped, sizeof(dropped)) == sizeof(dropped), "wrapped setup pop");
    RB_TEST(memcmp(dropped, "abcde", sizeof(dropped)) == 0, "wrapped setup pop order");
    RB_TEST(rbuf_push(buffer, "ghijklm", 7) == 7, "wrapped setup fill");
    RB_TEST(rbuf_bytes_available(buffer) == 8, "wrapped setup is full");
    RB_TEST(rbuf_get_capacity(buffer) == 8, "wrapped setup initial capacity");

    size_t bytes = 0;
    char *insert = rbuf_get_linear_insert_range(buffer, &bytes);
    RB_TEST(insert != NULL, "wrapped full buffer grows");
    RB_TEST(rbuf_get_capacity(buffer) == 16, "wrapped growth doubles capacity");
    RB_TEST(bytes == 8, "wrapped growth exposes free suffix");

    memcpy(insert, "nop", 3);
    RB_TEST(rbuf_bump_head(buffer, 3), "bump head after wrapped growth");

    char out[11];
    RB_TEST(rbuf_pop(buffer, out, sizeof(out)) == sizeof(out), "pop after wrapped growth");
    RB_TEST(memcmp(out, "fghijklmnop", sizeof(out)) == 0, "wrapped growth preserves FIFO order");

    rbuf_free(buffer);
    return errors;
}

static int ringbuffer_cap_unittest(void)
{
    int errors = 0;
    rbuf_t buffer = rbuf_create(4, 8);

    RB_TEST(rbuf_push(buffer, "abcdefgh", 8) == 8, "growable buffer fills to cap");
    RB_TEST(rbuf_get_capacity(buffer) == 8, "growable buffer reaches cap");
    RB_TEST(rbuf_get_max_capacity(buffer) == 8, "growable buffer max remains cap");
    RB_TEST(rbuf_bytes_free(buffer) == 0, "capped buffer has no free bytes");

    size_t bytes = 1;
    RB_TEST(rbuf_get_linear_insert_range(buffer, &bytes) == NULL, "capped full buffer refuses insert");
    RB_TEST(bytes == 0, "capped full buffer returns zero insert bytes");
    RB_TEST(rbuf_get_capacity(buffer) == 8, "capped full buffer does not grow past cap");

    char out[8];
    RB_TEST(rbuf_pop(buffer, out, sizeof(out)) == sizeof(out), "capped buffer pop");
    RB_TEST(memcmp(out, "abcdefgh", sizeof(out)) == 0, "capped buffer FIFO order");

    rbuf_free(buffer);
    return errors;
}

static int ringbuffer_flush_keeps_capacity_unittest(void)
{
    int errors = 0;
    rbuf_t buffer = rbuf_create(4, 16);

    RB_TEST(rbuf_push(buffer, "abcdefgh", 8) == 8, "flush setup grows buffer");
    RB_TEST(rbuf_get_capacity(buffer) == 8, "flush setup capacity");

    rbuf_flush(buffer);
    RB_TEST(rbuf_get_capacity(buffer) == 8, "flush does not shrink buffer");
    RB_TEST(rbuf_get_max_capacity(buffer) == 16, "flush preserves max capacity");
    RB_TEST(rbuf_bytes_available(buffer) == 0, "flush clears readable bytes");
    RB_TEST(rbuf_bytes_free(buffer) == 8, "flush restores free bytes at current capacity");

    rbuf_free(buffer);
    return errors;
}

int ringbuffer_unittest(void)
{
    int errors = 0;

    fprintf(stderr, "\nrunning ringbuffer unittest\n");

    errors += ringbuffer_fixed_size_unittest();
    errors += ringbuffer_linear_growth_unittest();
    errors += ringbuffer_wrapped_growth_unittest();
    errors += ringbuffer_cap_unittest();
    errors += ringbuffer_flush_keeps_capacity_unittest();

    if (errors)
        fprintf(stderr, "ringbuffer unittest: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "ringbuffer unittest: OK\n");

    return errors;
}
