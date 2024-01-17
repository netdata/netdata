// Copyright: SPDX-License-Identifier:  GPL-3.0-only

#include "ringbuffer.h"

// to be able to access internals
// never do this from app
#include "../src/ringbuffer_internal.h"

#include <stdio.h>
#include <string.h>

#define KNRM "\x1B[0m"
#define KRED "\x1B[31m"
#define KGRN "\x1B[32m"
#define KYEL "\x1B[33m"
#define KBLU "\x1B[34m"
#define KMAG "\x1B[35m"
#define KCYN "\x1B[36m"
#define KWHT "\x1B[37m"

#define UNUSED(x) (void)(x)

int total_fails = 0;
int total_tests = 0;
int total_checks = 0;

#define CHECK_EQ_RESULT(x, y)                                                                                          \
    while (s_len--)                                                                                                    \
        putchar('.');                                                                                                  \
    printf("%s%s " KNRM "\n", (((x) == (y)) ? KGRN : KRED), (((x) == (y)) ? " PASS " : " FAIL "));                     \
    if ((x) != (y))                                                                                                    \
        total_fails++;                                                                                                 \
    total_checks++;

#define CHECK_EQ_PREFIX(x, y, prefix, subtest_name, ...)                                                               \
    {                                                                                                                  \
        int s_len =                                                                                                    \
            100 -                                                                                                      \
            printf(("Checking: " KWHT "%s %s%2d " subtest_name " " KNRM), __func__, prefix, subtest_no, ##__VA_ARGS__);    \
        CHECK_EQ_RESULT(x, y)                                                                                          \
    }

#define CHECK_EQ(x, y, subtest_name, ...)                                                                              \
    {                                                                                                                  \
        int s_len =                                                                                                    \
            100 - printf(("Checking: " KWHT "%s %2d " subtest_name " " KNRM), __func__, subtest_no, ##__VA_ARGS__);        \
        CHECK_EQ_RESULT(x, y)                                                                                          \
    }

#define TEST_DECL()                                                                                                    \
    int subtest_no = 0;                                                                                                \
    printf(KYEL "TEST SUITE: %s\n" KNRM, __func__);                                                                    \
    total_tests++;

static void test_rbuf_get_linear_insert_range()
{
    TEST_DECL();

    // check empty buffer behaviour
    rbuf_t buff = rbuf_create(5);
    char *to_write;
    size_t ret;
    to_write = rbuf_get_linear_insert_range(buff, &ret);
    CHECK_EQ(ret, 5, "empty size");
    CHECK_EQ(to_write, buff->head, "empty write ptr");
    rbuf_free(buff);

    // check full buffer behaviour
    subtest_no++;
    buff = rbuf_create(5);
    ret = rbuf_bump_head(buff, 5);
    CHECK_EQ(ret, 1, "ret");
    to_write = rbuf_get_linear_insert_range(buff, &ret);
    CHECK_EQ(to_write, NULL, "writable NULL");
    CHECK_EQ(ret, 0, "writable count = 0");

    // check buffer flush
    subtest_no++;
    rbuf_flush(buff);
    CHECK_EQ(rbuf_bytes_free(buff), 5, "size_free");
    CHECK_EQ(rbuf_bytes_available(buff), 0, "size_avail");
    CHECK_EQ(buff->head, buff->data, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");

    // check behaviour head > tail
    subtest_no++;
    rbuf_flush(buff);
    rbuf_bump_head(buff, 3);
    to_write = rbuf_get_linear_insert_range(buff, &ret);
    CHECK_EQ(to_write, buff->head, "write location");
    CHECK_EQ(ret, 2, "availible to linear write");

    // check behaviour tail > head
    subtest_no++;
    rbuf_flush(buff);
    rbuf_bump_head(buff, 5);
    rbuf_bump_tail(buff, 3);
    CHECK_EQ(buff->head, buff->data, "head_ptr");
    CHECK_EQ(buff->tail, buff->data + 3, "tail_ptr");
    to_write = rbuf_get_linear_insert_range(buff, &ret);
    CHECK_EQ(to_write, buff->head, "write location");
    CHECK_EQ(ret, 3, "availible to linear write");

/*    // check behaviour tail and head at last element
    subtest_no++;
    rbuf_flush(buff);
    rbuf_bump_head(buff, 4);
    rbuf_bump_tail(buff, 4);
    CHECK_EQ(buff->head, buff->end - 1, "head_ptr");
    CHECK_EQ(buff->tail, buff->end - 1, "tail_ptr");
    to_write = rbuf_get_linear_insert_range(buff, &ret);
    CHECK_EQ(to_write, buff->head, "write location");
    CHECK_EQ(ret, 1, "availible to linear write");*/

    // check behaviour tail and head at last element
    // after rbuf_bump_tail optimisation that restarts buffer
    // in case tail catches up with head
    subtest_no++;
    rbuf_flush(buff);
    rbuf_bump_head(buff, 4);
    rbuf_bump_tail(buff, 4);
    CHECK_EQ(buff->head, buff->data, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");
    to_write = rbuf_get_linear_insert_range(buff, &ret);
    CHECK_EQ(to_write, buff->head, "write location");
    CHECK_EQ(ret, 5, "availible to linear write");
}

#define _CHECK_EQ(x, y, subtest_name, ...) CHECK_EQ_PREFIX(x, y, prefix, subtest_name, ##__VA_ARGS__)
#define _PREFX "(size = %5zu) "
static void test_rbuf_bump_head_bsize(size_t size)
{
    char prefix[16];
    snprintf(prefix, 16, _PREFX, size);
    int subtest_no = 0;
    rbuf_t buff = rbuf_create(size);
    _CHECK_EQ(rbuf_bytes_free(buff), size, "size_free");

    subtest_no++;
    int ret = rbuf_bump_head(buff, size);
    _CHECK_EQ(buff->data, buff->head, "loc");
    _CHECK_EQ(ret, 1, "ret");
    _CHECK_EQ(buff->size_data, buff->size, "size");
    _CHECK_EQ(rbuf_bytes_free(buff), 0, "size_free");

    subtest_no++;
    ret = rbuf_bump_head(buff, 1);
    _CHECK_EQ(buff->data, buff->head, "loc no move");
    _CHECK_EQ(ret, 0, "ret error");
    _CHECK_EQ(buff->size_data, buff->size, "size");
    _CHECK_EQ(rbuf_bytes_free(buff), 0, "size_free");
    rbuf_free(buff);

    subtest_no++;
    buff = rbuf_create(size);
    ret = rbuf_bump_head(buff, size - 1);
    _CHECK_EQ(buff->head, buff->end-1, "loc end");
    rbuf_free(buff);
}
#undef _CHECK_EQ

static void test_rbuf_bump_head()
{
    TEST_DECL();
    UNUSED(subtest_no);

    size_t test_sizes[] = { 1, 2, 3, 5, 6, 7, 8, 100, 99999, 0 };
    for (int i = 0; test_sizes[i]; i++)
        test_rbuf_bump_head_bsize(test_sizes[i]);
}

static void test_rbuf_bump_tail_noopt(int subtest_no)
{
    rbuf_t buff = rbuf_create(10);
    CHECK_EQ(rbuf_bytes_free(buff), 10, "size_free");
    CHECK_EQ(rbuf_bytes_available(buff), 0, "size_avail");

    subtest_no++;
    int ret = rbuf_bump_head(buff, 5);
    CHECK_EQ(ret, 1, "ret");
    CHECK_EQ(rbuf_bytes_free(buff), 5, "size_free");
    CHECK_EQ(rbuf_bytes_available(buff), 5, "size_avail");
    CHECK_EQ(buff->head, buff->data + 5, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");

    subtest_no++;
    ret = rbuf_bump_tail_noopt(buff, 2);
    CHECK_EQ(ret, 1, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 3, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 7, "size_free");
    CHECK_EQ(buff->head, buff->data + 5, "head_ptr");
    CHECK_EQ(buff->tail, buff->data + 2, "tail_ptr");

    subtest_no++;
    ret = rbuf_bump_tail_noopt(buff, 3);
    CHECK_EQ(ret, 1, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 0, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 10, "size_free");
    CHECK_EQ(buff->head, buff->data + 5, "head_ptr");
    CHECK_EQ(buff->tail, buff->data + 5, "tail_ptr");

    subtest_no++;
    ret = rbuf_bump_tail_noopt(buff, 1);
    CHECK_EQ(ret, 0, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 0, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 10, "size_free");
    CHECK_EQ(buff->head, buff->data + 5, "head_ptr");
    CHECK_EQ(buff->tail, buff->data + 5, "tail_ptr");

    subtest_no++;
    ret = rbuf_bump_head(buff, 7);
    CHECK_EQ(ret, 1, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 7, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 3, "size_free");
    CHECK_EQ(buff->head, buff->data + 2, "head_ptr");
    CHECK_EQ(buff->tail, buff->data + 5, "tail_ptr");

    subtest_no++;
    ret = rbuf_bump_tail_noopt(buff, 5);
    CHECK_EQ(ret, 1, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 2, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 8, "size_free");
    CHECK_EQ(buff->head, buff->data + 2, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");

    // check tail can't overrun head
    subtest_no++;
    ret = rbuf_bump_tail_noopt(buff, 3);
    CHECK_EQ(ret, 0, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 2, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 8, "size_free");
    CHECK_EQ(buff->head, buff->data + 2, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");

    // check head can't overrun tail
    subtest_no++;
    ret = rbuf_bump_head(buff, 9);
    CHECK_EQ(ret, 0, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 2, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 8, "size_free");
    CHECK_EQ(buff->head, buff->data + 2, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");

    // check head can fill the buffer
    subtest_no++;
    ret = rbuf_bump_head(buff, 8);
    CHECK_EQ(ret, 1, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 10, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 0, "size_free");
    CHECK_EQ(buff->head, buff->data, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");

    // check can empty the buffer
    subtest_no++;
    ret = rbuf_bump_tail_noopt(buff, 10);
    CHECK_EQ(ret, 1, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 0, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 10, "size_free");
    CHECK_EQ(buff->head, buff->data, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");
}

static void test_rbuf_bump_tail_opt(int subtest_no)
{
    subtest_no++;
    rbuf_t buff = rbuf_create(10);
    CHECK_EQ(rbuf_bytes_free(buff), 10, "size_free");
    CHECK_EQ(rbuf_bytes_available(buff), 0, "size_avail");

    subtest_no++;
    int ret = rbuf_bump_head(buff, 5);
    CHECK_EQ(ret, 1, "ret");
    CHECK_EQ(rbuf_bytes_free(buff), 5, "size_free");
    CHECK_EQ(rbuf_bytes_available(buff), 5, "size_avail");
    CHECK_EQ(buff->head, buff->data + 5, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");

    subtest_no++;
    ret = rbuf_bump_tail(buff, 2);
    CHECK_EQ(ret, 1, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 3, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 7, "size_free");
    CHECK_EQ(buff->head, buff->data + 5, "head_ptr");
    CHECK_EQ(buff->tail, buff->data + 2, "tail_ptr");

    subtest_no++;
    ret = rbuf_bump_tail(buff, 3);
    CHECK_EQ(ret, 1, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 0, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 10, "size_free");
    CHECK_EQ(buff->head, buff->data, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");

    subtest_no++;
    ret = rbuf_bump_tail_noopt(buff, 1);
    CHECK_EQ(ret, 0, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 0, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 10, "size_free");
    CHECK_EQ(buff->head, buff->data, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");

    subtest_no++;
    ret = rbuf_bump_head(buff, 6);
    ret = rbuf_bump_tail(buff, 5);
    ret = rbuf_bump_head(buff, 6);
    CHECK_EQ(ret, 1, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 7, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 3, "size_free");
    CHECK_EQ(buff->head, buff->data + 2, "head_ptr");
    CHECK_EQ(buff->tail, buff->data + 5, "tail_ptr");

    subtest_no++;
    ret = rbuf_bump_tail(buff, 5);
    CHECK_EQ(ret, 1, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 2, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 8, "size_free");
    CHECK_EQ(buff->head, buff->data + 2, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");

    // check tail can't overrun head
    subtest_no++;
    ret = rbuf_bump_tail(buff, 3);
    CHECK_EQ(ret, 0, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 2, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 8, "size_free");
    CHECK_EQ(buff->head, buff->data + 2, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");

    // check head can't overrun tail
    subtest_no++;
    ret = rbuf_bump_head(buff, 9);
    CHECK_EQ(ret, 0, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 2, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 8, "size_free");
    CHECK_EQ(buff->head, buff->data + 2, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");

    // check head can fill the buffer
    subtest_no++;
    ret = rbuf_bump_head(buff, 8);
    CHECK_EQ(ret, 1, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 10, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 0, "size_free");
    CHECK_EQ(buff->head, buff->data, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");

    // check can empty the buffer
    subtest_no++;
    ret = rbuf_bump_tail(buff, 10);
    CHECK_EQ(ret, 1, "ret");
    CHECK_EQ(rbuf_bytes_available(buff), 0, "size_avail");
    CHECK_EQ(rbuf_bytes_free(buff), 10, "size_free");
    CHECK_EQ(buff->head, buff->data, "head_ptr");
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");
}

static void test_rbuf_bump_tail()
{
    TEST_DECL();
    test_rbuf_bump_tail_noopt(subtest_no);
    test_rbuf_bump_tail_opt(subtest_no);
}

#define ASCII_A 0x61
#define ASCII_Z 0x7A
#define TEST_DATA_SIZE ASCII_Z-ASCII_A+1
static void test_rbuf_push()
{
    TEST_DECL();
    rbuf_t buff = rbuf_create(10);
    int i;
    char test_data[TEST_DATA_SIZE];

    for (int i = 0; i <= TEST_DATA_SIZE; i++)
        test_data[i] = i + ASCII_A;

    int ret = rbuf_push(buff, test_data, 10);
    CHECK_EQ(ret, 10, "written 10 bytes");
    CHECK_EQ(rbuf_bytes_free(buff), 0, "empty size == 0");
    for (i = 0; i < 10; i++)
        CHECK_EQ(buff->data[i], i + ASCII_A, "Check data");

    subtest_no++;
    rbuf_flush(buff);
    rbuf_bump_head(buff, 5);
    rbuf_bump_tail_noopt(buff, 5); //to not reset both pointers to beginning
    ret = rbuf_push(buff, test_data, 10);
    CHECK_EQ(ret, 10, "written 10 bytes");
    for (i = 0; i < 10; i++)
        CHECK_EQ(buff->data[i], ((i+5)%10) + ASCII_A, "Check Data");

    subtest_no++;
    rbuf_flush(buff);
    rbuf_bump_head(buff, 9);
    rbuf_bump_tail_noopt(buff, 9);
    ret = rbuf_push(buff, test_data, 10);
    CHECK_EQ(ret, 10, "written 10 bytes");
    for (i = 0; i < 10; i++)
        CHECK_EQ(buff->data[i], ((i + 1) % 10) + ASCII_A, "Check data");

    // let tail > head
    subtest_no++;
    rbuf_flush(buff);
    rbuf_bump_head(buff, 9);
    rbuf_bump_tail_noopt(buff, 9);
    rbuf_bump_head(buff, 1);
    ret = rbuf_push(buff, test_data, 9);
    CHECK_EQ(ret, 9, "written 9 bytes");
    CHECK_EQ(buff->head, buff->end - 1, "head_ptr");
    CHECK_EQ(buff->tail, buff->head, "tail_ptr");
    rbuf_bump_tail(buff, 1);
    //TODO push byte can be usefull optimisation
    ret = rbuf_push(buff, &test_data[9], 1);
    CHECK_EQ(ret, 1, "written 1 byte");
    CHECK_EQ(rbuf_bytes_free(buff), 0, "empty size == 0");
    for (i = 0; i < 10; i++)
        CHECK_EQ(buff->data[i], i + ASCII_A, "Check data");

    subtest_no++;
    rbuf_flush(buff);
    rbuf_bump_head(buff, 9);
    rbuf_bump_tail_noopt(buff, 7);
    rbuf_bump_head(buff, 1);
    ret = rbuf_push(buff, test_data, 7);
    CHECK_EQ(ret, 7, "written 7 bytes");
    CHECK_EQ(buff->head, buff->data + 7, "head_ptr");
    CHECK_EQ(buff->tail, buff->head, "tail_ptr");
    rbuf_bump_tail(buff, 3);
    CHECK_EQ(buff->tail, buff->data, "tail_ptr");
    //TODO push byte can be usefull optimisation
    ret = rbuf_push(buff, &test_data[7], 3);
    CHECK_EQ(ret, 3, "written 3 bytes");
    CHECK_EQ(rbuf_bytes_free(buff), 0, "empty size == 0");
    for (i = 0; i < 10; i++)
        CHECK_EQ(buff->data[i], i + ASCII_A, "Check data");

    // test can't overfill the buffer
    subtest_no++;
    rbuf_flush(buff);
    rbuf_push(buff, test_data, TEST_DATA_SIZE);
    CHECK_EQ(ret, 3, "written 10 bytes");
    for (i = 0; i < 10; i++)
        CHECK_EQ(buff->data[i], i + ASCII_A, "Check data");
}

#define TEST_RBUF_FIND_BYTES_SIZE 10
void test_rbuf_find_bytes()
{
    TEST_DECL();
    rbuf_t buff = rbuf_create(TEST_RBUF_FIND_BYTES_SIZE);
    char *filler_3 = "   ";
    char *needle = "needle";
    int idx;
    char *ptr;

    // make sure needle is wrapped aroung in the buffer
    // to test we still can find it
    // target "edle    ne"
    rbuf_bump_head(buff, TEST_RBUF_FIND_BYTES_SIZE / 2);
    rbuf_push(buff, filler_3, strlen(filler_3));
    rbuf_bump_tail(buff, TEST_RBUF_FIND_BYTES_SIZE / 2);
    rbuf_push(buff, needle, strlen(needle));
    ptr = rbuf_find_bytes(buff, needle, strlen(needle), &idx);
    CHECK_EQ(ptr, buff->data + (TEST_RBUF_FIND_BYTES_SIZE / 2) + strlen(filler_3), "Pointer to needle correct");
    CHECK_EQ(idx, ptr - buff->tail, "Check needle index");
}

int main()
{
    test_rbuf_bump_head();
    test_rbuf_bump_tail();
    test_rbuf_get_linear_insert_range();
    test_rbuf_push();
    test_rbuf_find_bytes();

    printf(
        KNRM "Total Tests %d, Total Checks %d, Successful Checks %d, Failed Checks %d\n",
        total_tests, total_checks, total_checks - total_fails, total_fails);
    if (total_fails)
        printf(KRED "!!!Some test(s) Failed!!!\n");
    else
        printf(KGRN "ALL TESTS PASSED\n");

    return total_fails;
}
